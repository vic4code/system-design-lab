# Phase 5 — Recommendation System

> Coming next — playlist co-occurrence based recommendations.

---

## Planned approach

### Data source
`play_events` table (populated by the analytics worker from Phase 2) — each row records which user played which track and when.

### Algorithm: item-item collaborative filtering (co-occurrence)
```
For track A, find all tracks B such that:
  users who played A also frequently played B

co_occurrence(A, B) = |users(A) ∩ users(B)| / sqrt(|users(A)| × |users(B)|)
```

This is the classic "users who played this also played" pattern (Spotify's early recommendation approach).

### Architecture sketch
```
play_events (DB)
  │
  ▼  scheduled job (nightly or every 6h)
co_occurrence matrix → Redis sorted set
  key:   recs:{track_id}
  value: sorted by score DESC (top-N candidates)
  │
  ▼
GET /v1/tracks/:id/recommendations
  └── reads Redis, returns top 10 tracks
```

### Files to add (planned)
```
beatstream/
├── internal/worker/recommendations.go   batch job: compute + store in Redis
├── internal/handler/recommendations.go  GET /v1/tracks/:id/recommendations
└── cmd/recs/main.go                      one-shot CLI to trigger recompute
```

---

## Interview questions to think about

> *"How does your recommendation algorithm scale to millions of tracks?"*
> Pure co-occurrence becomes O(n²) storage. At scale: use matrix factorisation (ALS/SVD) to reduce to embedding vectors, then approximate nearest-neighbour search (FAISS, Pinecone). Item embeddings also generalise to cold-start (new tracks with no play history) via content-based features (genre, BPM, mood).

> *"How do you handle cold start for new tracks?"*
> Two fallbacks: (1) editorial playlists as seed data; (2) content-based similarity on track metadata until co-occurrence data accumulates (~50 plays threshold).
