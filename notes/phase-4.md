# Phase 4 — Frontend (Next.js)

## Architecture

```
Browser
  │
  ▼
Next.js 15 (beatstream/web/) — Vercel / Docker
  │  App Router + Tailwind CSS (dark Spotify-like theme)
  │
  ├── /                  Track list + inline audio player
  ├── /search            Debounced full-text search
  ├── /upload            Multi-step upload → status polling
  └── /playlists         Playlist CRUD + track management
  │
  │  rewrites: /v1/* → API_URL/v1/*
  ▼
Go API (port 8080)
  └── CORS: Access-Control-Allow-Origin: *
```

---

## Files added

```
beatstream/
├── web/
│   ├── app/
│   │   ├── layout.tsx          root layout, PlayerContext provider, Navbar
│   │   ├── globals.css         Tailwind base + custom scrollbar
│   │   ├── page.tsx            home: track list, playback via TrackRow
│   │   ├── search/page.tsx     debounced search with 300ms input delay
│   │   ├── upload/page.tsx     wraps UploadForm component
│   │   └── playlists/
│   │       ├── page.tsx        playlist list + create form
│   │       └── [id]/page.tsx   playlist detail: tracks, add/remove
│   ├── components/
│   │   ├── Navbar.tsx          top nav with links
│   │   ├── Player.tsx          fixed bottom audio bar (play/pause/seek)
│   │   └── TrackRow.tsx        row: index/play, title, status badge, duration
│   ├── context/
│   │   └── PlayerContext.tsx   global play state (HTML5 Audio API)
│   ├── lib/
│   │   └── api.ts              typed fetch wrapper for all API endpoints
│   ├── next.config.ts          /v1/* rewrite proxy to backend
│   ├── vercel.json             framework=nextjs, region=sin1
│   └── .gitignore
├── internal/middleware/
│   └── cors.go                 CORS headers (required for browser → API)
└── .github/workflows/
    └── vercel.yml              GitHub Actions: auto-deploy to Vercel on push to main
```

**Go API changes:**
```
internal/handler/artists.go    added List() — GET /v1/artists
internal/handler/tracks.go     added List() — GET /v1/tracks
internal/handler/playlists.go  added List() — GET /v1/playlists
internal/middleware/cors.go    new — wildcard CORS for browser access
cmd/api/main.go                CORS middleware + new list routes registered
```

---

## Key design decisions

### HTML5 Audio + pre-signed URL streaming

The `GET /v1/tracks/:id/stream` endpoint returns a `307 Temporary Redirect` to a pre-signed S3 URL (valid 60s). The browser's `<audio>` element follows the redirect natively — no proxy needed, bandwidth goes S3 → browser directly.

`S3_PRESIGN_ENDPOINT` env var separates the internal endpoint (MinIO container DNS) from the browser-accessible one (localhost:9000 in dev, real AWS in prod).

### PlayerContext global state

The audio player needs to persist across page navigations (App Router navigates without unmounting the root layout). A React Context at the root layout level holds the `Audio` object — created once in a `useEffect` so it only runs client-side (avoids SSR issues with `window`).

```
layout.tsx  →  PlayerContext (Audio instance lives here)
  ├── Navbar.tsx
  ├── {children}  — any page can call usePlayer() to play/pause/seek
  └── Player.tsx  — reads context, renders fixed bottom bar
```

### Next.js rewrites vs CORS

Two options for local dev:

| | Rewrites | CORS headers |
|---|---|---|
| Setup | next.config.ts rewrite rule | Go middleware |
| Browser sees | Same origin (`localhost:3000/v1/*`) | Cross-origin (port 8080) |
| Cookie support | Yes (same-origin) | Requires `credentials: include` |
| Prod | Works without CORS if same domain | Needed if frontend on different domain |

Both are implemented: rewrites handle local dev; CORS headers handle prod (Vercel domain ≠ API domain until custom domain is set up).

### UploadForm state machine

```
idle → uploading (POST /v1/tracks, multipart)
     → polling   (GET /v1/tracks/:id every 2s)
     → ready     (status = ready, show player button)
     → error     (status = error, show retry)
```

Worker pipeline (from Phase 2): Kafka → upload worker → S3 → updates track status in DB.

---

## CI/CD: Vercel deployment

```yaml
# .github/workflows/vercel.yml
on:
  push:
    branches: [main]
    paths: ["beatstream/web/**"]
  pull_request:
    branches: [main]
    paths: ["beatstream/web/**"]
```

Flow:
1. Validate `VERCEL_TOKEN` secret exists
2. Resolve (or auto-create) Vercel project `beatstream` via REST API
3. `vercel build --prod` (runs inside `beatstream/web/`)
4. **main push** → `vercel deploy --prebuilt --prod`
5. **PR** → `vercel deploy --prebuilt` (preview) + posts preview URL as PR comment

**Setup steps:**
1. Vercel account → Settings → Tokens → create a token
2. GitHub repo → Settings → Secrets → `VERCEL_TOKEN`
3. Next push to main touching `beatstream/web/**` triggers first deploy

---

## Running locally

```bash
# Start backend
make up

# Install and start frontend
make web-install
make web-dev
# → http://localhost:3000

# Environment (optional — defaults work for docker-compose)
# beatstream/web/.env.local:
# NEXT_PUBLIC_API_URL=   (empty = use rewrites proxy)
# API_URL=http://localhost:8080  (for server-side rewrites)
```

---

## Interview questions to answer before Phase 5

> *"How does the audio player avoid buffering the whole file?"*
> The pre-signed URL points directly to S3. The browser sends HTTP Range requests natively (`Range: bytes=0-`), so it streams in chunks. S3 supports partial content responses (206). No server proxy means the API doesn't become a bandwidth bottleneck.

> *"Why React Context instead of Zustand/Redux for player state?"*
> The state surface is small (one track, one Audio object, play/pause, progress). Context avoids adding a dependency. If we add features like queue management, offline support, or cross-tab sync — then a proper store makes sense.

> *"What breaks when you navigate between pages?"*
> Nothing — the `Audio` object lives in the Context provider which wraps the entire app. Next.js App Router only unmounts/remounts `{children}`, not the root layout. The Player bar stays mounted and keeps playing.
