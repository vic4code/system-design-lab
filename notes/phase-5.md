# Phase 5 — JWT Authentication & Multi-User Management

> Status: **complete** — PR #12 (`feat/spotify-ui`)

---

## What we built

Full end-to-end JWT authentication so multiple users can sign up, log in, and have their own protected actions (creating playlists, uploading tracks).

---

## Backend

### Users table (`005_create_users.sql`)
```sql
CREATE TABLE users (
  id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  email         TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  name          TEXT NOT NULL,
  created_at    TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX users_email_idx ON users(email);
```

### Auth handler (`internal/handler/auth.go`)
| Route | Handler | Notes |
|---|---|---|
| `POST /v1/auth/register` | `Register` | bcrypt hash, INSERT, return JWT |
| `POST /v1/auth/login` | `Login` | compare hash, return JWT |
| `GET /v1/auth/me` | `Me` | protected, returns current user |

JWT: HS256, 7-day expiry, claims: `user_id`, `email`, `name`.

### JWT middleware (`internal/middleware/jwt.go`)
- `RequireAuth(secret)` — 401 if missing or invalid token
- `OptionalAuth(secret)` — sets context if token present, passes through regardless
- Reads `Authorization: Bearer <token>` header (also accepts `token` cookie)
- Sets `user_id`, `user_email`, `user_name` in gin context

### Protected routes
```
POST   /v1/artists              requireAuth
POST   /v1/tracks               requireAuth
POST   /v1/playlists            requireAuth
POST   /v1/playlists/:id/tracks requireAuth
DELETE /v1/playlists/:id/tracks/:track_id  requireAuth
```
All read routes remain public (no auth required).

---

## Frontend

### `context/AuthContext.tsx`
- Persists `token` + `user` in localStorage (`bs_token`, `bs_user`)
- States: `loading → guest | authenticated`
- Exports: `login(token, user)`, `logout()`, `user`, `token`, `state`

### `lib/api.ts`
- All requests auto-attach `Authorization: Bearer <token>` from localStorage
- Added `register(email, password, name)` and `login(email, password)` functions

### New pages
| Route | Description |
|---|---|
| `/login` | Email + password form → JWT stored, redirect to `/` |
| `/register` | Name + email + password → JWT stored, redirect to `/` |

### UI auth gates
- **Sidebar bottom**: logged-in → user avatar (initial) + name + hover logout; guest → Log in + Sign up links
- **Upload page**: redirects to `/login` if not authenticated
- **Library page**: create playlist button hidden for guests; login prompt shown
- **Playlist detail**: add/remove track controls hidden for guests

---

## Design decisions

**Why localStorage instead of httpOnly cookies?**
Simple SPA setup; Next.js frontend talks to a separate API domain. httpOnly cookies require CORS with `credentials: 'include'` and matching `Access-Control-Allow-Origin` + `Access-Control-Allow-Credentials` — more config for the same result in dev. For Phase 6 (AWS), we can revisit with Secure + SameSite=Strict cookies.

**Why 7-day token expiry with no refresh?**
Good enough for a lab. Production would use short-lived access tokens (15 min) + refresh tokens stored in httpOnly cookies.

**Why bcrypt DefaultCost (10)?**
Balances security vs. login latency (~100ms). For higher-traffic production use bcrypt cost 12+ or Argon2id.

---

## Interview questions

> *"How do you prevent token theft / XSS?"*
> localStorage is XSS-accessible. Mitigation: CSP headers, httpOnly refresh token with short-lived access token. For this lab, XSS surface is minimal (no user-generated HTML rendered).

> *"How would you implement role-based access (admin vs regular user)?"*
> Add a `role` claim to the JWT. Middleware checks `role == "admin"` for admin-only routes. Or store roles in DB and load on each request (slower but revocable).

> *"How do you revoke a JWT before it expires?"*
> JWT is stateless — you can't revoke without a server-side store. Options: (1) token blocklist in Redis (check on each request); (2) short expiry + refresh token rotation (rotation invalidates on reuse).
