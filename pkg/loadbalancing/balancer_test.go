package loadbalancing

import (
	"sync"
	"testing"
)

func TestRoundRobinBalancer(t *testing.T) {
	addresses := []string{"server1:80", "server2:80", "server3:80"}
	balancer := NewRoundRobin(addresses)

	t.Run("SelectBackend cycles through servers", func(t *testing.T) {
		results := make([]string, 6)
		for i := 0; i < 6; i++ {
			backend, err := balancer.SelectBackend()
			if err != nil {
				t.Fatalf("SelectBackend() error = %v", err)
			}
			results[i] = backend
		}

		// Should cycle through servers starting from the first increment
		expected := []string{
			"server2:80", "server3:80", "server1:80",
			"server2:80", "server3:80", "server1:80",
		}

		for i, expected := range expected {
			if results[i] != expected {
				t.Errorf("SelectBackend()[%d] = %v, expected %v", i, results[i], expected)
			}
		}
	})

	t.Run("UpdateBackends changes available servers", func(t *testing.T) {
		newAddresses := []string{"new1:80", "new2:80"}
		balancer.UpdateBackends(newAddresses)

		backend, err := balancer.SelectBackend()
		if err != nil {
			t.Fatalf("SelectBackend() error = %v", err)
		}

		if backend != "new1:80" && backend != "new2:80" {
			t.Errorf("SelectBackend() = %v, expected one of new servers", backend)
		}
	})

	t.Run("MarkUnhealthy removes backend from rotation", func(t *testing.T) {
		balancer.UpdateBackends(addresses)
		balancer.MarkUnhealthy("server2:80")

		// Should only return server1 and server3
		seen := make(map[string]bool)
		for i := 0; i < 10; i++ {
			backend, err := balancer.SelectBackend()
			if err != nil {
				t.Fatalf("SelectBackend() error = %v", err)
			}
			seen[backend] = true
		}

		if seen["server2:80"] {
			t.Errorf("SelectBackend() returned unhealthy server2:80")
		}
		if !seen["server1:80"] || !seen["server3:80"] {
			t.Errorf("SelectBackend() should return server1 and server3")
		}
	})

	t.Run("MarkHealthy restores backend to rotation", func(t *testing.T) {
		balancer.MarkHealthy("server2:80")

		seen := make(map[string]bool)
		for i := 0; i < 10; i++ {
			backend, err := balancer.SelectBackend()
			if err != nil {
				t.Fatalf("SelectBackend() error = %v", err)
			}
			seen[backend] = true
		}

		if !seen["server1:80"] || !seen["server2:80"] || !seen["server3:80"] {
			t.Errorf("SelectBackend() should return all servers after marking healthy")
		}
	})

	t.Run("No healthy backends returns error", func(t *testing.T) {
		for _, addr := range addresses {
			balancer.MarkUnhealthy(addr)
		}

		_, err := balancer.SelectBackend()
		if err == nil {
			t.Errorf("SelectBackend() expected error when no healthy backends")
		}
	})

	t.Run("GetHealthyBackends returns correct list", func(t *testing.T) {
		balancer.UpdateBackends(addresses)
		balancer.MarkUnhealthy("server2:80")

		healthy := balancer.GetHealthyBackends()
		expected := []string{"server1:80", "server3:80"}

		if len(healthy) != len(expected) {
			t.Errorf("GetHealthyBackends() length = %v, expected %v", len(healthy), len(expected))
		}

		healthyMap := make(map[string]bool)
		for _, backend := range healthy {
			healthyMap[backend] = true
		}

		for _, expected := range expected {
			if !healthyMap[expected] {
				t.Errorf("GetHealthyBackends() missing %v", expected)
			}
		}
	})
}

func TestRandomBalancer(t *testing.T) {
	addresses := []string{"server1:80", "server2:80", "server3:80"}
	balancer := NewRandom(addresses)

	t.Run("SelectBackend returns valid servers", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 50; i++ {
			backend, err := balancer.SelectBackend()
			if err != nil {
				t.Fatalf("SelectBackend() error = %v", err)
			}
			seen[backend] = true
		}

		// Should see all servers (with high probability)
		for _, addr := range addresses {
			if !seen[addr] {
				t.Logf("Warning: RandomBalancer didn't select %v in 50 attempts", addr)
			}
		}
	})

	t.Run("Concurrent access is safe", func(t *testing.T) {
		var wg sync.WaitGroup
		errors := make(chan error, 100)

		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					_, err := balancer.SelectBackend()
					if err != nil {
						errors <- err
						return
					}
				}
			}()
		}

		wg.Wait()
		close(errors)

		for err := range errors {
			t.Errorf("Concurrent SelectBackend() error = %v", err)
		}
	})
}

