# Sentinel - Dead Man's Switch API

> A server-side safety check-in service. Start a timed event. If you don't check in before the timer expires, your emergency contacts are automatically notified with your last known location and any files you pre-attached.

---

## What is Sentinel?

Sentinel is a backend API for a **dead man's switch** application. A user creates a timed event with a list of emergency contacts. If they fail to check in before the countdown reaches zero, the server fires alert emails to every contact containing:

- The user's **name** and a custom **message**
- Their **last known GPS location** (Google Maps link)
- Any **file attachments** pre-uploaded by the user (documents, photos, etc.)

Designed for: solo hiking, late-night travel, remote work, or any situation where someone should know if you don't report back.

---

## Tech Stack

| Layer | Technology |
|---|---|
| Language | Go 1.24 |
| Web framework | [Gin](https://github.com/gin-gonic/gin) |
| Database | PostgreSQL (via [GORM](https://gorm.io)) |
| Cache / Timers | Redis |
| Authentication | OTP via email → JWT (HMAC-SHA256) |
| Email | AWS SES |
| File storage | AWS S3 |
| Deployment | Any Linux host; designed for AWS (RDS + ElastiCache) |

---

## Features

- **Passwordless auth** - email OTP flow, no passwords stored
- **Timed events** - server-side countdown with millisecond precision (30s → 7 days)
- **Multiple simultaneous events** - run as many as needed
- **Emergency contacts** - per-event contact list (1–10 contacts), auto-deduplicated
- **Saved address book** - reusable contact list per user account
- **File attachments** - upload files before creating an event; they're sent as email attachments on alert
- **GPS location** - upserted on each update; included in alert emails as a Google Maps link
- **Alert retry** - up to 3 retries per contact on SES failure
- **Timer reliability** - Redis keyspace notifications (primary) + backup polling sweep; atomic claim prevents duplicate sends
- **Account deletion** - full cascade delete (events, contacts, files, location)
- **Staged file cleanup** - unattached uploads are automatically purged after 24h

---

## API Overview

### Public endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Deep health check (pings DB + Redis) |
| `POST` | `/auth/otp/send` | Send a 6-digit OTP to an email address |
| `POST` | `/auth/otp/verify` | Verify OTP → returns a JWT |
| `GET` | `/privacy` | Privacy policy |
| `GET` | `/terms` | Terms of service |

### Protected endpoints (require `Authorization: Bearer <token>`)

| Method | Path | Description |
|---|---|---|
| `POST` | `/events` | Create a new timed event |
| `GET` | `/events` | List events (paginated; `?filter=active&page=1&limit=20`) |
| `GET` | `/events/:id` | Get a single event with contacts and attachments |
| `POST` | `/events/:id/checkin` | Check in - terminates the event, no alerts sent |
| `DELETE` | `/events/:id` | Cancel an active event |
| `DELETE` | `/events/:id/history` | Permanently delete a terminated event and its S3 files |
| `POST` | `/files` | Upload a staged file (`multipart/form-data`) |
| `GET` | `/files` | List staged (unattached) files |
| `DELETE` | `/files/:id` | Delete a staged file |
| `GET` | `/files/:id/download` | Download any owned file |
| `POST` | `/location` | Upsert the user's GPS coordinates |
| `GET` | `/location` | Get the user's latest GPS coordinates |
| `GET` | `/profile` | Get user profile |
| `PATCH` | `/profile` | Update display name |
| `DELETE` | `/account` | Permanently delete account and all associated data |
| `GET` | `/contacts` | List saved contacts |
| `POST` | `/contacts` | Add a saved contact |
| `PATCH` | `/contacts/:id` | Update a saved contact |
| `DELETE` | `/contacts/:id` | Delete a saved contact |

All errors follow the shape `{"error": "description"}`.

---

## Self-Hosting

### Prerequisites

- Go 1.24+
- PostgreSQL 14+
- Redis 7+
- AWS account with SES and S3 configured

### 1. Clone and configure

```bash
git clone https://github.com/Yass5002/sentinel-core-api.git
cd sentinel-core-api
cp .env.example .env
```

Fill in all required values in `.env` - see [Environment Variables](#environment-variables) below.

### 2. Run database migrations

GORM auto-migrates on startup. No manual migration step is required for a fresh database.

### 3. Build and run

```bash
go build -o sentinel ./...
./sentinel
```

The server starts on the port defined by `PORT` (default: `8080`).

---

## Environment Variables

Copy `.env.example` to `.env` and fill in the values. Variables marked **required** will cause the server to exit on startup if missing.

| Variable | Required | Default | Description |
|---|---|---|---|
| `PORT` | | `8080` | HTTP port to listen on |
| `GIN_MODE` | | `debug` | Set to `release` in production |
| `DB_HOST` | **yes** | | PostgreSQL host |
| `DB_PORT` | | `5432` | PostgreSQL port |
| `DB_USER` | **yes** | | PostgreSQL user |
| `DB_PASSWORD` | **yes** | | PostgreSQL password |
| `DB_NAME` | **yes** | | PostgreSQL database name |
| `DB_SSLMODE` | | `require` | PostgreSQL SSL mode |
| `REDIS_ADDR` | **yes** | | Redis address (`host:port`) |
| `REDIS_PASSWORD` | | | Redis password (if any) |
| `REDIS_DB` | | `0` | Redis database index |
| `JWT_SECRET` | **yes** | | HMAC-SHA256 signing key for JWTs |
| `AWS_REGION` | **yes** | | AWS region |
| `AWS_PROFILE` | | | AWS named profile (for local dev) |
| `S3_BUCKET` | **yes** | | S3 bucket name for file attachments |
| `SES_FROM_OTP` | **yes** | | Sender address for OTP emails |
| `SES_FROM_ALERT` | **yes** | | Sender address for alert emails |
| `EMAIL_ASSETS_BASE_URL` | | | Base URL for email template logo images |
| `CORS_ALLOWED_ORIGINS` | | `http://localhost:3000` | Comma-separated list of allowed CORS origins |
| `OTP_LENGTH` | | `6` | Number of digits in OTP codes |
| `OTP_EXPIRY_MINUTES` | | `5` | OTP validity window in minutes |
| `MAX_FILE_SIZE_MB` | | `10` | Maximum file upload size in MB |
| `TEMPLATES_DIR` | | `internal/templates/html` | Path to the HTML email templates directory |

### Redis keyspace notifications

Sentinel uses Redis keyspace notifications to detect timer expiry. Make sure your Redis instance has the following config set:

```
notify-keyspace-events KEA
```

Or set it at runtime:

```bash
redis-cli config set notify-keyspace-events KEA
```

---

## Project Structure

```
.
├── internal/
│   ├── apperrors/     # Typed sentinel errors for clean handler dispatch
│   ├── cache/         # Redis client initialisation
│   ├── config/        # Environment variable loading
│   ├── database/      # PostgreSQL connection and GORM setup
│   ├── handlers/      # HTTP handlers (auth, events, files, location, profile, contacts)
│   ├── middleware/     # JWT auth middleware
│   ├── models/        # GORM model definitions
│   ├── router/        # Route registration and CORS configuration
│   ├── services/      # Business logic (auth, events, timers, email, files, location, profile, contacts)
│   └── templates/     # HTML email template engine (OTP + alert)
└── web/
    └── static/        # Static HTML pages (privacy policy, terms of service)
```

---

## Key Behaviours

| Behaviour | Detail |
|---|---|
| Timers are server-side | The client displays a countdown from `expires_at`. Alerts are fired by the server, not the client. |
| Events are one-shot | After check-in or expiry, create a new event. Active events cannot be edited. |
| File upload flow | Upload files first via `POST /files` (staged), then pass their IDs in `file_ids` when creating an event. |
| Location tracking | The client should call `POST /location` every 30–60 seconds while any event is active. |
| Alert delivery | Each contact receives an HTML email with the user's name, message, GPS link, and file attachments. Failed sends are retried up to 3 times per contact. |
| JWT validity | Tokens are valid for 30 days. There is no revocation mechanism. |

---

## Related

| Repo | Description |
|---|---|
| [Yass5002/sentinel](https://github.com/Yass5002/sentinel) | Flutter (Dart) frontend app |
| [Yass5002/sentinel-core-api](https://github.com/Yass5002/sentinel-core-api) | This repo - backend API |

---

## License

© 2026 Yass5002. All rights reserved. See [LICENSE](./LICENSE).
