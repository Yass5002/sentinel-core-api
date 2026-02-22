#!/bin/bash

BASE="https://sentinel.yssn.tech"
EMAIL="yass.elouazzani@outlook.com"
REDIS_HOST="sentinel-redis.ztdhth.0001.use1.cache.amazonaws.com"
PASS=0
FAIL=0

check() {
  local name="$1" expected="$2" actual="$3"
  if echo "$actual" | grep -q "$expected"; then
    echo "  ✅ $name"
    ((PASS++))
  else
    echo "  ❌ $name — expected '$expected', got: $actual"
    ((FAIL++))
  fi
}

echo "============================================"
echo "  SENTINEL API STRESS TEST"
echo "============================================"
echo

# --- 1. Health ---
echo "--- 1. Health Check ---"
R=$(curl -s $BASE/health)
check "GET /health" '"status":"ok"' "$R"

# --- 2. Send OTP ---
echo "--- 2. Send OTP ---"
# Clear rate limit first
redis-cli -h $REDIS_HOST DEL "otp_rate:$EMAIL" > /dev/null 2>&1
R=$(curl -s -X POST $BASE/auth/otp/send -H "Content-Type: application/json" -d "{\"email\":\"$EMAIL\"}")
check "POST /auth/otp/send" '"message":"OTP sent"' "$R"

# --- 3. Rate limit test ---
echo "--- 3. Rate Limit ---"
R=$(curl -s -X POST $BASE/auth/otp/send -H "Content-Type: application/json" -d "{\"email\":\"$EMAIL\"}")
check "Rate limited on second send" 'please wait' "$R"