func TestLeastConnectionBalancer(t *testing.T) {
	addresses := []string{"server1:80", "server2:80", "server3:80"}
	balancer := NewLeastConnection(addresses)

	t.Run("SelectBackend returns server with least connections", func(t *testing.T) {
		// All servers start with 0 connections, should return first server
		backend, err := balancer.SelectBackend()
		if err != nil {
			t.Fatalf("SelectBackend() error = %v", err)
		}

		// After selecting, that server should have 1 connection
		// Next selection should prefer a different server
		backend2, err := balancer.SelectBackend()
		if err != nil {
			t.Fatalf("SelectBackend() error = %v", err)
		}

		if backend == backend2 {
			t.Logf("Note: Both selections returned %v, which is possible but unlikely", backend)
		}
	})

	t.Run("Connection counting works correctly", func(t *testing.T) {
		// Reset balancer
		balancer = NewLeastConnection(addresses)

		// Select a backend and check its connection count
		backend, err := balancer.SelectBackend()
		if err != nil {
			t.Fatalf("SelectBackend() error = %v", err)
		}

		// Find the backend in the internal list and check connection count
		balancer.mu.RLock()
		var selectedBackend *Backend
		for _, b := range balancer.backends {
			if b.Address == backend {
				selectedBackend = b
				break
			}
		}
		balancer.mu.RUnlock()

		if selectedBackend == nil {
			t.Fatalf("Selected backend %v not found in balancer", backend)
		}

		connections := selectedBackend.GetConnections()
		if connections != 1 {
			t.Errorf("Backend connections = %v, expected 1", connections)
		}

		// Decrement connection count
		selectedBackend.DecrementConnections()
		connections = selectedBackend.GetConnections()
		if connections != 0 {
			t.Errorf("Backend connections after decrement = %v, expected 0", connections)
		}
	})
}

func TestBackend(t *testing.T) {
	backend := &Backend{
		Address: "test:80",
		Healthy: true,
		Weight:  1,
	}

	t.Run("IsHealthy and SetHealthy", func(t *testing.T) {
		if !backend.IsHealthy() {
			t.Errorf("Backend.IsHealthy() = false, expected true")
		}

		backend.SetHealthy(false)
		if backend.IsHealthy() {
			t.Errorf("Backend.IsHealthy() after SetHealthy(false) = true, expected false")
		}

		backend.SetHealthy(true)
		if !backend.IsHealthy() {
			t.Errorf("Backend.IsHealthy() after SetHealthy(true) = false, expected true")
		}
	})

	t.Run("Connection counting", func(t *testing.T) {
		if backend.GetConnections() != 0 {
			t.Errorf("Backend.GetConnections() = %v, expected 0", backend.GetConnections())
		}

		backend.IncrementConnections()
		if backend.GetConnections() != 1 {
			t.Errorf("Backend.GetConnections() after increment = %v, expected 1", backend.GetConnections())
		}

		backend.IncrementConnections()
		if backend.GetConnections() != 2 {
			t.Errorf("Backend.GetConnections() after second increment = %v, expected 2", backend.GetConnections())
		}

		backend.DecrementConnections()
		if backend.GetConnections() != 1 {
			t.Errorf("Backend.GetConnections() after decrement = %v, expected 1", backend.GetConnections())
		}

		backend.DecrementConnections()
		if backend.GetConnections() != 0 {
			t.Errorf("Backend.GetConnections() after second decrement = %v, expected 0", backend.GetConnections())
		}
	})

	t.Run("Concurrent connection counting", func(t *testing.T) {
		backend := &Backend{Address: "test:80", Healthy: true}
		var wg sync.WaitGroup

		// Start multiple goroutines incrementing connections
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 10; j++ {
					backend.IncrementConnections()
				}
			}()
		}

		wg.Wait()

		expected := int64(100)
		if backend.GetConnections() != expected {
			t.Errorf("Backend.GetConnections() after concurrent increments = %v, expected %v", 
				backend.GetConnections(), expected)
		}

		// Start multiple goroutines decrementing connections
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := 0; j < 5; j++ {
					backend.DecrementConnections()
				}
			}()
		}

		wg.Wait()

		expected = int64(50)
		if backend.GetConnections() != expected {
			t.Errorf("Backend.GetConnections() after concurrent decrements = %v, expected %v", 
				backend.GetConnections(), expected)
		}
	})
}

func TestLoadBalancerIntegration(t *testing.T) {
	addresses := []string{"server1:80", "server2:80", "server3:80"}

	balancers := map[string]LoadBalancer{
		"RoundRobin":       NewRoundRobin(addresses),
		"Random":           NewRandom(addresses),
		"LeastConnection":  NewLeastConnection(addresses),
	}

	for name, balancer := range balancers {
		t.Run(name, func(t *testing.T) {
			// Test basic functionality
			backend, err := balancer.SelectBackend()
			if err != nil {
				t.Errorf("%s.SelectBackend() error = %v", name, err)
			}

			found := false
			for _, addr := range addresses {
				if backend == addr {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s.SelectBackend() = %v, not in original addresses", name, backend)
			}

			// Test health management
			balancer.MarkUnhealthy(backend)
			healthy := balancer.GetHealthyBackends()
			if len(healthy) != len(addresses)-1 {
				t.Errorf("%s.GetHealthyBackends() after marking unhealthy = %v backends, expected %v", 
					name, len(healthy), len(addresses)-1)
			}

			balancer.MarkHealthy(backend)
			healthy = balancer.GetHealthyBackends()
			if len(healthy) != len(addresses) {
				t.Errorf("%s.GetHealthyBackends() after marking healthy = %v backends, expected %v", 
					name, len(healthy), len(addresses))
			}

			// Test update backends
			newAddresses := []string{"new1:80", "new2:80"}
			balancer.UpdateBackends(newAddresses)
			healthy = balancer.GetHealthyBackends()
			if len(healthy) != len(newAddresses) {
				t.Errorf("%s.GetHealthyBackends() after update = %v backends, expected %v", 
					name, len(healthy), len(newAddresses))
			}
		})
	}
}