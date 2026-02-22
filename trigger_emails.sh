#!/bin/bash
# Trigger both OTP + Alert emails to yass.elouazzani@outlook.com

BASE="http://localhost:8080"
EMAIL="yass.elouazzani@outlook.com"
REDIS="sentinel-redis.ztdhth.0001.use1.cache.amazonaws.com"

# 1. Send OTP email
echo ">>> Sending OTP email..."
curl -s -X POST $BASE/auth/otp/send -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\"}"
echo
sleep 0.5

# 2. Grab OTP from Redis and verify
OTP=$(redis-cli -h $REDIS GET "otp:$EMAIL" | tr -d '"')
echo ">>> OTP from Redis: $OTP"
RESP=$(curl -s -X POST $BASE/auth/otp/verify -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"otp\":\"$OTP\",\"name\":\"Yassine El Ouazzani\"}")
TOKEN=$(echo "$RESP" | grep -o '"token":"[^"]*"' | cut -d'"' -f4)

if [ -z "$TOKEN" ]; then
  echo "FATAL: auth failed: $RESP"
  exit 1
fi
echo ">>> Authenticated (token ${#TOKEN} chars)"

# 3. Set location so alert has GPS data
echo ">>> Setting location..."
curl -s -X POST $BASE/location -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"lat":33.5731,"lng":-7.5898,"accuracy":15.0}'
echo

# 4. Create event with 5-second timer → triggers alert on expiry
echo ">>> Creating event with 5s timer..."
curl -s -X POST $BASE/events -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"message\":\"This is a test alert from Sentinel. Please ignore.\",\"duration_ms\":5000,\"contacts\":[{\"email\":\"$EMAIL\",\"name\":\"Yassine El Ouazzani\"}]}"
echo

echo ">>> Waiting ~15s for timer to expire and alert to send..."
sleep 15

# Check logs for confirmation
echo ">>> Checking server logs..."
sudo journalctl -u sentinel --since "1 min ago" --no-pager | grep -i "alert\|expired\|fired"

echo ">>> Done. Check inbox for both OTP and alert emails."
