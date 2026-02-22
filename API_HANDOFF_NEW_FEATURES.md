# Sentinel API — New & Changed Endpoints

This document covers the newly implemented and modified endpoints for Account Deletion, Event History Deletion, Event Pagination, and File Downloading.

---

## 1. Delete Account
Permanently deletes the authenticated user account and all associated data (including active events, location history, saved contacts, and stored S3 files).

```http
DELETE /account
```

**Request Body**
```json
{
  "confirm": true
}
```

**Status Codes**
- **`200 OK`**: Account deleted successfully.
- **`400 Bad Request`**: Returned with `{"error": "confirmation required"}` if `confirm` is omitted, false, or if the request is malformed.
- **`401 Unauthorized`**: Missing or invalid authentication token.
- **`500 Internal Server Error`**: Server error during the deletion cascade.

**Response (200 OK)**
```json
{
  "message": "account deleted successfully"
}
```

**Notes:** If the user has active events, they are silently terminated and canceled to prevent spurious alerts. All staged and attached files belonging to the user are physically removed from S3.

---

## 2. Delete Event History
Deletes a specific terminated event and permanently removes its associated DB records (contacts/endpoints) and S3 file attachments.

```http
DELETE /events/:id/history
```

**Request Body**: None

**Status Codes**
- **`200 OK`**: Event history deleted.
- **`400 Bad Request`**: Returned with `{"error": "cannot delete an active event"}` if the event has not yet been terminated. Also returned if the UUID format is invalid.
- **`401 Unauthorized`**: Missing or invalid token.
- **`404 Not Found`**: Event not found or not owned by the authenticated user.
- **`500 Internal Server Error`**: S3 deletion or DB cascade failure.

**Response (200 OK)**
```json
{
  "message": "event history deleted"
}
```

**Notes:** This endpoint does *not* replace the existing `DELETE /events/:id` endpoint (which is used for canceling active events). This endpoint is exclusively for cleaning up events that are already in the "terminated" state.

---

## 3. Paginated Event Listing (Modified Endpoint)

The existing `GET /events` endpoint has been modified to support cursor pagination while maintaining backward compatibility.

```http
GET /events
GET /events?filter=active
GET /events?page=2&limit=10
```

**Query Parameters**
- `filter` (string, optional): Event mode to fetch (`all`, `active`). Defaults to `all`.
- `page` (integer, optional): The page number to fetch. Defaults to `1`.
- `limit` (integer, optional): The number of items per page. Defaults to `20`.

**Status Codes**
- **`200 OK`**: Success.
- **`400 Bad Request`**: Invalid page or limit parameters.
- **`401 Unauthorized`**: Missing or invalid token.
- **`500 Internal Server Error`**: Unknown DB error while counting or querying.

**Response (200 OK)**
```json
{
  "events": [
    {
      "id": "c90d38e7-...",
      "name": "Night Hike",
      "checked_in": true,
      "terminated": true
    }
  ],
  "total_count": 42,
  "page": 1,
  "limit": 20,
  "has_more": true
}
```

**Backward Compatibility Notes:** If `page` and `limit` are not provided, the API safely defaults to `page=1` and `limit=20`. If a requested page exceeds the total available pages, the API correctly returns an empty `events` array with `has_more: false`.

---

## 4. Download File
Allows downloading a single file (whether currently staged or attached to an event) as a streamed attachment, provided the user owns it.

```http
GET /files/:id/download
```

**Request Body**: None

**Response Format**
Binary file stream (`[]byte`). Contains the correct MIME type in `Content-Type` and includes a `Content-Disposition` header with the original filename to prompt a download.

**Status Codes**
- **`200 OK`**: File byte stream returned successfully.
- **`400 Bad Request`**: Invalid UUID format.
- **`401 Unauthorized`**: Missing or invalid token.
- **`404 Not Found`**: File not found or not owned by user.
- **`500 Internal Server Error`**: AWS S3 download error.

**Headers on Success**
```http
Content-Type: application/pdf
Content-Disposition: attachment; filename="passport_scan.pdf"
```
