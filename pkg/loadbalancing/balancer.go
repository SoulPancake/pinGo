package loadbalancing

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type LoadBalancer interface {
	SelectBackend() (string, *errors.Error)
	UpdateBackends(backends []string)
	MarkHealthy(backend string)
	MarkUnhealthy(backend string)
	GetHealthyBackends() []string
}

type Backend struct {
	Address     string
	Healthy     bool
	Weight      int
	Connections int64
	mu          sync.RWMutex
}

func (b *Backend) IsHealthy() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.Healthy
}

func (b *Backend) SetHealthy(healthy bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Healthy = healthy
}

func (b *Backend) IncrementConnections() {
	atomic.AddInt64(&b.Connections, 1)
}

func (b *Backend) DecrementConnections() {
	atomic.AddInt64(&b.Connections, -1)
}

func (b *Backend) GetConnections() int64 {
	return atomic.LoadInt64(&b.Connections)
}

type RoundRobinBalancer struct {
	backends []*Backend
	current  uint64
	mu       sync.RWMutex
}

func NewRoundRobin(addresses []string) *RoundRobinBalancer {
	backends := make([]*Backend, len(addresses))
	for i, addr := range addresses {
		backends[i] = &Backend{
			Address: addr,
			Healthy: true,
			Weight:  1,
		}
	}
	
	return &RoundRobinBalancer{
		backends: backends,
	}
}

func (r *RoundRobinBalancer) SelectBackend() (string, *errors.Error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	healthyBackends := r.getHealthyBackends()
	if len(healthyBackends) == 0 {
		return "", errors.New(errors.LoadBalancingError, "no healthy backends available")
	}
	
	index := atomic.AddUint64(&r.current, 1) % uint64(len(healthyBackends))
	return healthyBackends[index].Address, nil
}

func (r *RoundRobinBalancer) UpdateBackends(addresses []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	backends := make([]*Backend, len(addresses))
	for i, addr := range addresses {
		backends[i] = &Backend{
			Address: addr,
			Healthy: true,
			Weight:  1,
		}
	}
	r.backends = backends
	atomic.StoreUint64(&r.current, 0)
}

func (r *RoundRobinBalancer) MarkHealthy(backend string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for _, b := range r.backends {
		if b.Address == backend {
			b.SetHealthy(true)
			break
		}
	}
}

func (r *RoundRobinBalancer) MarkUnhealthy(backend string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for _, b := range r.backends {
		if b.Address == backend {
			b.SetHealthy(false)
			break
		}
	}
}

func (r *RoundRobinBalancer) GetHealthyBackends() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	healthyBackends := r.getHealthyBackends()
	addresses := make([]string, len(healthyBackends))
	for i, backend := range healthyBackends {
		addresses[i] = backend.Address
	}
	return addresses
}

func (r *RoundRobinBalancer) getHealthyBackends() []*Backend {
	var healthy []*Backend
	for _, backend := range r.backends {
		if backend.IsHealthy() {
			healthy = append(healthy, backend)
		}
	}
	return healthy
}

type RandomBalancer struct {
	backends []*Backend
	rand     *rand.Rand
	mu       sync.RWMutex
}

func NewRandom(addresses []string) *RandomBalancer {
	backends := make([]*Backend, len(addresses))
	for i, addr := range addresses {
		backends[i] = &Backend{
			Address: addr,
			Healthy: true,
			Weight:  1,
		}
	}
	
	return &RandomBalancer{
		backends: backends,
		rand:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (r *RandomBalancer) SelectBackend() (string, *errors.Error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	healthyBackends := r.getHealthyBackends()
	if len(healthyBackends) == 0 {
		return "", errors.New(errors.LoadBalancingError, "no healthy backends available")
	}
	
	index := r.rand.Intn(len(healthyBackends))
	return healthyBackends[index].Address, nil
}

func (r *RandomBalancer) UpdateBackends(addresses []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	backends := make([]*Backend, len(addresses))
	for i, addr := range addresses {
		backends[i] = &Backend{
			Address: addr,
			Healthy: true,
			Weight:  1,
		}
	}
	r.backends = backends
}

func (r *RandomBalancer) MarkHealthy(backend string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for _, b := range r.backends {
		if b.Address == backend {
			b.SetHealthy(true)
			break
		}
	}
}

func (r *RandomBalancer) MarkUnhealthy(backend string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for _, b := range r.backends {
		if b.Address == backend {
			b.SetHealthy(false)
			break
		}
	}
}

func (r *RandomBalancer) GetHealthyBackends() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	healthyBackends := r.getHealthyBackends()
	addresses := make([]string, len(healthyBackends))
	for i, backend := range healthyBackends {
		addresses[i] = backend.Address
	}
	return addresses
}

func (r *RandomBalancer) getHealthyBackends() []*Backend {
	var healthy []*Backend
	for _, backend := range r.backends {
		if backend.IsHealthy() {
			healthy = append(healthy, backend)
		}
	}
	return healthy
}

type LeastConnectionBalancer struct {
	backends []*Backend
	mu       sync.RWMutex
}

func NewLeastConnection(addresses []string) *LeastConnectionBalancer {
	backends := make([]*Backend, len(addresses))
	for i, addr := range addresses {
		backends[i] = &Backend{
			Address: addr,
			Healthy: true,
			Weight:  1,
		}
	}
	
	return &LeastConnectionBalancer{
		backends: backends,
	}
}

func (l *LeastConnectionBalancer) SelectBackend() (string, *errors.Error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	healthyBackends := l.getHealthyBackends()
	if len(healthyBackends) == 0 {
		return "", errors.New(errors.LoadBalancingError, "no healthy backends available")
	}
	
	var selected *Backend
	minConnections := int64(-1)
	
	for _, backend := range healthyBackends {
		connections := backend.GetConnections()
		if minConnections == -1 || connections < minConnections {
			minConnections = connections
			selected = backend
		}
	}
	
	if selected != nil {
		selected.IncrementConnections()
		return selected.Address, nil
	}
	
	return "", errors.New(errors.LoadBalancingError, "failed to select backend")
}

func (l *LeastConnectionBalancer) UpdateBackends(addresses []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	backends := make([]*Backend, len(addresses))
	for i, addr := range addresses {
		backends[i] = &Backend{
			Address: addr,
			Healthy: true,
			Weight:  1,
		}
	}
	l.backends = backends
}

func (l *LeastConnectionBalancer) MarkHealthy(backend string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	for _, b := range l.backends {
		if b.Address == backend {
			b.SetHealthy(true)
			break
		}
	}
}

func (l *LeastConnectionBalancer) MarkUnhealthy(backend string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	for _, b := range l.backends {
		if b.Address == backend {
			b.SetHealthy(false)
			break
		}
	}
}

func (l *LeastConnectionBalancer) GetHealthyBackends() []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	
	healthyBackends := l.getHealthyBackends()
	addresses := make([]string, len(healthyBackends))
	for i, backend := range healthyBackends {
		addresses[i] = backend.Address
	}
	return addresses
}

func (l *LeastConnectionBalancer) getHealthyBackends() []*Backend {
	var healthy []*Backend
	for _, backend := range l.backends {
		if backend.IsHealthy() {
			healthy = append(healthy, backend)
		}
	}
	return healthy
}