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
	kubeconfig, err := cicd.GetKubeconfigSecret(ctx, dag)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	helmStatusJSON, err := dag.Container().
		From("alpine/helm:latest").
		WithMountedSecret("/tmp/kubeconfig", kubeconfig, dagger.ContainerWithMountedSecretOpts{Mode: 0444}).
		WithEnvVariable("KUBECONFIG", "/tmp/kubeconfig").
		WithExec([]string{"helm", "status", releaseName, "-n", namespace, "-o", "json"}).
		Stdout(ctx)
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

		// version check via helm list
		if expectedVersion != "" {
			helmListJSON, err := dag.Container().
				From("alpine/helm:latest").
				WithMountedSecret("/tmp/kubeconfig", kubeconfig, dagger.ContainerWithMountedSecretOpts{Mode: 0444}).
				WithEnvVariable("KUBECONFIG", "/tmp/kubeconfig").
				WithExec([]string{"helm", "list", "-n", namespace, "-o", "json"}).
				Stdout(ctx)
			if err == nil {
				var releases []map[string]interface{}
				if json.Unmarshal([]byte(helmListJSON), &releases) == nil {
					for _, r := range releases {
						if r["name"] == releaseName {
							chartField, _ := r["chart"].(string)
							// chart field is e.g. "goserv-1.2.3"
							chartVersion := strings.TrimPrefix(chartField, releaseName+"-")
							if chartVersion == expectedVersion {
								pass("Helm chart version matches expected (" + expectedVersion + ")")
							} else {
								fail("Helm chart version matches expected ("+expectedVersion+")", "deployed version is: "+chartVersion)
							}
							break
						}
					}
				}
			}
		}
	}

	// -----------------------------------------------------------------------
	// 2. Deployment ready replicas
	// -----------------------------------------------------------------------
	deployJSON, err := cicd.KubectlGet(ctx, dag, namespace, "deployment/"+releaseName)
	if err != nil {
		fail("Deployment exists", err.Error())
	} else {
		pass("Deployment exists")
		var deploy map[string]interface{}
		if json.Unmarshal([]byte(deployJSON), &deploy) == nil {
			spec, _ := deploy["spec"].(map[string]interface{})
			status, _ := deploy["status"].(map[string]interface{})
			desired := int(jsonFloat(spec, "replicas"))
			ready := int(jsonFloat(status, "readyReplicas"))
			if ready > 0 && ready == desired {
				pass(fmt.Sprintf("Deployment has correct number of ready replicas (%d/%d)", ready, desired))
			} else {
				fail("Deployment has correct number of ready replicas",
					fmt.Sprintf("ready: %d, desired: %d", ready, desired))
			}
		}
	}

	// -----------------------------------------------------------------------
	// 3. Pods running and ready
	// -----------------------------------------------------------------------
	// Use a label selector via the pods resource with -l flag; KubectlGet uses
	// -o json which returns a List when given a resource type without a name.
	podsJSON, err := cicd.KubectlGet(ctx, dag, namespace,
		"pods -l app.kubernetes.io/name=goserv,app.kubernetes.io/instance="+releaseName)
	if err != nil {
		fail("Pods exist", err.Error())
	} else {
		var podList map[string]interface{}
		if json.Unmarshal([]byte(podsJSON), &podList) != nil {
			fail("Pods exist", "failed to parse pod list JSON")
		} else {
			items, _ := podList["items"].([]interface{})
			if len(items) == 0 {
				fail("Pods exist", "no pods found with label app.kubernetes.io/instance="+releaseName)
			} else {
				pass(fmt.Sprintf("Pods exist (%d found)", len(items)))
				allRunning, allReady := true, true
				for _, item := range items {
					pod, _ := item.(map[string]interface{})
					meta, _ := pod["metadata"].(map[string]interface{})
					podName, _ := meta["name"].(string)
					podStatus, _ := pod["status"].(map[string]interface{})
					phase, _ := podStatus["phase"].(string)
					if phase != "Running" {
						allRunning = false
						fail("Pod "+podName+" is Running", "phase is: "+phase)
					}
					conditions, _ := podStatus["conditions"].([]interface{})
					ready := false
					for _, c := range conditions {
						cond, _ := c.(map[string]interface{})
						if cond["type"] == "Ready" && cond["status"] == "True" {
							ready = true
						}
					}
					if !ready {
						allReady = false
						fail("Pod "+podName+" is Ready", "Ready condition is not True")
					}
				}
				if allRunning {
					pass("All pods are Running")
				}
				if allReady {
					pass("All pods are Ready")
				}
			}
		}
	}

	// -----------------------------------------------------------------------
	// 4. Service exists and has endpoints
	// -----------------------------------------------------------------------
	_, err = cicd.KubectlGet(ctx, dag, namespace, "svc/"+releaseName)
	if err != nil {
		fail("Service exists", err.Error())
	} else {
		pass("Service exists")
		epJSON, err := cicd.KubectlGet(ctx, dag, namespace, "endpoints/"+releaseName)
		if err != nil {
			fail("Service has endpoints", err.Error())
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
		pass("Port-forward to service")

		curlBase := dag.Container().
			From("curlimages/curl:latest").
			WithServiceBinding("app", portForwardSvc)

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
	}

	// -----------------------------------------------------------------------
	// 6. Pod log error scan
	// -----------------------------------------------------------------------
	// Resolve the first matching pod name from the list fetched in step 3.
	firstPod := ""
	if podsJSON != "" {
		var podList map[string]interface{}
		if json.Unmarshal([]byte(podsJSON), &podList) == nil {
			if items, ok := podList["items"].([]interface{}); ok && len(items) > 0 {
				pod, _ := items[0].(map[string]interface{})
				meta, _ := pod["metadata"].(map[string]interface{})
				firstPod, _ = meta["name"].(string)
			}
		}
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
