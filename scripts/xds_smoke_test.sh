#!/usr/bin/env bash
# Brings up the dynamic xDS stack (control-plane + envoy-dynamic + backend-a),
# proves Envoy picks up registry changes via CDS/EDS/LDS/RDS with zero
# restarts, and tears down. Exits non-zero on any unexpected result.
set -euo pipefail

COMPOSE_FILE="deployments/docker/docker-compose-xds.yml"
cd "$(dirname "$0")/.."

cleanup() {
  docker compose -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> starting stack"
docker compose -f "$COMPOSE_FILE" up -d --build

echo "==> waiting for control-plane HTTP API"
for i in $(seq 1 30); do
  curl -sf localhost:8080/readyz >/dev/null 2>&1 && break
  sleep 1
done

echo "==> test: no listener before registration"
code=$(curl -s -o /dev/null -w "%{http_code}" -m 2 localhost:20000/echo) || true
[[ "$code" == "000" ]] || { echo "FAIL: expected no listener (000), got $code"; exit 1; }
echo "OK: 000 (connection refused, no listener yet)"

echo "==> registering backend-a"
BACKEND_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' docker-backend-a-1)
curl -sf -X POST localhost:8080/v1/services -d "{\"service_name\":\"backend-a\",\"instance_id\":\"i1\",\"address\":\"$BACKEND_IP\",\"port\":9000}" >/dev/null

echo "==> waiting for Envoy to pick up dynamic config (no restart)"
resp=""
for i in $(seq 1 20); do
  resp=$(curl -s -m 2 localhost:20000/echo || true)
  [[ "$resp" == '{"service":"backend-a","version":""}' ]] && break
  sleep 1
done
[[ "$resp" == '{"service":"backend-a","version":""}' ]] || { echo "FAIL: dynamic config never propagated, got $resp"; exit 1; }
echo "OK: $resp (via dynamically created listener/cluster/route)"

echo "==> deregistering backend-a"
curl -sf -X DELETE localhost:8080/v1/services/backend-a/instances/i1 >/dev/null

echo "==> waiting for Envoy to remove the listener"
for i in $(seq 1 20); do
  code=$(curl -s -o /dev/null -w "%{http_code}" -m 2 localhost:20000/echo) || true
  [[ "$code" == "000" ]] && break
  sleep 1
done
[[ "$code" == "000" ]] || { echo "FAIL: listener still reachable after deregistration, got $code"; exit 1; }
echo "OK: listener removed dynamically after deregistration"

echo "==> all xds smoke tests passed"
