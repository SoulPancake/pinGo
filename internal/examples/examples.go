package examples

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/SoulPancake/pinGo/pkg/core"
	"github.com/SoulPancake/pinGo/pkg/errors"
	"github.com/SoulPancake/pinGo/pkg/proxy"
)

type EchoService struct {
	name      string
	listeners []net.Listener
	server    *http.Server
}

func NewEchoService(name string) *EchoService {
	return &EchoService{
		name:      name,
		listeners: make([]net.Listener, 0),
	}
}

func (e *EchoService) Name() string {
	return e.name
}

func (e *EchoService) AddTCP(addr string) *errors.Error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return errors.NewWithCause(errors.ConnectionError, fmt.Sprintf("failed to listen on %s", addr), err)
	}

	e.listeners = append(e.listeners, listener)
	return nil
}

func (e *EchoService) AddTLS(addr, certFile, keyFile string) *errors.Error {
	return errors.New(errors.InternalError, "TLS not implemented in echo service")
}

func (e *EchoService) AddUDS(path string, mode *os.FileMode) *errors.Error {
	listener, err := net.Listen("unix", path)
	if err != nil {
		return errors.NewWithCause(errors.ConnectionError, fmt.Sprintf("failed to listen on %s", path), err)
	}

	e.listeners = append(e.listeners, listener)
	return nil
}

func (e *EchoService) Start(ctx context.Context) *errors.Error {
	if len(e.listeners) == 0 {
		return errors.New(errors.InternalError, "no listeners configured")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", e.echoHandler)
	mux.HandleFunc("/health", e.healthHandler)

	e.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	for _, listener := range e.listeners {
		go func(l net.Listener) {
			if err := e.server.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("Echo service %s failed to serve: %v", e.name, err)
			}
		}(listener)
	}

	return nil
}

func (e *EchoService) Stop(ctx context.Context) *errors.Error {
	if e.server != nil {
		if err := e.server.Shutdown(ctx); err != nil {
			return errors.NewWithCause(errors.InternalError, "failed to shutdown server", err)
		}
	}

	for _, listener := range e.listeners {
		if err := listener.Close(); err != nil {
			log.Printf("Failed to close listener: %v", err)
		}
	}

	return nil
}

func (e *EchoService) echoHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Service", e.name)

	fmt.Fprintf(w, `{
  "method": "%s",
  "path": "%s",
  "query": "%s",
  "headers": {`, r.Method, r.URL.Path, r.URL.RawQuery)

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
  "remote_addr": "%s",
  "timestamp": "%s"
}`, r.RemoteAddr, time.Now().Format(time.RFC3339))
}

func (e *EchoService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status": "ok", "service": "%s", "timestamp": "%s"}`, 
		e.name, time.Now().Format(time.RFC3339))
}

func LoadBalancerExample() {
	fmt.Println("=== Load Balancer Example ===")

	config := &proxy.ProxyConfig{
		Upstreams: []string{
			"httpbin.org:80",
			"httpbingo.org:80",
			"postman-echo.com:80",
		},
		LoadBalancingMethod: "round_robin",
	}

	proxyService, err := proxy.NewProxyService("load-balancer", config)
	if err != nil {
		log.Fatalf("Failed to create proxy service: %v", err)
	}

	if err := proxyService.AddTCP(":8080"); err != nil {
		log.Fatalf("Failed to add TCP listener: %v", err)
	}

	server := core.NewServer(nil)
	server.AddService(proxyService)

	fmt.Println("Load balancer listening on :8080")
	fmt.Println("Upstreams: httpbin.org, httpbingo.org, postman-echo.com")
	fmt.Println("Try: curl http://localhost:8080/get")

	if err := server.RunForever(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func EchoServerExample() {
	fmt.Println("=== Echo Server Example ===")

	echoService := NewEchoService("echo-server")
	if err := echoService.AddTCP(":8080"); err != nil {
		log.Fatalf("Failed to add TCP listener: %v", err)
	}

	server := core.NewServer(nil)
	server.AddService(echoService)

	fmt.Println("Echo server listening on :8080")
	fmt.Println("Try: curl http://localhost:8080/")
	fmt.Println("Health check: curl http://localhost:8080/health")

	if err := server.RunForever(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func MultiServiceExample() {
	fmt.Println("=== Multi-Service Example ===")

	echoService1 := NewEchoService("echo-1")
	if err := echoService1.AddTCP(":8080"); err != nil {
		log.Fatalf("Failed to add TCP listener for echo-1: %v", err)
	}

	echoService2 := NewEchoService("echo-2")
	if err := echoService2.AddTCP(":8081"); err != nil {
		log.Fatalf("Failed to add TCP listener for echo-2: %v", err)
	}

	proxyConfig := &proxy.ProxyConfig{
		Upstreams: []string{"httpbin.org:80"},
	}
	proxyService, err := proxy.NewProxyService("proxy", proxyConfig)
	if err != nil {
		log.Fatalf("Failed to create proxy service: %v", err)
	}

	if err := proxyService.AddTCP(":8082"); err != nil {
		log.Fatalf("Failed to add TCP listener for proxy: %v", err)
	}

	server := core.NewServer(nil)
	server.AddServices([]core.Service{
		echoService1,
		echoService2,
		proxyService,
	})

	fmt.Println("Services running:")
	fmt.Println("  Echo-1: http://localhost:8080/")
	fmt.Println("  Echo-2: http://localhost:8081/")
	fmt.Println("  Proxy:  http://localhost:8082/ -> httpbin.org")

	if err := server.RunForever(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}