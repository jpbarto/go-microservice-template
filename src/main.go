package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	DependencyURL  string
	Port           string
}

type Response struct {
	ServiceName       string              `json:"service_name"`
	ServiceVersion    string              `json:"service_version"`
	IPAddress         string              `json:"ip_address"`
	InstanceUUID      string              `json:"instance_uuid"`
	DependencyHeaders map[string][]string `json:"dependency_headers,omitempty"`
	Timestamp         string              `json:"timestamp"`
}

var (
	config       Config
	instanceUUID string
	version      = "dev" // Set via ldflags at build time
)

func init() {
	// Generate a UUID for this instance
	instanceUUID = uuid.New().String()

	// Load configuration from environment variables
	config = Config{
		ServiceName:    getEnv("SERVICE_NAME", "goserv"),
		ServiceVersion: getEnv("SERVICE_VERSION", version),
		DependencyURL:  getEnv("DEPENDENCY_URL", ""),
		Port:           getEnv("PORT", "8080"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getOutboundIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "unknown"
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}

func callDependency() (map[string][]string, error) {
	if config.DependencyURL == "" {
		return nil, nil
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(config.DependencyURL)
	if err != nil {
		return nil, fmt.Errorf("failed to call dependency: %w", err)
	}
	defer resp.Body.Close()

	return resp.Header, nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	response := Response{
		ServiceName:    config.ServiceName,
		ServiceVersion: config.ServiceVersion,
		IPAddress:      getOutboundIP(),
		InstanceUUID:   instanceUUID,
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
	}

	// Call dependency if configured
	if config.DependencyURL != "" {
		headers, err := callDependency()
		if err != nil {
			log.Printf("Error calling dependency: %v", err)
			// Continue without dependency headers
		} else {
			response.DependencyHeaders = headers
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

func main() {
	http.HandleFunc("/", rootHandler)
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/ready", readyHandler)

	addr := ":" + config.Port
	log.Printf("Starting %s v%s on %s (Instance: %s)",
		config.ServiceName, config.ServiceVersion, addr, instanceUUID)

	if config.DependencyURL != "" {
		log.Printf("Dependency URL configured: %s", config.DependencyURL)
	}

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
