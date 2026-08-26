#!/bin/sh
set -eu

: "${INFRAI_API_KEY:?set INFRAI_API_KEY}"
go run ./cmd/storefrontd &
server_pid=$!
trap 'kill "$server_pid"' EXIT
sleep 1
curl --fail --silent --show-error -X POST http://localhost:8080/signup \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: signup-demo-1' \
  -d "{\"Email\":\"buyer@example.com\",\"Password\":\"change-this-password\",\"Name\":\"Buyer\",\"CaptchaToken\":\"${CAPTCHA_TOKEN:?set CAPTCHA_TOKEN}\"}"