# --- 4. Verify OTP ---
echo "--- 4. Verify OTP ---"
OTP=$(redis-cli -h $REDIS_HOST GET "otp:$EMAIL" | tr -d '"')
echo "  OTP from Redis: $OTP"
R=$(curl -s -X POST $BASE/auth/otp/verify -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"otp\":\"$OTP\",\"name\":\"Yassine El Ouazzani\"}")
check "POST /auth/otp/verify" '"token"' "$R"
TOKEN=$(echo "$R" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
  echo "  ❌ FATAL: No token received. Cannot continue."
  exit 1
fi
echo "  Token: ${TOKEN:0:40}..."

# --- 5. Bad token ---
echo "--- 5. Auth Error (bad token) ---"
R=$(curl -s $BASE/profile -H "Authorization: Bearer badtoken123")
check "Rejected bad token" 'error' "$R"

# --- 6. Get Profile ---
echo "--- 6. Get Profile ---"
R=$(curl -s $BASE/profile -H "Authorization: Bearer $TOKEN")
check "GET /profile" '"email":"yass.elouazzani@outlook.com"' "$R"

# --- 7. Update Profile ---
echo "--- 7. Update Profile ---"
R=$(curl -s -X PATCH $BASE/profile -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"name":"Yassine E."}')
check "PATCH /profile" '"message"' "$R"

# --- 8. Verify profile update ---
echo "--- 8. Verify Profile Update ---"
R=$(curl -s $BASE/profile -H "Authorization: Bearer $TOKEN")
check "Name updated" '"name":"Yassine E."' "$R"

# --- 9. Update Location ---
echo "--- 9. Update Location ---"
R=$(curl -s -X POST $BASE/location -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"lat":33.5731,"lng":-7.5898,"accuracy":15.0}')
check "POST /location" '"message"' "$R"

# --- 10. Get Location ---
echo "--- 10. Get Location ---"
R=$(curl -s $BASE/location -H "Authorization: Bearer $TOKEN")
check "GET /location" '"lat":33.5731' "$R"

# --- 11. Upload File ---
echo "--- 11. Upload File ---"
echo "Test attachment content for stress test." > /tmp/sentinel_test.txt
R=$(curl -s -X POST $BASE/files -H "Authorization: Bearer $TOKEN" \
  -F "file=@/tmp/sentinel_test.txt")
check "POST /files" '"filename"' "$R"
FILE_ID=$(echo "$R" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  Staged File ID: $FILE_ID"

# --- 12. List Staged Files ---
echo "--- 12. List Staged Files ---"
R=$(curl -s $BASE/files -H "Authorization: Bearer $TOKEN")
check "GET /files" "$FILE_ID" "$R"

# --- 13. Create Event ---
echo "--- 13. Create Event ---"
R=$(curl -s -X POST $BASE/events -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Stress Test Night Hike\",\"message\":\"Stress test — ignore this.\",\"duration_ms\":300000,\"contacts\":[{\"email\":\"$EMAIL\",\"name\":\"Test Contact\"}],\"file_ids\":[\"$FILE_ID\"]}")
check "POST /events" '"id"' "$R"
EVENT_ID=$(echo "$R" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  Event ID: $EVENT_ID"

# --- 14. Get Event (with attachment) ---
echo "--- 14. Get Event (with attachment) ---"
R=$(curl -s $BASE/events/$EVENT_ID -H "Authorization: Bearer $TOKEN")
check "GET /events/:id" "$EVENT_ID" "$R"
check "Event has attachment" '"attachments"' "$R"

# --- 15. List Active Events ---
echo "--- 15. List Active Events ---"
R=$(curl -s "$BASE/events?filter=active" -H "Authorization: Bearer $TOKEN")
check "GET /events?filter=active" "$EVENT_ID" "$R"

# --- 16. Check In ---
echo "--- 16. Check In ---"
R=$(curl -s -X POST $BASE/events/$EVENT_ID/checkin -H "Authorization: Bearer $TOKEN")
check "POST /events/:id/checkin" '"message"' "$R"

# --- 17. Check In again (should fail) ---
echo "--- 17. Check In Again (expect 404) ---"
R=$(curl -s -X POST $BASE/events/$EVENT_ID/checkin -H "Authorization: Bearer $TOKEN")
check "Already terminated" 'error' "$R"

# --- 18. Create Event for Cancel ---
echo "--- 18. Create Event for Cancel ---"
R=$(curl -s -X POST $BASE/events -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Cancel Test Event\",\"message\":\"Cancel test.\",\"duration_ms\":600000,\"contacts\":[{\"email\":\"$EMAIL\",\"name\":\"Test\"}]}")
EVENT_ID2=$(echo "$R" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)
echo "  Event ID2: $EVENT_ID2"

# --- 19. Cancel Event ---
echo "--- 19. Cancel Event ---"
R=$(curl -s -X DELETE $BASE/events/$EVENT_ID2 -H "Authorization: Bearer $TOKEN")
check "DELETE /events/:id" '"message"' "$R"

# --- 20. Cancel again (should fail) ---
echo "--- 20. Cancel Again (expect 404) ---"
R=$(curl -s -X DELETE $BASE/events/$EVENT_ID2 -H "Authorization: Bearer $TOKEN")
check "Already cancelled" 'error' "$R"

# --- 21. List All Events ---
echo "--- 21. List All Events ---"
R=$(curl -s "$BASE/events" -H "Authorization: Bearer $TOKEN")
check "GET /events (all)" '"events"' "$R"

# --- 22. Get nonexistent event ---
echo "--- 22. 404 Test ---"
R=$(curl -s $BASE/events/00000000-0000-0000-0000-000000000000 -H "Authorization: Bearer $TOKEN")
check "Nonexistent event" 'error' "$R"

# --- 23. Restore profile name ---
echo "--- 23. Restore Profile Name ---"
R=$(curl -s -X PATCH $BASE/profile -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" -d '{"name":"Yassine El Ouazzani"}')
check "Restored name" '"message"' "$R"

echo
echo "============================================"
echo "  RESULTS: $PASS passed, $FAIL failed"
echo "============================================"
