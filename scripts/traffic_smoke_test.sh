#!/usr/bin/env bash
# Brings up two versions of backend-a plus a dynamically-configured Envoy,
# sets a weighted canary split, measures the ACTUAL observed traffic
# distribution over real requests, shifts the split, and re-measures --
# proving weighted routing works live with zero Envoy restarts. All
# percentages printed are measured, never invented.
set -euo pipefail

COMPOSE_FILE="deployments/docker/docker-compose-traffic.yml"
SAMPLE_SIZE=200
cd "$(dirname "$0")/.."

HB_PID=""
cleanup() {
  [[ -n "$HB_PID" ]] && kill "$HB_PID" 2>/dev/null || true
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

V1_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' docker-backend-a-v1-1)
V2_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' docker-backend-a-v2-1)

echo "==> registering backend-a v1 ($V1_IP) and v2 ($V2_IP)"
curl -sf -X POST localhost:8080/v1/services -d "{\"service_name\":\"backend-a\",\"instance_id\":\"v1-i1\",\"address\":\"$V1_IP\",\"port\":9000,\"version\":\"v1\"}" >/dev/null
curl -sf -X POST localhost:8080/v1/services -d "{\"service_name\":\"backend-a\",\"instance_id\":\"v2-i1\",\"address\":\"$V2_IP\",\"port\":9000,\"version\":\"v2\"}" >/dev/null

# Keep both instances heartbeating so they stay in EDS for the duration of
# the measurement (the registry marks instances stale after 15s otherwise).
(while true; do
  curl -s -X POST localhost:8080/v1/services/backend-a/instances/v1-i1/heartbeat >/dev/null
  curl -s -X POST localhost:8080/v1/services/backend-a/instances/v2-i1/heartbeat >/dev/null
  sleep 3
done) &
HB_PID=$!

measure() {
  local v1=0 v2=0 other=0
  for i in $(seq 1 "$SAMPLE_SIZE"); do
    local v
    v=$(curl -s -m 2 localhost:20000/echo | python3 -c "import sys,json; print(json.load(sys.stdin).get('version',''))" 2>/dev/null || true)
    case "$v" in
      v1) v1=$((v1+1)) ;;
      v2) v2=$((v2+1)) ;;
      *) other=$((other+1)) ;;
    esac
  done
  echo "$v1 $v2 $other"
}

echo "==> setting 90/10 canary split"
curl -sf -X PUT localhost:8080/v1/routes/backend-a -d '{"splits":[{"version":"v1","weight":90},{"version":"v2","weight":10}]}' >/dev/null
echo "==> waiting for propagation"
sleep 4
read -r v1 v2 other < <(measure)
echo "measured (90/10 configured): v1=$v1 v2=$v2 other=$other / $SAMPLE_SIZE"
[[ "$other" -eq 0 ]] || { echo "FAIL: $other requests had no answer"; exit 1; }
[[ "$v1" -gt "$v2" ]] || { echo "FAIL: expected v1 majority at 90/10, got v1=$v1 v2=$v2"; exit 1; }

echo "==> shifting to 50/50 canary split (no Envoy restart)"
curl -sf -X PUT localhost:8080/v1/routes/backend-a -d '{"splits":[{"version":"v1","weight":50},{"version":"v2","weight":50}]}' >/dev/null
echo "==> waiting for propagation"
sleep 4
read -r v1b v2b otherb < <(measure)
echo "measured (50/50 configured): v1=$v1b v2=$v2b other=$otherb / $SAMPLE_SIZE"
[[ "$otherb" -eq 0 ]] || { echo "FAIL: $otherb requests had no answer"; exit 1; }
diff=$(( v1b > v2b ? v1b - v2b : v2b - v1b ))
[[ "$diff" -lt 40 ]] || { echo "FAIL: 50/50 split too skewed (diff=$diff)"; exit 1; }
[[ "$v1b" -lt "$v1" ]] || { echo "FAIL: expected v1 share to drop after shifting to 50/50 (v1 was $v1, now $v1b)"; exit 1; }

echo "==> all traffic-management smoke tests passed (measured, not invented)"
