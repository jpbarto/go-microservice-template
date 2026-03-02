package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"dagger/goserv/internal/cicd"
	"dagger/goserv/internal/dagger"
)

// checkResult holds the outcome of a single validation check.
type checkResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Details string `json:"details,omitempty"`
}

// Validate verifies that the goserv deployment is healthy by reproducing
// the checks from tests/validate.sh using the internal/cicd functions.
func (m *Goserv) Validate(
	ctx context.Context,
	// Source directory containing the project (used to read VERSION)
	source *dagger.Directory,
	// +optional
	// Deployment context file produced by Deploy
	deploymentContext *dagger.File,
	// +optional
	// Build as release candidate (appends -rc to version)
	releaseCandidate bool,
) (*dagger.File, error) {
	// --- resolve release coordinates ---
	releaseName := "goserv"
	namespace := "goserv"
	var endpoint string

	if deploymentContext != nil {
		raw, err := deploymentContext.Contents(ctx)
		if err == nil {
			var depCtx map[string]interface{}
			if json.Unmarshal([]byte(raw), &depCtx) == nil {
				if v, ok := depCtx["releaseName"].(string); ok && v != "" {
					releaseName = v
				}
				if v, ok := depCtx["namespace"].(string); ok && v != "" {
					namespace = v
				}
				if v, ok := depCtx["endpoint"].(string); ok {
					endpoint = v
				}
			}
		}
	}

	versionContent, err := source.File("VERSION").Contents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to read VERSION file: %w", err)
	}
	expectedVersion := strings.TrimSpace(versionContent)
	if releaseCandidate {
		expectedVersion += "-rc"
	}

	var checks []checkResult
	pass := func(name string) { checks = append(checks, checkResult{Name: name, Passed: true}) }
	fail := func(name, detail string) {
		checks = append(checks, checkResult{Name: name, Passed: false, Details: detail})
	}

	// -----------------------------------------------------------------------
	// 1. Helm release status & version
	// -----------------------------------------------------------------------
	helmStatusJSON, err := cicd.HelmStatus(ctx, dag, releaseName, namespace)
	if err != nil {
		fail("Helm release exists and is deployed", fmt.Sprintf("helm status failed: %s", err))
	} else {
		var helmStatus map[string]interface{}
		if json.Unmarshal([]byte(helmStatusJSON), &helmStatus) == nil {
			if info, ok := helmStatus["info"].(map[string]interface{}); ok {
				if status, _ := info["status"].(string); status == "deployed" {
					pass("Helm release exists and is deployed")
				} else {
					fail("Helm release exists and is deployed", "status is: "+status)
				}
			}
		}
	}

	// version check via helm list
	if expectedVersion != "" {
		helmListJSON, err := cicd.HelmList(ctx, dag, namespace)
		if err != nil {
			fail("Helm chart version matches expected ("+expectedVersion+")",
				fmt.Sprintf("helm list failed: %s", err))
		} else {
			var releases []map[string]interface{}
			if json.Unmarshal([]byte(helmListJSON), &releases) == nil {
				found := false
				for _, r := range releases {
					if r["name"] == releaseName {
						found = true
						chartField, _ := r["chart"].(string)
						// chart field is e.g. "goserv-1.2.3"
						chartVersion := strings.TrimPrefix(chartField, releaseName+"-")
						if chartVersion == expectedVersion {
							pass("Helm chart version matches expected (" + expectedVersion + ")")
						} else {
							fail("Helm chart version matches expected ("+expectedVersion+")",
								"deployed version is: "+chartVersion)
						}
						break
					}
				}
				if !found {
					fail("Helm chart version matches expected ("+expectedVersion+")",
						"release "+releaseName+" not found in helm list output")
				}
			}
		}
	}

	// -----------------------------------------------------------------------
	// 2–4. Fetch all namespace resources in one call
	// -----------------------------------------------------------------------
	allJSON, err := cicd.KubectlGetAll(ctx, dag, namespace)

	// Helper: extract items from the KubectlGetAll response by kind.
	itemsByKind := func(kind string) []map[string]interface{} {
		if err != nil || allJSON == "" {
			return nil
		}
		var list map[string]interface{}
		if json.Unmarshal([]byte(allJSON), &list) != nil {
			return nil
		}
		items, _ := list["items"].([]interface{})
		var out []map[string]interface{}
		for _, item := range items {
			obj, _ := item.(map[string]interface{})
			if obj == nil {
				continue
			}
			if k, _ := obj["kind"].(string); k == kind {
				out = append(out, obj)
			}
		}
		return out
	}

	// Helper: find a single resource by kind + metadata.name.
	findResource := func(kind, name string) map[string]interface{} {
		for _, obj := range itemsByKind(kind) {
			meta, _ := obj["metadata"].(map[string]interface{})
			if n, _ := meta["name"].(string); n == name {
				return obj
			}
		}
		return nil
	}

	if err != nil {
		fail("Fetch namespace resources", err.Error())
	}

	// -----------------------------------------------------------------------
	// 2. Deployment ready replicas (retry up to 60s, checking every 10s)
	// -----------------------------------------------------------------------
	const deployReadyTimeout = 60 * time.Second
	const deployReadyInterval = 10 * time.Second

	deployCheckStart := time.Now()
	for {
		currentAllJSON, fetchErr := cicd.KubectlGetAll(ctx, dag, namespace)

		currentFindResource := func(kind, name string) map[string]interface{} {
			if fetchErr != nil || currentAllJSON == "" {
				return nil
			}
			var list map[string]interface{}
			if json.Unmarshal([]byte(currentAllJSON), &list) != nil {
				return nil
			}
			items, _ := list["items"].([]interface{})
			for _, item := range items {
				obj, _ := item.(map[string]interface{})
				if obj == nil {
					continue
				}
				if k, _ := obj["kind"].(string); k != kind {
					continue
				}
				meta, _ := obj["metadata"].(map[string]interface{})
				if n, _ := meta["name"].(string); n == name {
					return obj
				}
			}
			return nil
		}

		deploy := currentFindResource("Deployment", releaseName)
		if deploy == nil {
			if time.Since(deployCheckStart) >= deployReadyTimeout {
				fail("Deployment exists", "Deployment '"+releaseName+"' not found in namespace '"+namespace+"' after 60s")
				break
			}
			fmt.Printf("Deployment '%s' not found yet, retrying in %s...\n", releaseName, deployReadyInterval)
			time.Sleep(deployReadyInterval)
			continue
		}

		spec, _ := deploy["spec"].(map[string]interface{})
		status, _ := deploy["status"].(map[string]interface{})
		desired := int(jsonFloat(spec, "replicas"))
		ready := int(jsonFloat(status, "readyReplicas"))

		if ready > 0 && ready == desired {
			pass("Deployment exists")
			pass(fmt.Sprintf("Deployment has correct number of ready replicas (%d/%d)", ready, desired))
			break
		}

		if time.Since(deployCheckStart) >= deployReadyTimeout {
			pass("Deployment exists")
			fail("Deployment has correct number of ready replicas",
				fmt.Sprintf("timed out after 60s: ready: %d, desired: %d", ready, desired))
			break
		}

		fmt.Printf("Deployment not yet ready (%d/%d replicas), waiting %s (%.0fs elapsed)...\n",
			ready, desired, deployReadyInterval, time.Since(deployCheckStart).Seconds())
		time.Sleep(deployReadyInterval)
	}

	// -----------------------------------------------------------------------
	// 3. Pods running and ready (retry up to 60s, checking every 10s)
	// -----------------------------------------------------------------------
	const podReadyTimeout = 60 * time.Second
	const podReadyInterval = 10 * time.Second

	var matchingPods []map[string]interface{}
	podCheckStart := time.Now()
	for {
		// Re-fetch namespace resources so we get the latest pod status
		currentAllJSON, fetchErr := cicd.KubectlGetAll(ctx, dag, namespace)

		currentItemsByKind := func(kind string) []map[string]interface{} {
			if fetchErr != nil || currentAllJSON == "" {
				return nil
			}
			var list map[string]interface{}
			if json.Unmarshal([]byte(currentAllJSON), &list) != nil {
				return nil
			}
			items, _ := list["items"].([]interface{})
			var out []map[string]interface{}
			for _, item := range items {
				obj, _ := item.(map[string]interface{})
				if obj == nil {
					continue
				}
				if k, _ := obj["kind"].(string); k == kind {
					out = append(out, obj)
				}
			}
			return out
		}

		matchingPods = nil
		for _, pod := range currentItemsByKind("Pod") {
			meta, _ := pod["metadata"].(map[string]interface{})
			labels, _ := meta["labels"].(map[string]interface{})
			appName, _ := labels["app.kubernetes.io/name"].(string)
			appInstance, _ := labels["app.kubernetes.io/instance"].(string)
			if appName == "goserv" && appInstance == releaseName {
				matchingPods = append(matchingPods, pod)
			}
		}

		if len(matchingPods) == 0 {
			if time.Since(podCheckStart) >= podReadyTimeout {
				fail("Pods exist", "no pods found with label app.kubernetes.io/instance="+releaseName)
				break
			}
			fmt.Printf("No matching pods found yet, retrying in %s...\n", podReadyInterval)
			time.Sleep(podReadyInterval)
			continue
		}

		// Check whether all matching pods are ready
		allReady := true
		for _, pod := range matchingPods {
			meta, _ := pod["metadata"].(map[string]interface{})
			podName, _ := meta["name"].(string)
			podStatus, _ := pod["status"].(map[string]interface{})
			conditions, _ := podStatus["conditions"].([]interface{})
			readyCond := false
			for _, c := range conditions {
				cond, _ := c.(map[string]interface{})
				if cond["type"] == "Ready" && cond["status"] == "True" {
					readyCond = true
				}
			}
			if !readyCond {
				allReady = false
				fmt.Printf("Pod %s is not yet Ready, will retry...\n", podName)
			}
		}

		if allReady {
			pass(fmt.Sprintf("Pods exist (%d found)", len(matchingPods)))
			allRunning := true
			for _, pod := range matchingPods {
				meta, _ := pod["metadata"].(map[string]interface{})
				podName, _ := meta["name"].(string)
				podStatus, _ := pod["status"].(map[string]interface{})
				phase, _ := podStatus["phase"].(string)
				if phase != "Running" {
					allRunning = false
					fail("Pod "+podName+" is Running", "phase is: "+phase)
				}
			}
			if allRunning {
				pass("All pods are Running")
			}
			pass("All pods are Ready")
			break
		}

		if time.Since(podCheckStart) >= podReadyTimeout {
			pass(fmt.Sprintf("Pods exist (%d found)", len(matchingPods)))
			for _, pod := range matchingPods {
				meta, _ := pod["metadata"].(map[string]interface{})
				podName, _ := meta["name"].(string)
				podStatus, _ := pod["status"].(map[string]interface{})
				conditions, _ := podStatus["conditions"].([]interface{})
				readyCond := false
				for _, c := range conditions {
					cond, _ := c.(map[string]interface{})
					if cond["type"] == "Ready" && cond["status"] == "True" {
						readyCond = true
					}
				}
				if !readyCond {
					fail("Pod "+podName+" is Ready", "timed out after 60s waiting for Ready condition")
				}
			}
			break
		}

		fmt.Printf("Not all pods are ready, waiting %s before retrying (%.0fs elapsed)...\n",
			podReadyInterval, time.Since(podCheckStart).Seconds())
		time.Sleep(podReadyInterval)
	}

	// -----------------------------------------------------------------------
	// 4. Service exists and has endpoints
	// -----------------------------------------------------------------------
	svc := findResource("Service", releaseName)
	if svc == nil {
		fail("Service exists", "Service '"+releaseName+"' not found in namespace '"+namespace+"'")
	} else {
		pass("Service exists")
		// `kubectl get all` does not return Endpoints objects, so use KubectlGet
		// for the endpoints specifically.
		epJSON, epErr := cicd.KubectlGet(ctx, dag, namespace, "endpoints/"+releaseName)
		if epErr != nil {
			fail("Service has endpoints", epErr.Error())
		} else {
			var ep map[string]interface{}
			epCount := 0
			if json.Unmarshal([]byte(epJSON), &ep) == nil {
				if subsets, ok := ep["subsets"].([]interface{}); ok {
					for _, s := range subsets {
						subset, _ := s.(map[string]interface{})
						addrs, _ := subset["addresses"].([]interface{})
						epCount += len(addrs)
					}
				}
			}
			if epCount > 0 {
				pass(fmt.Sprintf("Service has endpoints (%d)", epCount))
			} else {
				fail("Service has endpoints", "no endpoint addresses found")
			}
		}
	}

	// -----------------------------------------------------------------------
	// 5. HTTP endpoint checks via port-forward
	// -----------------------------------------------------------------------
	portForwardSvc, err := cicd.KubectlPortForward(ctx, dag, namespace, "svc/"+releaseName, "8080:80")
	if err != nil {
		fail("Port-forward to service", err.Error())
	} else {
		// Eagerly start the service so Dagger confirms it is healthy before
		// we send any HTTP requests.
		startedSvc, startErr := portForwardSvc.Start(ctx)
		if startErr != nil {
			fail("Port-forward to service", fmt.Sprintf("service start failed: %s", startErr))
			goto afterHTTP
		}
		pass("Port-forward to service")

		curlBase := dag.Container().
			From("curlimages/curl:latest").
			WithServiceBinding("app", startedSvc)

		httpChecks := []struct {
			path        string
			wantCode    string
			extraChecks func(body string) []checkResult
		}{
			{
				path:     "/health",
				wantCode: "200",
				extraChecks: func(body string) []checkResult {
					var r []checkResult
					var data map[string]interface{}
					if json.Unmarshal([]byte(body), &data) == nil {
						if data["status"] == "healthy" {
							r = append(r, checkResult{Name: "/health returns status=healthy", Passed: true})
						} else {
							r = append(r, checkResult{Name: "/health returns status=healthy", Passed: false,
								Details: fmt.Sprintf("status field is: %v", data["status"])})
						}
					}
					return r
				},
			},
			{
				path:     "/ready",
				wantCode: "200",
				extraChecks: func(body string) []checkResult {
					var r []checkResult
					var data map[string]interface{}
					if json.Unmarshal([]byte(body), &data) == nil {
						if data["status"] == "ready" {
							r = append(r, checkResult{Name: "/ready returns status=ready", Passed: true})
						} else {
							r = append(r, checkResult{Name: "/ready returns status=ready", Passed: false,
								Details: fmt.Sprintf("status field is: %v", data["status"])})
						}
					}
					return r
				},
			},
			{
				path:     "/",
				wantCode: "200",
				extraChecks: func(body string) []checkResult {
					var r []checkResult
					var data map[string]interface{}
					if json.Unmarshal([]byte(body), &data) != nil {
						return append(r, checkResult{Name: "Root endpoint returns valid JSON", Passed: false, Details: "response is not valid JSON"})
					}
					r = append(r, checkResult{Name: "Root endpoint returns valid JSON", Passed: true})
					requiredFields := []string{"service_name", "service_version", "ip_address", "instance_uuid", "timestamp"}
					for _, field := range requiredFields {
						v, _ := data[field].(string)
						if v != "" {
							if field == "instance_uuid" {
								uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
								if uuidRe.MatchString(v) {
									r = append(r, checkResult{Name: "Root endpoint includes valid UUID", Passed: true})
								} else {
									r = append(r, checkResult{Name: "Root endpoint includes valid UUID", Passed: false, Details: "invalid format: " + v})
								}
							} else {
								r = append(r, checkResult{Name: "Root endpoint includes " + field, Passed: true})
							}
						} else {
							r = append(r, checkResult{Name: "Root endpoint includes " + field, Passed: false, Details: "field missing or null"})
						}
					}
					return r
				},
			},
			{
				path:     "/invalid-path",
				wantCode: "404",
			},
		}

		for _, hc := range httpChecks {
			// curl: write HTTP status code on last line, body on preceding lines
			out, err := curlBase.
				WithExec([]string{
					"curl", "-s", "-w", "\n%{http_code}", "-m", "5",
					"http://app:8080" + hc.path,
				}).
				Stdout(ctx)
			if err != nil {
				fail(hc.path+" endpoint reachable", err.Error())
				continue
			}
			lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
			code := lines[len(lines)-1]
			body := strings.Join(lines[:len(lines)-1], "\n")

			checkName := hc.path + " responds with " + hc.wantCode
			if code == hc.wantCode {
				pass(checkName)
				if hc.extraChecks != nil {
					checks = append(checks, hc.extraChecks(body)...)
				}
			} else {
				fail(checkName, "got HTTP "+code)
			}
		}

		// Explicitly stop the port-forward service so kubectl port-forward
		// terminates and Dagger can release the container.
		_, _ = startedSvc.Stop(ctx)
	}
