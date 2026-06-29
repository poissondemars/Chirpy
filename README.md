# Chirpy

A Twitter-like REST API built in Go, featuring JWT authentication, refresh tokens, and a premium membership tier.

## What it demonstrates

- JWT access tokens + long-lived refresh tokens with revocation
- Argon2id password hashing
- Role-based access (Chirpy Red membership via Polka webhook)
- Type-safe SQL with [sqlc](https://sqlc.dev) code generation
- Database migrations with [goose](https://github.com/pressly/goose)
- Standard library `net/http` routing (no framework)

## Tech stack

| Layer | Tool |
|---|---|
| Language | Go 1.24 |
| Database | PostgreSQL |
| Query generation | sqlc |
| Migrations | goose |
| Auth | golang-jwt/jwt, alexedwards/argon2id |

## API

### Auth

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/users` | Create account |
| `PUT` | `/api/users` | Update email / password (JWT required) |
| `POST` | `/api/login` | Login — returns access + refresh tokens |
| `POST` | `/api/refresh` | Exchange refresh token for new access token |
| `POST` | `/api/revoke` | Revoke refresh token |

### Chirps

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/chirps` | List all chirps (`?author_id=<uuid>`, `?sort=asc\|desc`) |
| `GET` | `/api/chirps/{chirpId}` | Get a single chirp |
| `POST` | `/api/chirps` | Create a chirp (JWT required) |
| `DELETE` | `/api/chirps/{chirpId}` | Delete own chirp (JWT required) |

### Webhooks

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/polka/webhooks` | Polka payment webhook — upgrades user to Chirpy Red |

### Admin

| Method | Path | Description |
|---|---|---|
| `GET` | `/admin/metrics` | Fileserver hit count |
| `POST` | `/admin/reset` | Reset all data (dev only) |

## Setup

**1. Environment**

Create a `.env` file:

```
DB_URL=postgres://user:password@localhost:5432/chirpy?sslmode=disable
JWT_SECRET=your-secret-here
POLKA_KEY=your-polka-key-here
```

**2. Run migrations**

```sh
cd sql/schema && goose postgres $DB_URL up
```

**3. Generate query code**

```sh
sqlc generate
```

**4. Start the server**

```sh
go run .
```

Server listens on `:8080`.
