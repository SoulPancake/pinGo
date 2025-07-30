package ketama

import (
	"crypto/md5"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type Ring struct {
	nodes    map[uint32]string
	sortedKeys []uint32
	replicas int
	mu       sync.RWMutex
}

func NewRing(replicas int) *Ring {
	return &Ring{
		nodes:    make(map[uint32]string),
		replicas: replicas,
	}
}

func (r *Ring) AddNode(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for i := 0; i < r.replicas; i++ {
		key := r.hash(fmt.Sprintf("%s:%d", node, i))
		r.nodes[key] = node
		r.sortedKeys = append(r.sortedKeys, key)
	}
	
	sort.Slice(r.sortedKeys, func(i, j int) bool {
		return r.sortedKeys[i] < r.sortedKeys[j]
	})
}

func (r *Ring) RemoveNode(node string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	for i := 0; i < r.replicas; i++ {
		key := r.hash(fmt.Sprintf("%s:%d", node, i))
		delete(r.nodes, key)
	}
	
	r.rebuildSortedKeys()
}

func (r *Ring) GetNode(key string) (string, *errors.Error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if len(r.nodes) == 0 {
		return "", errors.New(errors.LoadBalancingError, "no nodes available in ring")
	}
	
	hash := r.hash(key)
	idx := r.search(hash)
	
	if idx >= len(r.sortedKeys) {
		idx = 0
	}
	
	nodeKey := r.sortedKeys[idx]
	return r.nodes[nodeKey], nil
}

func (r *Ring) GetNodes(key string, count int) ([]string, *errors.Error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	if len(r.nodes) == 0 {
		return nil, errors.New(errors.LoadBalancingError, "no nodes available in ring")
	}
	
	if count <= 0 {
		return nil, errors.New(errors.LoadBalancingError, "count must be positive")
	}
	
	hash := r.hash(key)
	idx := r.search(hash)
	
	seen := make(map[string]bool)
	result := make([]string, 0, count)
	
	for len(result) < count && len(seen) < len(r.getUniqueNodes()) {
		if idx >= len(r.sortedKeys) {
			idx = 0
		}
		
		nodeKey := r.sortedKeys[idx]
		node := r.nodes[nodeKey]
		
		if !seen[node] {
			seen[node] = true
			result = append(result, node)
		}
		
		idx++
	}
	
	return result, nil
}

func (r *Ring) GetAllNodes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	return r.getUniqueNodes()
}

func (r *Ring) getUniqueNodes() []string {
	nodeSet := make(map[string]bool)
	for _, node := range r.nodes {
		nodeSet[node] = true
	}
	
	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}
	
	sort.Strings(nodes)
	return nodes
}

func (r *Ring) search(hash uint32) int {
	return sort.Search(len(r.sortedKeys), func(i int) bool {
		return r.sortedKeys[i] >= hash
	})
}

func (r *Ring) rebuildSortedKeys() {
	r.sortedKeys = make([]uint32, 0, len(r.nodes))
	for key := range r.nodes {
		r.sortedKeys = append(r.sortedKeys, key)
	}
	
	sort.Slice(r.sortedKeys, func(i, j int) bool {
		return r.sortedKeys[i] < r.sortedKeys[j]
	})
}

func (r *Ring) hash(key string) uint32 {
	h := md5.New()
	h.Write([]byte(key))
	digest := h.Sum(nil)
	
	return uint32(digest[0])<<24 |
		uint32(digest[1])<<16 |
		uint32(digest[2])<<8 |
		uint32(digest[3])
}

func (r *Ring) Size() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.getUniqueNodes())
}

func (r *Ring) IsEmpty() bool {
	return r.Size() == 0
}

type KetamaBalancer struct {
	ring *Ring
}

func NewKetamaBalancer(nodes []string, replicas int) *KetamaBalancer {
	if replicas <= 0 {
		replicas = 160
	}
	
	ring := NewRing(replicas)
	for _, node := range nodes {
		ring.AddNode(node)
	}
	
	return &KetamaBalancer{
		ring: ring,
	}
}

func (k *KetamaBalancer) SelectBackend(key string) (string, *errors.Error) {
	return k.ring.GetNode(key)
}

func (k *KetamaBalancer) SelectBackends(key string, count int) ([]string, *errors.Error) {
	return k.ring.GetNodes(key, count)
}

func (k *KetamaBalancer) AddBackend(backend string) {
	k.ring.AddNode(backend)
}

func (k *KetamaBalancer) RemoveBackend(backend string) {
	k.ring.RemoveNode(backend)
}

func (k *KetamaBalancer) GetAllBackends() []string {
	return k.ring.GetAllNodes()
}

func (k *KetamaBalancer) UpdateBackends(backends []string) {
	currentBackends := k.ring.GetAllNodes()
	
	toRemove := make(map[string]bool)
	for _, backend := range currentBackends {
		toRemove[backend] = true
	}
	
	for _, backend := range backends {
		if toRemove[backend] {
			delete(toRemove, backend)
		} else {
			k.ring.AddNode(backend)
		}
	}
	
	for backend := range toRemove {
		k.ring.RemoveNode(backend)
	}
}

type WeightedKetamaBalancer struct {
	ring    *Ring
	weights map[string]int
	mu      sync.RWMutex
}

func NewWeightedKetamaBalancer(replicas int) *WeightedKetamaBalancer {
	if replicas <= 0 {
		replicas = 160
	}
	
	return &WeightedKetamaBalancer{
		ring:    NewRing(replicas),
		weights: make(map[string]int),
	}
}

func (w *WeightedKetamaBalancer) AddBackend(backend string, weight int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	if weight <= 0 {
		weight = 1
	}
	
	w.weights[backend] = weight
	
	for i := 0; i < weight; i++ {
		virtualNode := fmt.Sprintf("%s-%d", backend, i)
		w.ring.AddNode(virtualNode)
	}
}

func (w *WeightedKetamaBalancer) RemoveBackend(backend string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	
	weight, exists := w.weights[backend]
	if !exists {
		return
	}
	
	for i := 0; i < weight; i++ {
		virtualNode := fmt.Sprintf("%s-%d", backend, i)
		w.ring.RemoveNode(virtualNode)
	}
	
	delete(w.weights, backend)
}

func (w *WeightedKetamaBalancer) SelectBackend(key string) (string, *errors.Error) {
	node, err := w.ring.GetNode(key)
	if err != nil {
		return "", err
	}
	
	if idx := len(node) - 1; idx >= 0 {
		for i := idx; i >= 0; i-- {
			if node[i] == '-' {
				return node[:i], nil
			}
		}
	}
	
	return node, nil
}

func (w *WeightedKetamaBalancer) GetStats() map[string]int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	
	stats := make(map[string]int)
	for backend, weight := range w.weights {
		stats[backend] = weight
	}
	
	return stats
}

func HashKey(key string) string {
	return strconv.FormatUint(uint64(md5.Sum([]byte(key))[0]), 16)
}