afterHTTP:

	// -----------------------------------------------------------------------
	// 6. Pod log error scan
	// -----------------------------------------------------------------------
	firstPod := ""
	if len(matchingPods) > 0 {
		meta, _ := matchingPods[0]["metadata"].(map[string]interface{})
		firstPod, _ = meta["name"].(string)
	}
	if firstPod == "" {
		fail("Pod logs are accessible", "could not resolve a pod name")
	} else {
		logs, err := cicd.KubectlLogs(ctx, dag, namespace, firstPod, 100)
		if err != nil {
			fail("Pod logs are accessible", err.Error())
		} else {
			pass("Pod logs are accessible")
			errorRe := regexp.MustCompile(`(?i)(error|fatal|panic)`)
			errorLines := 0
			for _, line := range strings.Split(logs, "\n") {
				if errorRe.MatchString(line) {
					errorLines++
				}
			}
			if errorLines == 0 {
				pass("Pod logs contain no errors")
			} else {
				fail("Pod logs contain no errors",
					fmt.Sprintf("found %d error-like lines in last 100 log lines", errorLines))
			}
		}
	}

	// -----------------------------------------------------------------------
	// Build output
	// -----------------------------------------------------------------------
	totalPassed, totalFailed := 0, 0
	for _, c := range checks {
		if c.Passed {
			totalPassed++
		} else {
			totalFailed++
		}
	}

	overallStatus := "healthy"
	if totalFailed > 0 {
		overallStatus = "failed"
	}

	validationContext := map[string]interface{}{
		"timestamp":       time.Now().Format(time.RFC3339),
		"releaseName":     releaseName,
		"namespace":       namespace,
		"endpoint":        endpoint,
		"expectedVersion": expectedVersion,
		"status":          overallStatus,
		"totalTests":      totalPassed + totalFailed,
		"passed":          totalPassed,
		"failed":          totalFailed,
		"checks":          checks,
	}

	contextJSON, err := json.MarshalIndent(validationContext, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal validation context: %w", err)
	}

	if totalFailed > 0 {
		return nil, fmt.Errorf("validation failed: %d/%d checks failed — see validationContext for details\n%s",
			totalFailed, totalPassed+totalFailed, string(contextJSON))
	}

	return dag.Directory().
		WithNewFile("validationContext", string(contextJSON)).
		File("validationContext"), nil
}

// jsonFloat safely extracts a float64 from a map[string]interface{} by key.
func jsonFloat(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	v, _ := m[key].(float64)
	return v
}
