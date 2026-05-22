// Consistent hashing ring implementation.
// Run: go run main.go
package main

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
)

type Ring struct {
	nodes  map[uint32]string // hash position → node name
	sorted []uint32          // sorted positions for binary search
	vnodes int               // virtual nodes per physical node
}

func NewRing(vnodes int) *Ring {
	return &Ring{
		nodes:  make(map[uint32]string),
		vnodes: vnodes,
	}
}

func (r *Ring) Add(node string) {
	for i := 0; i < r.vnodes; i++ {
		key := fmt.Sprintf("%s-v%d", node, i)
		h := hash(key)
		r.nodes[h] = node
		r.sorted = append(r.sorted, h)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return r.sorted[i] < r.sorted[j] })
}

func (r *Ring) Remove(node string) {
	for i := 0; i < r.vnodes; i++ {
		key := fmt.Sprintf("%s-v%d", node, i)
		h := hash(key)
		delete(r.nodes, h)
	}
	r.sorted = r.sorted[:0]
	for h := range r.nodes {
		r.sorted = append(r.sorted, h)
	}
	sort.Slice(r.sorted, func(i, j int) bool { return r.sorted[i] < r.sorted[j] })
}

func (r *Ring) Get(key string) string {
	if len(r.sorted) == 0 {
		return ""
	}
	h := hash(key)
	idx := sort.Search(len(r.sorted), func(i int) bool {
		return r.sorted[i] >= h
	})
	if idx == len(r.sorted) {
		idx = 0 // wrap around the ring
	}
	return r.nodes[r.sorted[idx]]
}

func hash(s string) uint32 {
	return crc32.ChecksumIEEE([]byte(s))
}

func main() {
	ring := NewRing(150) // 150 virtual nodes per physical node

	nodes := []string{"cache-1", "cache-2", "cache-3", "cache-4", "cache-5"}
	for _, n := range nodes {
		ring.Add(n)
	}

	// Show distribution of 1000 keys across nodes
	distribution := make(map[string]int)
	for i := 0; i < 1000; i++ {
		key := "track:" + strconv.Itoa(i)
		node := ring.Get(key)
		distribution[node]++
	}

	fmt.Println("Key distribution across 5 nodes (1000 keys, 150 vnodes each):")
	for _, n := range nodes {
		bar := ""
		for j := 0; j < distribution[n]/5; j++ {
			bar += "█"
		}
		fmt.Printf("  %-10s %3d keys  %s\n", n, distribution[n], bar)
	}

	// Demonstrate: add a new node and measure remapping
	fmt.Println("\nAdding cache-6...")
	before := make(map[string]string)
	for i := 0; i < 1000; i++ {
		key := "track:" + strconv.Itoa(i)
		before[key] = ring.Get(key)
	}

	ring.Add("cache-6")

	remapped := 0
	for i := 0; i < 1000; i++ {
		key := "track:" + strconv.Itoa(i)
		if ring.Get(key) != before[key] {
			remapped++
		}
	}
	fmt.Printf("Keys remapped: %d/1000 (%.1f%%) — expected ~%.0f%%\n",
		remapped, float64(remapped)/10, 100.0/6)
}
