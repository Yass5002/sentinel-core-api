#!/bin/bash
OTP="416675"
echo "Verifying OTP ($OTP)..."
JWT=$(curl -s -X POST https://sentinel.yssn.tech/auth/otp/verify -H "Content-Type: application/json" -d '{"email": "yass.elouazzani@outlook.com", "otp": "'"$OTP"'", "name": "Yassine"}' | python3 -c "import sys, json; print(json.load(sys.stdin).get('token', 'ERROR'))")

if [ "$JWT" == "ERROR" ]; then
    echo "Failed to get token"
    exit 1
fi
echo "Successfully authenticated!"

echo -e "\n1. POST /events => Creating a temporary Event"
EVENT_RESP=$(curl -s -X POST https://sentinel.yssn.tech/events -H "Content-Type: application/json" -H "Authorization: Bearer $JWT" -d '{"name": "Live Test Event", "duration_ms": 60000, "contacts": [{"email": "fake@test.com"}]}')
echo "$EVENT_RESP"
EVENT_ID=$(echo "$EVENT_RESP" | python3 -c "import sys, json; print(json.load(sys.stdin).get('id', ''))")

echo -e "\n\n2. GET /events?page=1&limit=10 => Listing Paginated Events"
curl -s -X GET "https://sentinel.yssn.tech/events?page=1&limit=10" -H "Authorization: Bearer $JWT"

echo -e "\n\n\n3. POST /events/$EVENT_ID/checkin => Checking in to safely terminate"
curl -s -X POST "https://sentinel.yssn.tech/events/$EVENT_ID/checkin" -H "Authorization: Bearer $JWT"

echo -e "\n\n4. DELETE /events/$EVENT_ID/history => Deleting Terminated History"
curl -s -X DELETE "https://sentinel.yssn.tech/events/$EVENT_ID/history" -H "Authorization: Bearer $JWT"

echo -e "\n\n5. DELETE /account => Completely wiping User Account Data from DB & Attachments"
curl -s -X DELETE https://sentinel.yssn.tech/account -H "Content-Type: application/json" -H "Authorization: Bearer $JWT" -d '{"confirm": true}'
