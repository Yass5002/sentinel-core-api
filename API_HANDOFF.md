# SENTINEL — API Reference

## Overview
Sentinel backend. User creates a timed event with contacts. If they don't check in before the timer expires, the server sends alert emails to all contacts with the user's last known GPS location and any file attachments.

---

## Base URL
```
https://sentinel.yssn.tech
```
SSL is terminated at the Cloudflare edge — the app uses standard HTTPS with no custom certificate handling.

## Authentication
All endpoints except `/auth/*` and `/health` require:
```
Authorization: Bearer <jwt_token>
```
Tokens expire after **30 days**.

---

## Endpoints

### Health Check
```
GET /health
```
```json
{"status": "ok"}
```

---

### Send OTP
```
POST /auth/otp/send
```
```json
{ "email": "user@example.com" }
```
**`200`** `{"message": "OTP sent"}`
**`429`** `{"error": "please wait before requesting another OTP"}`

- 6-digit code, expires in 5 minutes.
- Rate limited: 1 per 60 seconds per email.

---

### Verify OTP
```
POST /auth/otp/verify
```
```json
{
  "email": "user@example.com",
  "otp": "594288",
  "name": "Yassine El Ouazzani"
}
```
**`200`**
```json
{"token": "eyJhbGciOiJIUzI1NiI..."}
```
**`401`** — invalid or expired OTP.

- `name` is **required**. Stored as the user's display name on first login.
- If the user already exists, `name` is ignored (update via `PATCH /profile`).
- Creates a new account automatically if the user doesn't exist.

---

### Create Event
```
POST /events
```
```json
{
  "name": "Night Hike",
  "message": "If you receive this, I haven't checked in. Please help.",
  "duration_ms": 2700000,
  "contacts": [
    {"email": "contact1@example.com", "name": "Mom"},
    {"email": "contact2@example.com", "name": "Friend"}
  ],
  "file_ids": ["8158aad5-...", "dd6cbb7d-..."]
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name for the event (max 100 chars). Used as alert email subject line. Whitespace-trimmed. |
| `message` | string | No | Text included in alert emails (max 2000 chars). Whitespace-trimmed. |
| `duration_ms` | int | Yes | Timer duration in milliseconds. Min: 30000 (30s). Max: 604800000 (7 days). |
| `contacts` | array | Yes (1–10) | Each contact needs `email` (required) and `name` (optional). Duplicate emails are auto-deduplicated. Emails are lowercased. |
| `file_ids` | array | No | IDs of staged files (from `POST /files`) to attach to this event. Files must belong to the authenticated user and not already be attached to another event. |

**`201`**
```json
{
  "id": "c90d38e7-...",
  "user_id": "358abb89-...",
  "name": "Night Hike",
  "message": "If you receive this...",
  "expires_at": "2026-02-11T14:00:00Z",
  "duration_ms": 2700000,
  "checked_in": false,
  "terminated": false,
  "fired": false,
  "created_at": "2026-02-11T13:15:00Z",
  "updated_at": "2026-02-11T13:15:00Z",
  "contacts": [
    {
      "id": "408d4a20-...",
      "event_id": "c90d38e7-...",
      "email": "contact1@example.com",
      "name": "Mom",
      "created_at": "2026-02-11T13:15:00Z"
    }
  ]
}
```

---

### Get Event
```
GET /events/:id
```
**`200`** — same shape as create response, includes `contacts` and `attachments`.

---

### List Events
```
GET /events
GET /events?filter=active
```
- Default / `filter=all` — all events, newest first.
- `filter=active` — non-terminated only, soonest expiry first.

**`200`**
```json
{ "events": [ ... ] }
```

---

### Check In
```
POST /events/:id/checkin
```
Terminates the event. Timer stops. No alerts sent.

**`200`** `{"message": "checked in — event terminated"}`
**`404`** — not found or already terminated.

---

### Cancel Event
```
DELETE /events/:id
```
Terminates the event without checking in.

**`200`** `{"message": "event cancelled"}`

---

### Upload File (Staged)
```
POST /files
Content-Type: multipart/form-data
```
Form field: `file`

- Max size: **10 MB**.
- Files are staged (no event attached yet). Pass the returned `id` in `file_ids` when creating an event.
- Staged files are automatically cleaned up after 24 hours if not attached to an event.

**`201`**
```json
{
  "id": "8158aad5-...",
  "user_id": "358abb89-...",
  "event_id": null,
  "key": "staging/358abb89-.../abc123.pdf",
  "filename": "passport_scan.pdf",
  "file_size": 245000,
  "mime_type": "application/pdf",
  "created_at": "2026-02-17T13:20:00Z"
}
```

---

### List Staged Files
```
GET /files
```
Returns all files uploaded by the user that are not yet attached to an event.

**`200`**
```json
[
  {
    "id": "8158aad5-...",
    "user_id": "358abb89-...",
    "event_id": null,
    "filename": "passport_scan.pdf",
    "file_size": 245000,
    "mime_type": "application/pdf",
    "created_at": "2026-02-17T13:20:00Z"
  }
]
```

---

### Delete Staged File
```
DELETE /files/:id
```
Deletes a staged file. Cannot delete files already attached to an event.

**`200`** `{"message": "file deleted"}`
**`404`** — file not found, not owned by user, or already attached.
```

