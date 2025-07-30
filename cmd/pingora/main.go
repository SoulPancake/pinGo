package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/SoulPancake/pinGo/pkg/core"
	"github.com/SoulPancake/pinGo/pkg/proxy"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "proxy":
		runProxy()
	case "server":
		runServer()
	case "client":
		runClient()
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`pinGo - Pingora implementation in Go

Usage:
  pingora proxy   - Run HTTP proxy server
  pingora server  - Run HTTP server examples
  pingora client  - Run HTTP client examples

Examples:
  pingora proxy                    # Run proxy on :8080 -> httpbin.org:80
  PORT=8080 TARGET=httpbin.org:80 pingora proxy
  pingora server                   # Run echo server on :8080
  pingora client                   # Make HTTP requests`)
}

func runProxy() {
	listenAddr := getEnv("LISTEN", ":8080")
	targetAddr := getEnv("TARGET", "httpbin.org:80")

	fmt.Printf("Starting proxy server...\n")
	fmt.Printf("Listen: %s\n", listenAddr)
	fmt.Printf("Target: %s\n", targetAddr)

	server := core.NewServer(nil)
	
	proxyService := proxy.SimpleProxyService(listenAddr, targetAddr)
	server.AddService(proxyService)

	if err := server.RunForever(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func runServer() {
	listenAddr := getEnv("LISTEN", ":8080")

	fmt.Printf("Starting echo server on %s\n", listenAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", echoHandler)
	mux.HandleFunc("/health", healthHandler)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
	}

	log.Fatal(server.ListenAndServe())
}

func runClient() {
	targetURL := getEnv("URL", "http://httpbin.org/get")

	fmt.Printf("Making request to %s\n", targetURL)

	resp, err := http.Get(targetURL)
	if err != nil {
		log.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Headers:\n")
	for key, values := range resp.Header {
		for _, value := range values {
			fmt.Printf("  %s: %s\n", key, value)
		}
	}
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	fmt.Fprintf(w, `{
  "method": "%s",
  "path": "%s",
  "headers": {`, r.Method, r.URL.Path)

	first := true
	for key, values := range r.Header {
		for _, value := range values {
			if !first {
				fmt.Fprintf(w, ",")
			}
			fmt.Fprintf(w, `
    "%s": "%s"`, key, value)
			first = false
		}
	}

	fmt.Fprintf(w, `
  },
  "remote_addr": "%s"
}`, r.RemoteAddr)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "service": "pinGo"}`)
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}