#!/bin/sh
set -eu

url="${1:-http://admin:8082/admin/api/health}"
shift || true

while ! wget -q -O /dev/null "$url"; do
  echo "waiting for admin api at $url"
  sleep 1
done

exec "$@"