---

### Update Location
```
POST /location
```
```json
{
  "lat": 33.5731,
  "lng": -7.5898,
  "accuracy": 15.0
}
```
- `accuracy` in meters, optional.
- Upserts — only the latest location is stored.
- Send every 30–60 seconds while an event is active.

**`200`** `{"message": "location updated"}`

---

### Get Location
```
GET /location
```
**`200`**
```json
{
  "id": "...",
  "user_id": "...",
  "lat": 33.5731,
  "lng": -7.5898,
  "accuracy": 15.0,
  "updated_at": "2026-02-11T13:15:30Z"
}
```

---

### Get Profile
```
GET /profile
```
**`200`**
```json
{
  "id": "358abb89-...",
  "email": "user@example.com",
  "name": "Yassine El Ouazzani",
  "created_at": "2026-02-11T13:01:13Z",
  "updated_at": "2026-02-11T13:01:13Z"
}
```

---

### Update Profile
```
PATCH /profile
```
```json
{ "name": "Yassine E." }
```
**`200`** `{"message": "profile updated"}`

---

### List Saved Contacts
```
GET /contacts
```
**`200`**
```json
{
  "contacts": [
    {
      "id": "a1b2c3d4-...",
      "user_id": "358abb89-...",
      "email": "mom@example.com",
      "name": "Mom",
      "created_at": "2026-02-17T10:00:00Z",
      "updated_at": "2026-02-17T10:00:00Z"
    }
  ]
}
```

---

### Add Saved Contact
```
POST /contacts
```
```json
{ "email": "mom@example.com", "name": "Mom" }
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string | Yes | Contact email address. Must be unique per user. |
| `name` | string | No | Contact display name. |

**`201`** — returns the created contact object.
**`409`** — contact with this email already exists.

---

### Update Saved Contact
```
PATCH /contacts/:id
```
```json
{ "email": "mother@example.com", "name": "Mother" }
```
Both `email` and `name` are required in the request body (sends the full contact).

**`200`** — returns the updated contact object.
**`404`** — contact not found.
**`409`** — another contact with this email already exists.

---

### Delete Saved Contact
```
DELETE /contacts/:id
```
**`200`** `{"message": "contact deleted"}`
**`404`** — contact not found.

---

## Error Format
All errors:
```json
{"error": "description"}
```

| Status | Meaning |
|--------|---------|
| 400 | Bad request / validation error |
| 401 | Not authenticated or expired token |
| 404 | Not found or not owned by user |
| 409 | Conflict (duplicate saved contact email) |
| 429 | Rate limited |
| 500 | Server error |

---

## Key Behaviors

| Behavior | Detail |
|----------|--------|
| Timer is server-side | App shows countdown from `expires_at`. App does NOT trigger alerts. |
| Events are one-shot | After check-in or expiry, create a new event. Cannot edit active events. |
| Multiple simultaneous events | Supported. |
| Alert delivery | Server sends HTML emails to all contacts with: user's name, message, GPS (Google Maps link), file attachments, timestamps. Subject line uses event name. Each contact is retried up to 3 times on failure. |
| Event creation | Atomic — if the timer fails to be set, the event is not created (staged files remain staged). Safe to retry on error. |
| File uploads | Upload files FIRST via `POST /files`, then pass their IDs in `file_ids` when creating the event. Files are staged until attached. Stale staged files are auto-cleaned after 24h. |
| Location tracking | App must send `POST /location` periodically while any event is active. |
| Saved contacts | User can save contacts to their profile via `/contacts` endpoints. These are separate from per-event contacts — the app can use them to pre-populate the contact picker when creating an event. |
| JWT storage | Store locally. Valid for 30 days. |

---

## Current Limitations

| Limitation | Impact on App |
|------------|---------------|
| SES Sandbox Mode | Emails only send to verified addresses. Contacts must be pre-verified in AWS SES until production access is granted. |
| No pagination | `GET /events` returns all events. |
| No JWT revocation | Tokens can't be invalidated before 30-day expiry. |

---

*Last updated: February 21, 2026*
