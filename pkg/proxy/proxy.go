package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/SoulPancake/pinGo/pkg/core"
	"github.com/SoulPancake/pinGo/pkg/errors"
	pingora_http "github.com/SoulPancake/pinGo/pkg/http"
	"github.com/SoulPancake/pinGo/pkg/loadbalancing"
	"github.com/SoulPancake/pinGo/pkg/pool"
)

type ProxyService struct {
	name        string
	upstreams   []string
	loadBalancer loadbalancing.LoadBalancer
	connPool    pool.ConnectionPool
	listeners   []net.Listener
	mu          sync.RWMutex
	server      *http.Server
}

type ProxyConfig struct {
	Upstreams           []string
	LoadBalancingMethod string
	HealthCheckInterval time.Duration
	ConnectTimeout      time.Duration
	ResponseTimeout     time.Duration
	MaxIdleConns        int
	MaxConnsPerHost     int
	KeepAlive          time.Duration
}

func NewProxyService(name string, config *ProxyConfig) (*ProxyService, *errors.Error) {
	if config == nil {
		config = &ProxyConfig{
			LoadBalancingMethod: "round_robin",
			HealthCheckInterval: 30 * time.Second,
			ConnectTimeout:      10 * time.Second,
			ResponseTimeout:     30 * time.Second,
			MaxIdleConns:        100,
			MaxConnsPerHost:     10,
			KeepAlive:          30 * time.Second,
		}
	}

	if len(config.Upstreams) == 0 {
		return nil, errors.New(errors.ProxyError, "at least one upstream is required")
	}

	var lb loadbalancing.LoadBalancer
	switch config.LoadBalancingMethod {
	case "round_robin":
		lb = loadbalancing.NewRoundRobin(config.Upstreams)
	case "random":
		lb = loadbalancing.NewRandom(config.Upstreams)
	case "least_conn":
		lb = loadbalancing.NewLeastConnection(config.Upstreams)
	default:
		return nil, errors.New(errors.LoadBalancingError, fmt.Sprintf("unknown load balancing method: %s", config.LoadBalancingMethod))
	}

	connPool := pool.NewHTTPConnectionPool(&pool.PoolConfig{
		MaxIdleConns:    config.MaxIdleConns,
		MaxConnsPerHost: config.MaxConnsPerHost,
		KeepAlive:      config.KeepAlive,
		ConnectTimeout: config.ConnectTimeout,
	})

	return &ProxyService{
		name:         name,
		upstreams:    config.Upstreams,
		loadBalancer: lb,
		connPool:     connPool,
		listeners:    make([]net.Listener, 0),
	}, nil
}

func (p *ProxyService) Name() string {
	return p.name
}

func (p *ProxyService) AddTCP(addr string) *errors.Error {
	return p.addListener("tcp", addr)
}

func (p *ProxyService) AddTLS(addr, certFile, keyFile string) *errors.Error {
	return errors.New(errors.InternalError, "TLS not implemented yet")
}

func (p *ProxyService) AddUDS(path string, mode *os.FileMode) *errors.Error {
	return p.addListener("unix", path)
}

func (p *ProxyService) addListener(network, addr string) *errors.Error {
	listener, err := net.Listen(network, addr)
	if err != nil {
		return errors.NewWithCause(errors.ConnectionError, fmt.Sprintf("failed to listen on %s", addr), err)
	}

	p.mu.Lock()
	p.listeners = append(p.listeners, listener)
	p.mu.Unlock()

	return nil
}

func (p *ProxyService) Start(ctx context.Context) *errors.Error {
	p.mu.RLock()
	listeners := make([]net.Listener, len(p.listeners))
	copy(listeners, p.listeners)
	p.mu.RUnlock()

	if len(listeners) == 0 {
		return errors.New(errors.ProxyError, "no listeners configured")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleRequest)

	p.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	for _, listener := range listeners {
		go func(l net.Listener) {
			if err := p.server.Serve(l); err != nil && err != http.ErrServerClosed {
				log.Printf("Proxy service %s failed to serve: %v", p.name, err)
			}
		}(listener)
	}

	return nil
}

