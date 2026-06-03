// Command healthcheck is a tiny dependency-free readiness probe for the backend
// container. It performs a single local GET against the server's /ready endpoint
// on 127.0.0.1 and exits 0 on HTTP 200, non-zero otherwise. It is invoked by the
// Docker Compose `healthcheck` for backend-prod and backend-staging.
//
// It is deliberately minimal: it makes no external network calls (loopback only),
// requires no secrets, and is silent on success — it never prints request or
// response bodies, headers, or tokens. The target port is read from the same PORT
// env var the server listens on, so the probe always tracks the server.
package main

import (
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

const (
	envVarPort   = "PORT"
	defaultPort  = "8080"
	loopbackHost = "127.0.0.1"
	readyPath    = "/ready"
	probeTimeout = 2 * time.Second

	minPort = 1
	maxPort = 65535
)

func main() {
	port := os.Getenv(envVarPort)
	if port == "" {
		port = defaultPort
	}

	// Validate PORT before building the URL so a malformed value fails the probe
	// (exit 1) rather than producing a request to an unexpected host:port.
	if portNum, err := strconv.Atoi(port); err != nil || portNum < minPort || portNum > maxPort {
		os.Exit(1)
	}

	url := "http://" + net.JoinHostPort(loopbackHost, port) + readyPath
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(url)
	if err != nil {
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Exit(1)
	}
}
