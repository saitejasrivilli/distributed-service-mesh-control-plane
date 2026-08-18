#!/usr/bin/env bash
# Proves the v0.6.0 health-aware reconciliation loop against real Docker
# containers: a backend with no heartbeats goes stale -> unhealthy ->
# excluded from Envoy's EDS (zero restart), then a heartbeat recovers it.
set -euo pipefail

COMPOSE_FILE="deployments/docker/docker-compose-xds.yml"
cd "$(dirname "$0")/.."

cleanup() {
  docker compose -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "==> starting stack (CP_STALE_AFTER=5s baked into docker-compose-xds.yml)"
docker compose -f "$COMPOSE_FILE" up -d --build

echo "==> waiting for control-plane HTTP API"
for i in $(seq 1 30); do
  curl -sf localhost:8080/readyz >/dev/null 2>&1 && break
  sleep 1
done

BACKEND_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' docker-backend-a-1)
echo "==> registering backend-a ($BACKEND_IP)"
curl -sf -X POST localhost:8080/v1/services -d "{\"service_name\":\"backend-a\",\"instance_id\":\"i1\",\"address\":\"$BACKEND_IP\",\"port\":9000}" >/dev/null
sleep 2

echo "==> test: healthy instance serves traffic"
code=$(curl -s -o /dev/null -w "%{http_code}" -m 2 localhost:20000/echo)
[[ "$code" == "200" ]] || { echo "FAIL: expected 200, got $code"; exit 1; }
echo "OK: 200"

echo "==> waiting 10s with no heartbeat (staleAfter=5s)"
sleep 10

healthy=$(curl -s localhost:8080/v1/services/backend-a | python3 -c "import sys,json; print(json.load(sys.stdin)['instances'][0]['Healthy'])")
[[ "$healthy" == "False" ]] || { echo "FAIL: expected instance to be marked unhealthy, Healthy=$healthy"; exit 1; }
echo "OK: instance transitioned to Healthy=false after missed heartbeats"

code=$(curl -s -o /dev/null -w "%{http_code}" -m 2 localhost:20000/echo || echo "000")
[[ "$code" != "200" ]] || { echo "FAIL: expected traffic to avoid the stale instance, got 200"; exit 1; }
echo "OK: Envoy excludes the stale instance (no restart)"

echo "==> sending heartbeat to recover"
curl -sf -X POST localhost:8080/v1/services/backend-a/instances/i1/heartbeat >/dev/null
sleep 3

healthy=$(curl -s localhost:8080/v1/services/backend-a | python3 -c "import sys,json; print(json.load(sys.stdin)['instances'][0]['Healthy'])")
[[ "$healthy" == "True" ]] || { echo "FAIL: expected instance to recover to healthy, Healthy=$healthy"; exit 1; }

code=$(curl -s -o /dev/null -w "%{http_code}" -m 2 localhost:20000/echo)
[[ "$code" == "200" ]] || { echo "FAIL: expected 200 after recovery, got $code"; exit 1; }
echo "OK: instance recovered, Envoy resumed routing traffic (no restart)"

echo "==> all health-reconciliation smoke tests passed"
