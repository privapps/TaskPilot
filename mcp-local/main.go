package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const VERSION = "1.0.0"

func main() {
	// Parse command-line flags
	apiURL := flag.String("url", "", "TaskPilot API server URL (default: http://localhost:8080)")
	flag.Parse()

	// Configuration precedence: flag > env var > default
	if *apiURL == "" {
		*apiURL = os.Getenv("TASKPILOT_API_URL")
	}
	if *apiURL == "" {
		*apiURL = "http://localhost:8080"
	}

	// Open debug log file - Log ONLY to file, not stderr, to avoid polluting copilot CLI stderr
	// if it uses stderr for anything other than logs. 
	// The problem is likely that Copilot CLI doesn't separate stderr well or blocks on it.
	// So we log ONLY to a file.
	// DEBUG: Disabled by default for production
	var logOutput io.Writer = io.Discard
	if os.Getenv("MCP_DEBUG") == "true" {
		logFile, err := os.OpenFile(os.Getenv("HOME")+"/.taskpilot-mcp.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			logOutput = logFile
			defer logFile.Close()
		}
	}

	// Set logs to file only
	log.SetOutput(logOutput)
	log.SetPrefix("[mcp-local] ")
	log.SetFlags(log.Ldate | log.Ltime)

	log.Printf("TaskPilot MCP Proxy v%s started (API: %s)", VERSION, *apiURL)

	// Create HTTP client with 30-second timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Create stdin scanner for newline-delimited JSON
	scanner := bufio.NewScanner(os.Stdin)

	// Channel to coordinate shutdown
	done := make(chan bool)

	// Start processing loop in goroutine
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			
			// Log incoming request
			if logOutput != io.Discard {
				fmt.Fprintf(logOutput, "[IN] %s\n", line)
			}
			
			// Check if this is a notification (no id field) - per JSON-RPC 2.0 spec
			var reqCheck map[string]interface{}
			isNotification := false
			if err := json.Unmarshal([]byte(line), &reqCheck); err == nil {
				_, hasID := reqCheck["id"]
				isNotification = !hasID
			}
			
			// Forward request to API server
			response := forwardRequest(client, *apiURL, line)
			
			// For notifications, don't output a response per JSON-RPC 2.0 spec
			if isNotification {
				if logOutput != io.Discard {
					fmt.Fprintf(logOutput, "[NOTIFICATION] No response sent for notification\n")
				}
				continue
			}
			
			// Log outgoing response
			if logOutput != io.Discard {
				fmt.Fprintf(logOutput, "[OUT] %s\n", response)
			}
			
			// Write response to stdout
			fmt.Println(response)
		}

		// Check for scanner errors (not EOF)
		if err := scanner.Err(); err != nil {
			log.Printf("Error reading stdin: %v", err)
			writeErrorResponse("", -32700, "Parse error")
		}

		done <- true
	}()

	// Wait for shutdown signal or stdin EOF
	select {
	case <-sigChan:
		log.Println("Received interrupt signal, shutting down gracefully")
	case <-done:
		log.Println("Stdin closed, shutting down gracefully")
	}
}

// forwardRequest sends a JSON-RPC request to the API server and returns the response
func forwardRequest(client *http.Client, apiURL, request string) string {
	// Validate JSON
	var js json.RawMessage
	if err := json.Unmarshal([]byte(request), &js); err != nil {
		return createErrorResponse("", -32700, "Parse error: invalid JSON")
	}

	// Create HTTP POST request
	mcpEndpoint := apiURL + "/api/mcp"
	req, err := http.NewRequestWithContext(context.Background(), "POST", mcpEndpoint, bytes.NewBufferString(request))
	if err != nil {
		log.Printf("Error creating HTTP request: %v", err)
		return createErrorResponse("", -32603, "Internal error")
	}

	req.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("API request failed: %v", err)
		return createErrorResponse("", -32603, "API server not available")
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Error reading response: %v", err)
		return createErrorResponse("", -32603, "Error reading API response")
	}

	return string(body)
}

// createErrorResponse generates a JSON-RPC error response
func createErrorResponse(id string, code int, message string) string {
	// Handle null ID
	if id == "" {
		id = "null"
	}

	errorResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":{"code":%d,"message":"%s"}}`, id, code, message)
	return errorResp
}

// writeErrorResponse writes an error to stdout (helper for unrecoverable errors)
func writeErrorResponse(id string, code int, message string) {
	if id == "" {
		id = "null"
	}
	errorResp := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":{"code":%d,"message":"%s"}}`, id, code, message)
	fmt.Println(errorResp)
}
