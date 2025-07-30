package pool

import (
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type ConnectionPool interface {
	GetClient() *http.Client
	GetConnection(address string) (net.Conn, *errors.Error)
	ReleaseConnection(conn net.Conn, address string) *errors.Error
	Close() *errors.Error
}

type PoolConfig struct {
	MaxIdleConns    int
	MaxConnsPerHost int
	KeepAlive      time.Duration
	ConnectTimeout time.Duration
	IdleTimeout    time.Duration
}

func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxIdleConns:    100,
		MaxConnsPerHost: 10,
		KeepAlive:      30 * time.Second,
		ConnectTimeout: 10 * time.Second,
		IdleTimeout:    90 * time.Second,
	}
}

type HTTPConnectionPool struct {
	config *PoolConfig
	client *http.Client
	mu     sync.RWMutex
}

func NewHTTPConnectionPool(config *PoolConfig) *HTTPConnectionPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   config.ConnectTimeout,
			KeepAlive: config.KeepAlive,
		}).DialContext,
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleTimeout,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  false,
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   30 * time.Second,
	}

	return &HTTPConnectionPool{
		config: config,
		client: client,
	}
}

func (p *HTTPConnectionPool) GetClient() *http.Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client
}

func (p *HTTPConnectionPool) GetConnection(address string) (net.Conn, *errors.Error) {
	dialer := &net.Dialer{
		Timeout:   p.config.ConnectTimeout,
		KeepAlive: p.config.KeepAlive,
	}

	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, errors.NewWithCause(errors.ConnectionError, "failed to dial", err)
	}

	return conn, nil
}

func (p *HTTPConnectionPool) ReleaseConnection(conn net.Conn, address string) *errors.Error {
	if conn != nil {
		if err := conn.Close(); err != nil {
			return errors.NewWithCause(errors.ConnectionError, "failed to close connection", err)
		}
	}
	return nil
}

func (p *HTTPConnectionPool) Close() *errors.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if transport, ok := p.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	return nil
}

type TCPConnectionPool struct {
	config      *PoolConfig
	pools       map[string]*hostPool
	mu          sync.RWMutex
}

type hostPool struct {
	address     string
	connections []net.Conn
	mu          sync.Mutex
	maxConns    int
	activeConns int
}

func NewTCPConnectionPool(config *PoolConfig) *TCPConnectionPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	return &TCPConnectionPool{
		config: config,
		pools:  make(map[string]*hostPool),
	}
}

func (p *TCPConnectionPool) GetClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := p.GetConnection(addr)
				if err != nil {
					return nil, err
				}
				return conn, nil
			},
		},
		Timeout: 30 * time.Second,
	}
}

func (p *TCPConnectionPool) GetConnection(address string) (net.Conn, *errors.Error) {
	p.mu.RLock()
	pool, exists := p.pools[address]
	p.mu.RUnlock()

	if !exists {
		p.mu.Lock()
		if pool, exists = p.pools[address]; !exists {
			pool = &hostPool{
				address:     address,
				connections: make([]net.Conn, 0, p.config.MaxConnsPerHost),
				maxConns:    p.config.MaxConnsPerHost,
			}
			p.pools[address] = pool
		}
		p.mu.Unlock()
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	if len(pool.connections) > 0 {
		conn := pool.connections[len(pool.connections)-1]
		pool.connections = pool.connections[:len(pool.connections)-1]
		pool.activeConns++
		return conn, nil
	}

	if pool.activeConns >= pool.maxConns {
		return nil, errors.New(errors.PoolError, "connection pool exhausted")
	}

	dialer := &net.Dialer{
		Timeout:   p.config.ConnectTimeout,
		KeepAlive: p.config.KeepAlive,
	}

	conn, err := dialer.Dial("tcp", address)
	if err != nil {
		return nil, errors.NewWithCause(errors.ConnectionError, "failed to dial", err)
	}

	pool.activeConns++
	return conn, nil
}

func (p *TCPConnectionPool) ReleaseConnection(conn net.Conn, address string) *errors.Error {
	if conn == nil {
		return nil
	}

	p.mu.RLock()
	pool, exists := p.pools[address]
	p.mu.RUnlock()

	if !exists {
		return errors.OrErr(conn.Close(), errors.ConnectionError, "failed to close connection")
	}

	pool.mu.Lock()
	defer pool.mu.Unlock()

	pool.activeConns--

	if len(pool.connections) < cap(pool.connections) {
		pool.connections = append(pool.connections, conn)
		return nil
	}

	return errors.OrErr(conn.Close(), errors.ConnectionError, "failed to close excess connection")
}

func (p *TCPConnectionPool) Close() *errors.Error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, pool := range p.pools {
		pool.mu.Lock()
		for _, conn := range pool.connections {
			conn.Close()
		}
		pool.connections = nil
		pool.mu.Unlock()
	}

	p.pools = make(map[string]*hostPool)
	return nil
}

type PoolStats struct {
	MaxIdleConns     int
	MaxConnsPerHost  int
	CurrentIdleConns int
	ActiveConns      int
	TotalConns       int
}

func (p *HTTPConnectionPool) GetStats() PoolStats {
	return PoolStats{
		MaxIdleConns:    p.config.MaxIdleConns,
		MaxConnsPerHost: p.config.MaxConnsPerHost,
	}
}

func (p *TCPConnectionPool) GetStats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := PoolStats{
		MaxIdleConns:    p.config.MaxIdleConns,
		MaxConnsPerHost: p.config.MaxConnsPerHost,
	}

	for _, pool := range p.pools {
		pool.mu.Lock()
		stats.CurrentIdleConns += len(pool.connections)
		stats.ActiveConns += pool.activeConns
		pool.mu.Unlock()
	}

	stats.TotalConns = stats.CurrentIdleConns + stats.ActiveConns
	return stats
}