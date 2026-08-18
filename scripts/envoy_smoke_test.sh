#!/usr/bin/env bash
# Brings up the Envoy sidecar demo stack, exercises every v0.3.0 connectivity
# and failure scenario against it, and tears it down. Exits non-zero on any
# unexpected result.
set -euo pipefail

COMPOSE_FILE="deployments/docker/docker-compose.yml"
cd "$(dirname "$0")/.."

cleanup() {
  docker compose -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> starting stack"
docker compose -f "$COMPOSE_FILE" up -d --build

echo "==> waiting for envoy listeners"
for i in $(seq 1 30); do
  if curl -sf localhost:10000/echo >/dev/null 2>&1 && curl -sf localhost:10001/echo >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

echo "==> test: client -> envoy-a -> backend-a"
resp=$(curl -sf localhost:10000/echo)
[[ "$resp" == '{"service":"backend-a"}' ]] || { echo "FAIL: got $resp"; exit 1; }
echo "OK: $resp"

echo "==> test: client -> envoy-a -> envoy-b -> backend-b"
resp=$(curl -sf localhost:10000/via-b)
[[ "$resp" == '{"service":"backend-b"}' ]] || { echo "FAIL: got $resp"; exit 1; }
echo "OK: $resp"

echo "==> test: backend-a failure -> 503 via envoy-a"
docker stop docker-backend-a-1 >/dev/null
sleep 2
code=$(curl -s -o /dev/null -w "%{http_code}" localhost:10000/echo)
[[ "$code" == "503" ]] || { echo "FAIL: expected 503, got $code"; exit 1; }
echo "OK: 503 while backend-a is down"

echo "==> test: backend-a recovery -> 200 via envoy-a"
docker start docker-backend-a-1 >/dev/null
sleep 5
code=$(curl -s -o /dev/null -w "%{http_code}" localhost:10000/echo)
[[ "$code" == "200" ]] || { echo "FAIL: expected 200 after recovery, got $code"; exit 1; }
echo "OK: 200 after backend-a recovery"

echo "==> test: envoy config validation rejects malformed config"
tmp=$(mktemp)
cat > "$tmp" <<'YAML'
static_resources:
  listeners:
    - name: bad
      this_is_not_valid: true
YAML
if docker run --rm -v "$tmp:/etc/envoy/envoy.yaml:ro" envoyproxy/envoy:v1.31-latest --mode validate -c /etc/envoy/envoy.yaml >/dev/null 2>&1; then
  echo "FAIL: malformed config was accepted"
  exit 1
fi
echo "OK: malformed config rejected"

echo "==> all envoy smoke tests passed"
