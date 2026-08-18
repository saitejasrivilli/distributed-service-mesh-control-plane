#!/usr/bin/env bash
# Builds the mesh image, loads it into a kind cluster, deploys control-plane
# + backend-a (3 replicas) + k8s-watcher + envoy-dynamic, and proves:
#   - pod IPs are discovered dynamically (never hardcoded) via k8s-watcher
#   - traffic flows client -> Envoy -> k8s-discovered backends
#   - scaling 3 -> 5 -> 2 replicas is reflected in the registry and xDS,
#     with traffic continuing throughout, no manual intervention
# Requires: docker, kind, kubectl. Uses (and leaves running) a kind cluster
# named "mesh-demo" unless KEEP_CLUSTER=0 is set.
set -euo pipefail
cd "$(dirname "$0")/.."

CLUSTER_NAME="mesh-demo"
KEEP_CLUSTER="${KEEP_CLUSTER:-1}"

cleanup() {
  kill "${PF1:-0}" "${PF2:-0}" 2>/dev/null || true
  if [[ "$KEEP_CLUSTER" != "1" ]]; then
    kind delete cluster --name "$CLUSTER_NAME" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

if ! kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}\$"; then
  echo "==> creating kind cluster $CLUSTER_NAME"
  kind create cluster --name "$CLUSTER_NAME"
fi
kubectl config use-context "kind-${CLUSTER_NAME}" >/dev/null

echo "==> building and loading image"
docker build -t mesh/control-plane:dev .
kind load docker-image mesh/control-plane:dev --name "$CLUSTER_NAME"

echo "==> applying manifests"
kubectl apply -f deployments/kubernetes/control-plane.yaml \
              -f deployments/kubernetes/backend-a.yaml \
              -f deployments/kubernetes/k8s-watcher.yaml \
              -f deployments/kubernetes/envoy-dynamic.yaml

echo "==> waiting for rollouts"
kubectl rollout status deploy/control-plane --timeout=90s
kubectl rollout status deploy/backend-a --timeout=90s
kubectl rollout status deploy/k8s-watcher --timeout=90s
kubectl rollout status deploy/envoy-dynamic --timeout=90s

kubectl port-forward svc/control-plane 8080:8080 >/tmp/k8s-smoke-pf1.log 2>&1 &
PF1=$!
kubectl port-forward svc/envoy-dynamic 20000:20000 >/tmp/k8s-smoke-pf2.log 2>&1 &
PF2=$!
sleep 3

count_instances() {
  curl -s localhost:8080/v1/services/backend-a | python3 -c "import sys,json; print(len(json.load(sys.stdin)['instances']))"
}

echo "==> waiting for k8s-watcher to discover 3 replicas (no hardcoded IPs)"
n=0
for i in $(seq 1 30); do
  n=$(count_instances || echo 0)
  [[ "$n" == "3" ]] && break
  sleep 1
done
[[ "$n" == "3" ]] || { echo "FAIL: expected 3 discovered instances, got $n"; exit 1; }
echo "OK: 3 instances discovered dynamically"

echo "==> test: traffic flows through Envoy to a k8s-discovered backend"
code=$(curl -s -o /dev/null -w "%{http_code}" -m 3 localhost:20000/echo)
[[ "$code" == "200" ]] || { echo "FAIL: expected 200, got $code"; exit 1; }
echo "OK: 200"

echo "==> scaling backend-a to 5 replicas"
kubectl scale deploy/backend-a --replicas=5
for i in $(seq 1 30); do
  n=$(count_instances || echo 0)
  [[ "$n" == "5" ]] && break
  sleep 1
done
[[ "$n" == "5" ]] || { echo "FAIL: expected 5 instances after scale-up, got $n"; exit 1; }
echo "OK: 5 instances discovered after scale-up"

code=$(curl -s -o /dev/null -w "%{http_code}" -m 3 localhost:20000/echo)
[[ "$code" == "200" ]] || { echo "FAIL: traffic broke during scale-up, got $code"; exit 1; }
echo "OK: traffic still flowing during scale-up"

echo "==> scaling backend-a to 2 replicas"
kubectl scale deploy/backend-a --replicas=2
for i in $(seq 1 30); do
  n=$(count_instances || echo 0)
  [[ "$n" == "2" ]] && break
  sleep 1
done
[[ "$n" == "2" ]] || { echo "FAIL: expected 2 instances after scale-down, got $n"; exit 1; }
echo "OK: 2 instances discovered after scale-down"

code=$(curl -s -o /dev/null -w "%{http_code}" -m 3 localhost:20000/echo)
[[ "$code" == "200" ]] || { echo "FAIL: traffic broke during scale-down, got $code"; exit 1; }
echo "OK: traffic still flowing during scale-down"

echo "==> all Kubernetes smoke tests passed"
