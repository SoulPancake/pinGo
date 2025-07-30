# pinGo

![pinGo banner](https://img.shields.io/badge/pinGo-Pingora%20in%20Go-blue?style=for-the-badge)

**pinGo** is a comprehensive Go implementation of Cloudflare's Pingora proxy framework. It provides fast, reliable, and programmable networking services with extensive customization capabilities.

## Features

### Core Features
- **HTTP/1.x and HTTP/2 Support**: Complete HTTP protocol implementation
- **TLS/SSL Support**: Secure connections with modern TLS configurations
- **Load Balancing**: Multiple algorithms including round-robin, random, and least-connection
- **Connection Pooling**: Efficient connection management and reuse
- **Caching**: In-memory caching with LRU eviction and cache stampede protection
- **Rate Limiting**: Token bucket, sliding window, and fixed window limiters
- **Circuit Breaker**: Fault tolerance and resilience patterns
- **Consistent Hashing**: Ketama algorithm for distributed systems
- **Graceful Shutdown**: Zero-downtime restarts and upgrades

### Advanced Features
- **Concurrent Safety**: Thread-safe operations across all components
- **Observability**: Built-in metrics and health checking
- **Timeout Management**: Comprehensive timeout and deadline handling
- **Error Handling**: Robust error types with HTTP status code mapping
- **Service Abstraction**: Pluggable service architecture

## Quick Start

### Installation

```bash
git clone https://github.com/SoulPancake/pinGo.git
cd pinGo
go build -o bin/pingora ./cmd/pingora
```

### Basic Usage

**Run a simple HTTP proxy:**
```bash
./bin/pingora proxy
# Proxies :8080 -> httpbin.org:80
```

**Run an echo server:**
```bash
./bin/pingora server
# Starts echo server on :8080
```

**Make HTTP requests:**
```bash
./bin/pingora client
# Makes requests to httpbin.org
```

### Custom Configuration

```bash
# Custom proxy configuration
LISTEN=:9000 TARGET=example.com:80 ./bin/pingora proxy

# Custom server configuration  
LISTEN=:9000 ./bin/pingora server

# Custom client target
URL=https://api.github.com ./bin/pingora client
```

## Architecture

pinGo follows a modular architecture mirroring Cloudflare's Pingora:

```
pinGo/
├── cmd/pingora/          # Main application entry point
├── pkg/
│   ├── core/            # Core server and service abstractions
│   ├── proxy/           # HTTP proxy implementation
│   ├── http/            # HTTP utilities and header handling
│   ├── tls/             # TLS/SSL configuration and management
│   ├── loadbalancing/   # Load balancing algorithms
│   ├── cache/           # Caching layer with LRU and tiered caching
│   ├── pool/            # Connection pooling
│   ├── ketama/          # Consistent hashing implementation
│   ├── limits/          # Rate limiting and concurrency control
│   ├── timeout/         # Timeout and circuit breaker utilities
│   └── errors/          # Error handling and Result types
├── internal/examples/   # Example implementations
└── tests/              # Integration and benchmark tests
```

## Package Documentation

### Core Components

#### `pkg/core` - Server and Service Framework
```go
import "github.com/SoulPancake/pinGo/pkg/core"

// Create a new server
server := core.NewServer(nil)

// Add services
server.AddService(myService)

// Run forever with graceful shutdown
server.RunForever()
```

#### `pkg/proxy` - HTTP Proxy Services
```go
import "github.com/SoulPancake/pinGo/pkg/proxy"

// Create proxy with load balancing
config := &proxy.ProxyConfig{
    Upstreams: []string{"server1:80", "server2:80"},
    LoadBalancingMethod: "round_robin",
}

proxyService, err := proxy.NewProxyService("my-proxy", config)
proxyService.AddTCP(":8080")
```

#### `pkg/loadbalancing` - Load Balancing Algorithms
```go
import "github.com/SoulPancake/pinGo/pkg/loadbalancing"

// Round-robin balancer
balancer := loadbalancing.NewRoundRobin([]string{"server1:80", "server2:80"})
backend, err := balancer.SelectBackend()

// Least-connection balancer
balancer = loadbalancing.NewLeastConnection([]string{"server1:80", "server2:80"})
backend, err = balancer.SelectBackend()
```

#### `pkg/cache` - Caching Layer
```go
import "github.com/SoulPancake/pinGo/pkg/cache"

// Memory cache with LRU eviction
cache := cache.NewMemoryCache(&cache.MemoryCacheConfig{
    MaxSize:  100 * 1024 * 1024, // 100MB
    MaxItems: 10000,
})

err := cache.Set(ctx, "key", []byte("value"), time.Hour)
value, err := cache.Get(ctx, "key")
```

#### `pkg/ketama` - Consistent Hashing
```go
import "github.com/SoulPancake/pinGo/pkg/ketama"

// Create consistent hash ring
balancer := ketama.NewKetamaBalancer([]string{"server1:80", "server2:80"}, 160)
backend, err := balancer.SelectBackend("user123")
```

#### `pkg/limits` - Rate Limiting
```go
import "github.com/SoulPancake/pinGo/pkg/limits"

// Token bucket rate limiter
limiter := limits.NewTokenBucket(100, 10) // 100 capacity, 10/sec refill
if limiter.Allow() {
    // Process request
}

// Sliding window rate limiter
limiter := limits.NewSlidingWindowLimiter(1000, time.Minute)
err := limiter.Wait(ctx) // Wait for token
```

#### `pkg/timeout` - Timeout and Circuit Breakers
```go
import "github.com/SoulPancake/pinGo/pkg/timeout"

// Circuit breaker
circuit := timeout.NewCircuitBreaker(5, time.Minute)
err := circuit.Call(ctx, func(ctx context.Context) error {
    // Your operation
    return nil
})

// Retry with exponential backoff
err := timeout.WithRetry(ctx, timeout.DefaultRetryConfig(), func(ctx context.Context) error {
    // Your operation
    return nil
})
```

## Testing

pinGo includes comprehensive table-driven tests following Go best practices:

```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test ./... -v

# Run specific package tests
go test ./pkg/loadbalancing -v

# Run benchmarks
go test ./... -bench=.

# Generate coverage report
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Coverage

- **Unit Tests**: Comprehensive coverage of all packages
- **Integration Tests**: Full service testing
- **Benchmark Tests**: Performance validation
- **Concurrent Tests**: Race condition detection
- **Table-Driven Tests**: Extensive edge case coverage

## Examples

### Load Balancer with Health Checking

```go
package main

import (
    "github.com/SoulPancake/pinGo/pkg/core"
    "github.com/SoulPancake/pinGo/pkg/proxy"
)

func main() {
    config := &proxy.ProxyConfig{
        Upstreams: []string{
            "backend1.example.com:80",
            "backend2.example.com:80", 
            "backend3.example.com:80",
        },
        LoadBalancingMethod: "least_conn",
        HealthCheckInterval: 30 * time.Second,
    }
    
    proxyService, _ := proxy.NewProxyService("api-proxy", config)
    proxyService.AddTCP(":8080")
    
    server := core.NewServer(nil)
    server.AddService(proxyService)
    server.RunForever()
}
```

### Multi-Service Application

```go
// Run multiple services in one application
func main() {
    // API proxy
    apiProxy := createAPIProxy()
    
    // Static file server  
    staticServer := createStaticServer()
    
    // Admin interface
    adminServer := createAdminServer()
    
    server := core.NewServer(nil)
    server.AddServices([]core.Service{
        apiProxy,
        staticServer, 
        adminServer,
    })
    
    server.RunForever()
}
```

## Performance

pinGo is designed for high performance with:

- **Zero-copy operations** where possible
- **Connection pooling** for efficient resource usage
- **Concurrent-safe data structures** for multi-threaded performance
- **Memory-efficient caching** with configurable limits
- **Optimized load balancing** algorithms

### Benchmarks

```bash
# Run performance benchmarks
go test ./pkg/loadbalancing -bench=BenchmarkRoundRobin
go test ./pkg/cache -bench=BenchmarkMemoryCache  
go test ./pkg/ketama -bench=BenchmarkKetama
```

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

### Development Guidelines

- Follow Go best practices and conventions
- Write comprehensive tests for new features
- Use table-driven tests where appropriate
- Ensure thread safety for concurrent operations
- Add benchmark tests for performance-critical code
- Update documentation for API changes

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.

## Acknowledgments

- **Cloudflare Pingora**: Original framework inspiration
- **Go Community**: Excellent standard library and tooling
- **Contributors**: Thank you for your contributions

## Roadmap

- [ ] HTTP/3 and QUIC support
- [ ] gRPC proxying capabilities  
- [ ] WebSocket proxying
- [ ] Advanced observability (Prometheus metrics, distributed tracing)
- [ ] Plugin system for custom filters
- [ ] Configuration hot-reloading
- [ ] Kubernetes integration
- [ ] Docker support and container images

---

**pinGo** - Fast, reliable, and programmable networking in Go 🚀
