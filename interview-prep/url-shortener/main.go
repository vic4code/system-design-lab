// Minimal URL shortener API.
// Run: go run main.go
// Demonstrates: hashing, redirect, in-memory store (swap for Redis+Postgres in prod).
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"sync"
)

type store struct {
	mu    sync.RWMutex
	codes map[string]string // short code → original URL
}

func (s *store) save(code, url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[code] = url
}

func (s *store) lookup(code string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.codes[code]
	return u, ok
}

func shortCode() string {
	b := make([]byte, 6)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)[:8]
}

func main() {
	db := &store{codes: make(map[string]string)}
	mux := http.NewServeMux()

	// POST /shorten  body: url=https://...
	mux.HandleFunc("POST /shorten", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		original := r.FormValue("url")
		if original == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}
		code := shortCode()
		db.save(code, original)
		fmt.Fprintf(w, "http://localhost:8080/%s\n", code)
	})

	// GET /{code}  → 301 redirect
	mux.HandleFunc("GET /{code}", func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		url, ok := db.lookup(code)
		if !ok {
			http.NotFound(w, r)
			return
		}
		// 301 Permanent: browser caches the redirect (good for CDN, but hard to update)
		// 302 Temporary: no caching (easier to change destinations)
		// Use 302 until you're sure the mapping is permanent.
		http.Redirect(w, r, url, http.StatusFound)
	})

	log.Println("url shortener on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