func (p *ProxyService) Stop(ctx context.Context) *errors.Error {
	if p.server != nil {
		if err := p.server.Shutdown(ctx); err != nil {
			return errors.NewWithCause(errors.ProxyError, "failed to shutdown server", err)
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, listener := range p.listeners {
		if err := listener.Close(); err != nil {
			log.Printf("Failed to close listener: %v", err)
		}
	}

	return nil
}

func (p *ProxyService) handleRequest(w http.ResponseWriter, r *http.Request) {
	upstream, err := p.loadBalancer.SelectBackend()
	if err != nil {
		http.Error(w, "No available upstream", http.StatusBadGateway)
		return
	}

	if err := p.proxyRequest(w, r, upstream); err != nil {
		log.Printf("Proxy error: %v", err)
		http.Error(w, "Proxy error", err.HTTPStatusCode())
	}
}

func (p *ProxyService) proxyRequest(w http.ResponseWriter, r *http.Request, upstream string) *errors.Error {
	upstreamURL, err := url.Parse(fmt.Sprintf("http://%s", upstream))
	if err != nil {
		return errors.NewWithCause(errors.ProxyError, "invalid upstream URL", err)
	}

	proxyURL := &url.URL{
		Scheme:   upstreamURL.Scheme,
		Host:     upstreamURL.Host,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, proxyURL.String(), r.Body)
	if err != nil {
		return errors.NewWithCause(errors.ProxyError, "failed to create proxy request", err)
	}

	copyHeaders(proxyReq.Header, r.Header)
	proxyReq.Header.Set("X-Forwarded-For", getClientIP(r))
	proxyReq.Header.Set("X-Forwarded-Proto", getScheme(r))
	proxyReq.Header.Set("X-Forwarded-Host", r.Host)

	client := p.connPool.GetClient()
	resp, err := client.Do(proxyReq)
	if err != nil {
		return errors.NewWithCause(errors.ProxyError, "failed to proxy request", err)
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		return errors.NewWithCause(errors.ProxyError, "failed to copy response body", err)
	}

	return nil
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func getClientIP(r *http.Request) string {
	xRealIP := r.Header.Get("X-Real-Ip")
	if xRealIP != "" {
		return xRealIP
	}

	xForwardedFor := r.Header.Get("X-Forwarded-For")
	if xForwardedFor != "" {
		return strings.Split(xForwardedFor, ",")[0]
	}

	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	return ip
}

func getScheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	return "http"
}

type ProxyHTTP struct {
	upstreamAddr string
	client       *http.Client
}

func NewProxyHTTP(upstreamAddr string) *ProxyHTTP {
	return &ProxyHTTP{
		upstreamAddr: upstreamAddr,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (p *ProxyHTTP) RequestFilter(req *pingora_http.RequestHeader) *errors.Error {
	return nil
}

func (p *ProxyHTTP) ResponseFilter(resp *pingora_http.ResponseHeader) *errors.Error {
	return nil
}

func (p *ProxyHTTP) UpstreamPeer() string {
	return p.upstreamAddr
}

type HTTPProxy interface {
	RequestFilter(req *pingora_http.RequestHeader) *errors.Error
	ResponseFilter(resp *pingora_http.ResponseHeader) *errors.Error
	UpstreamPeer() string
}

func SimpleProxyService(listenAddr, upstreamAddr string) core.Service {
	config := &ProxyConfig{
		Upstreams: []string{upstreamAddr},
	}
	
	proxyService, err := NewProxyService("simple-proxy", config)
	if err != nil {
		log.Fatalf("Failed to create proxy service: %v", err)
	}
	
	if err := proxyService.AddTCP(listenAddr); err != nil {
		log.Fatalf("Failed to add TCP listener: %v", err)
	}
	
	return proxyService
}