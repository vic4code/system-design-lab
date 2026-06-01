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

---

## Demo

**前置：** `make up && make migrate && make seed`

---

### 1. Register → 看到 JWT token 回傳

```bash
curl -s -X POST https://localhost/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"demo@example.com","password":"password123","name":"Demo User"}' \
  | python3 -m json.tool
```

**你應該看到：**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "user": {
    "id": "uuid...",
    "email": "demo@example.com",
    "name": "Demo User",
    "role": "user"
  }
}
```

**解碼 JWT payload（不需要 secret）：**
```bash
TOKEN="（上面的 token）"
echo $TOKEN | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool
```

**你應該看到：** `user_id`、`email`、`role`、`exp`（7天後的 unix timestamp）。

**這說明了什麼：** JWT 是 base64 編碼，payload 任何人都可以讀，但不能偽造（需要 `JWT_SECRET` 才能簽名）。Stateless：server 不需要存 session，3 個 API instance 都可以用同一個 secret 驗證。

---

### 2. 沒有 token 打保護路由 → 看到 401

```bash
curl -s https://localhost/v1/auth/me
```

**你應該看到：** `{"error":"authentication required"}`

```bash
# 帶 token
curl -s https://localhost/v1/auth/me \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
```

**你應該看到：** 完整的 user 資料。

---

### 3. 錯誤密碼 → 看到 401，且 bcrypt 保護

```bash
curl -s -X POST https://localhost/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"demo@example.com","password":"wrongpassword"}'
```

**你應該看到：** `{"error":"invalid email or password"}`（故意模糊，不透露是 email 不存在還是密碼錯）

**驗證密碼是 bcrypt hash，不是明文：**
```bash
docker exec beatstream-postgres-1 psql -U user -d beatstream \
  -c "SELECT email, LEFT(password_hash, 7) FROM users WHERE email='demo@example.com';"
```

**你應該看到：** `$2a$10$`（bcrypt 格式，cost factor 10）。

---

### 4. 建立 playlist（需要登入）→ 沒 token 就 401

```bash
# 沒 token
curl -s -X POST https://localhost/v1/playlists \
  -H "Content-Type: application/json" \
  -d '{"name":"My Playlist"}'
# → {"error":"authentication required"}

# 有 token
curl -s -X POST https://localhost/v1/playlists \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"My Playlist"}' | python3 -m json.tool
# → {"id":"...","name":"My Playlist"}
```
