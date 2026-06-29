# Tech Debt

Issues ranked by severity: **Critical → High → Medium → Low**.

---

## Critical

### 1. JWT algorithm confusion attack
**File:** `internal/auth/auth.go:53`

`ValidateJWT` does not validate the signing method in the key callback. An attacker can craft a token signed with `none` or a different algorithm and bypass signature verification.

```go
// Fix: check the method before returning the key
func(token *jwt.Token) (any, error) {
    if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
        return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
    }
    return []byte(tokenSecret), nil
}
```

### 2. Empty JWT_SECRET starts the server silently
**File:** `main.go:646`

If `JWT_SECRET` or `POLKA_KEY` are missing from the environment, the server starts with empty string secrets. JWTs signed with `""` are valid but trivially forgeable.

```go
// Fix: fatal if secrets are missing
if jwtSecret == "" {
    log.Fatal("JWT_SECRET is not set")
}
if polkaKey == "" {
    log.Fatal("POLKA_KEY is not set")
}
```

### 3. `admin/reset` is unguarded in production
**File:** `main.go:66`

`POST /admin/reset` deletes all users, chirps, and tokens. There is no environment check preventing this from being called outside of a dev/test environment.

Fix: gate behind an `APP_ENV` check, or remove the route entirely when not in development.

### 4. `MakeRefreshToken` silently ignores a crypto error
**File:** `internal/auth/auth.go:91`

`rand.Read` can fail. The error is discarded, meaning a failed read would produce a zero-byte token without any indication.

```go
func MakeRefreshToken() (string, error) {
    key := make([]byte, 32)
    if _, err := rand.Read(key); err != nil {
        return "", fmt.Errorf("failed to generate refresh token: %w", err)
    }
    return hex.EncodeToString(key), nil
}
```

---

## High

### 5. `handleLogin` returns `400` when the email is not found
**File:** `main.go:445`

When `GetUserByEmail` returns an error (email not in DB), the handler returns `400 Bad Request`. It should return `401 Unauthorized` to be consistent with the password-mismatch branch. Returning 400 also leaks that the email lookup specifically failed.

### 6. Inconsistent error responses
**File:** `main.go` — multiple handlers

Handlers return error information in three different ways:
- JSON body: `{"error": "Chirp is too long"}` (`handleChirpCreate`)
- Plain text body: `Incorrect email or password` (`handleLogin`)
- Empty body: most other error paths

Clients cannot rely on a consistent response shape. A shared `respondWithError(w, code, message)` helper should be used everywhere.

### 7. All DB errors in `handleGetChirp` map to `404`
**File:** `main.go:109`

A database connection failure returns `404 Not Found` instead of `500 Internal Server Error`. Only `sql.ErrNoRows` should be a 404.

```go
if errors.Is(err, sql.ErrNoRows) {
    w.WriteHeader(404)
} else {
    w.WriteHeader(500)
}
```

### 8. DB error in `handleChirpCreate` returns `400`
**File:** `main.go:285`

A database error when saving a chirp returns `400 Bad Request`. It should be `500`.

### 9. `server.ListenAndServe()` error is silently swallowed
**File:** `main.go:684`

If the server fails to bind (e.g., port already in use), the process exits silently.

```go
log.Fatal(server.ListenAndServe())
```

### 10. Duplicate email on user create returns `400` instead of `409`
**File:** `main.go:339`

A unique-constraint violation (duplicate email) is indistinguishable from any other DB error. The response should be `409 Conflict`.

---

## Medium

### 11. Duplicate refresh token validation logic
**Files:** `main.go:495` and `main.go:550`

`handleRefreshToken` and `handleRevokeToken` both contain the same ~15-line block that checks if a token is revoked or expired. This should be extracted into a shared helper:

```go
func (cfg *apiConfig) validateRefreshToken(ctx context.Context, tokenStr string) (database.RefreshToken, error)
```

### 12. `GetBearerToken` and `GetAPIKey` are identical
**File:** `internal/auth/auth.go:74` and `:95`

Both functions parse the `Authorization` header the same way. Neither validates the scheme prefix (`Bearer` vs `ApiKey`). Deduplicate into one function:

```go
func extractAuthHeader(headers http.Header, scheme string) (string, error)
```

### 13. Spurious log noise from `handleGetChirps`
**File:** `main.go:173`

When no `author_id` query param is provided (the normal case), `uuid.Parse("")` fails and logs `"could not parse author_id query param"`. Every request to `GET /api/chirps` without an `author_id` will produce this log line.

Fix: only attempt to parse if the param is non-empty.

```go
if s := r.URL.Query().Get("author_id"); s != "" {
    userId, err := uuid.Parse(s)
    ...
}
```

### 14. Response types redeclared in every handler
**File:** `main.go` — multiple handlers

`returnVals`, `chirp`, `parameters` structs are defined locally inside each handler function. Several are structurally identical (e.g., the user response shape in `handleUserCreate`, `handleUserUpdate`, `handleLogin`). These should be package-level types.

### 15. `main.go` is a 700-line monolith
**File:** `main.go`

All route handlers, middleware, and `main()` live in one file. As the project grows this becomes hard to navigate. A conventional layout would be:

```
handlers/
  chirps.go
  users.go
  auth.go
  webhooks.go
```

### 16. All dependencies are marked `// indirect`
**File:** `go.mod`

Direct dependencies (`argon2id`, `jwt`, `uuid`, `godotenv`, `pq`) are incorrectly flagged as `// indirect`. Run `go mod tidy` to fix.

---

## Low

### 17. Schema uses `TIMESTAMP` without timezone
**Files:** `sql/schema/*.sql`

All timestamp columns use `TIMESTAMP` (no timezone). PostgreSQL stores these as-is without timezone info, which can cause subtle bugs when the server or DB timezone changes. Prefer `TIMESTAMPTZ`.

### 18. Schema columns missing `NOT NULL` constraints
**Files:** `sql/schema/001_users.sql`, `002_chirps.sql`, `004_refresh_tokens..sql`

`created_at`, `updated_at`, `email`, `body`, and `user_id` are nullable in the schema even though they should never be null. This is why sqlc generates `sql.NullTime`, `sql.NullString`, and `uuid.NullUUID` for fields that are always set — forcing unnecessary `.Time`, `.String`, and `.UUID` unwrapping throughout the codebase.

Fix: add `NOT NULL` to these columns in new migrations.

### 19. Schema filename typo
**File:** `sql/schema/004_refresh_tokens..sql`

Double dot in the filename. Rename to `004_refresh_tokens.sql`.

### 20. Magic numbers for token durations
**File:** `main.go:461`, `main.go:471`, `main.go:529`

`time.Duration(1 * 60 * 60 * time.Second)` and `60 * 24 * time.Hour` are repeated. Use named constants:

```go
const (
    accessTokenTTL  = time.Hour
    refreshTokenTTL = 60 * 24 * time.Hour
)
```

### 21. Comment typos and copy-paste errors
**Files:** `internal/auth/auth.go:98`, `main.go:513`

- `"authrorization"` typo in both `GetBearerToken` and `GetAPIKey`
- `handleRefreshToken` has `// Token revoked` as the comment for the `ExpiresAt.Valid == false` check (should be something like `// Token has no expiry`)

### 22. No request body size limit
**File:** `main.go` — all POST/PUT handlers

There is no `http.MaxBytesReader` on any request body. A large request body will be read entirely into memory.

```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB
```

### 23. No input validation on user create / update
**File:** `main.go:304`, `main.go:358`

- Email is not validated (empty string accepted, no format check)
- Password has no minimum length requirement
