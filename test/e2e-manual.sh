#!/usr/bin/env bash
# Manual e2e smoke test for sovereign-sensor
# Prerequisites: kind cluster running, kubectl context set, Tetragon installed
set -euo pipefail

CLUSTER=${CLUSTER_NAME:-sovereign-test}
PASS=0; FAIL=0

log()  { echo "  $1"; }
pass() { echo "  ✓ $1"; ((PASS++)); }
fail() { echo "  ✗ $1"; ((FAIL++)); }

echo ""
echo "sovereign-sensor · manual e2e smoke test"
echo "────────────────────────────────────────"

# ── 1. Install CRDs ──────────────────────────────────────────────────────────
echo ""
echo "① installing CRDs..."
kubectl apply -f config/crd/bases/ --context "kind-${CLUSTER}" > /dev/null
sleep 2

kubectl get crd sovereignsensors.sec.sovereign.io  \
  --context "kind-${CLUSTER}" &>/dev/null \
  && pass "SovereignSensor CRD installed" \
  || fail "SovereignSensor CRD missing"

kubectl get crd sovereigntypolicies.sec.sovereign.io \
  --context "kind-${CLUSTER}" &>/dev/null \
  && pass "SovereigntyPolicy CRD installed" \
  || fail "SovereigntyPolicy CRD missing"

# ── 2. Apply a test SovereignSensor ──────────────────────────────────────────
echo ""
echo "② applying SovereignSensor CR..."
kubectl apply -f config/samples/sec_v1alpha1_sovereignsensor.yaml \
  --context "kind-${CLUSTER}" > /dev/null
sleep 2
pass "SovereignSensor CR applied"

# ── 3. Apply a test policy ───────────────────────────────────────────────────
echo ""
echo "③ applying test SovereigntyPolicy..."
cat <<EOF | kubectl apply --context "kind-${CLUSTER}" -f - > /dev/null
apiVersion: sec.sovereign.io/v1alpha1
kind: SovereigntyPolicy
metadata:
  name: e2e-eu-only
spec:
  action: log
  namespaces:
    - e2e-test
  allowedCountries:
    - DE
    - IE
    - NL
  description: "e2e test policy — EU only"
EOF
sleep 1
pass "SovereigntyPolicy CR applied"

# ── 4. Create a test namespace and a curl pod ────────────────────────────────
echo ""
echo "④ spinning up test pod to generate traffic..."
kubectl create namespace e2e-test \
  --context "kind-${CLUSTER}" 2>/dev/null || true

# This pod curls a US endpoint (Google DNS) and an EU endpoint,
# giving the sensor something to catch.
kubectl run e2e-traffic-gen \
  --image=curlimages/curl:latest \
  --namespace=e2e-test \
  --context "kind-${CLUSTER}" \
  --restart=Never \
  -- sh -c "
    echo 'hitting US endpoint...'; curl -sm 3 https://8.8.8.8 || true;
    echo 'hitting EU endpoint...';  curl -sm 3 https://1.1.1.1 || true;
    sleep 5
  " > /dev/null 2>&1

kubectl wait pod/e2e-traffic-gen \
  --for=condition=Ready \
  --namespace=e2e-test \
  --context "kind-${CLUSTER}" \
  --timeout=30s 2>/dev/null || true

# Give it time to run and complete
sleep 8
pass "traffic generator pod ran"

# ── 5. Check agent logs for violations ───────────────────────────────────────
echo ""
echo "⑤ checking sensor agent logs..."

AGENT_POD=$(kubectl get pods \
  --namespace kube-system \
  --context "kind-${CLUSTER}" \
  -l app=sovereign-sensor-agent \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

if [ -z "$AGENT_POD" ]; then
  fail "no agent pod found in kube-system — is the operator running?"
else
  pass "agent pod found: ${AGENT_POD}"

  # Grab last 50 lines of logs and look for the e2e-test namespace
  LOGS=$(kubectl logs "$AGENT_POD" \
    --namespace kube-system \
    --context "kind-${CLUSTER}" \
    --tail=50 2>/dev/null || echo "")

  echo "$LOGS" | grep -q "e2e-test" \
    && pass "agent logged events from e2e-test namespace" \
    || fail "no events from e2e-test namespace in agent logs"

  echo "$LOGS" | grep -q '"violated":true' \
    && pass "at least one sovereignty violation detected" \
    || fail "no violations detected (expected US traffic to be flagged)"

  echo "$LOGS" | grep -q '"dst_country":"US"' \
    && pass "US destination country resolved correctly" \
    || fail "US country code not found in events"
fi

# ── 6. Check Prometheus metrics ───────────────────────────────────────────────
echo ""
echo "⑥ checking Prometheus metrics..."

# Port-forward in background, give it a second to bind
kubectl port-forward \
  "pod/${AGENT_POD}" 9090:9090 \
  --namespace kube-system \
  --context "kind-${CLUSTER}" &>/dev/null &
PF_PID=$!
sleep 2

METRICS=$(curl -sf http://localhost:9090/metrics 2>/dev/null || echo "")
kill $PF_PID 2>/dev/null || true

echo "$METRICS" | grep -q "sovereign_connections_total" \
  && pass "sovereign_connections_total metric present" \
  || fail "sovereign_connections_total metric missing"

echo "$METRICS" | grep -q "sovereign_evaluation_duration_seconds" \
  && pass "sovereign_evaluation_duration_seconds metric present" \
  || fail "sovereign_evaluation_duration_seconds metric missing"

# ── 7. Cleanup ────────────────────────────────────────────────────────────────
echo ""
echo "⑦ cleaning up..."
kubectl delete namespace e2e-test \
  --context "kind-${CLUSTER}" \
  --ignore-not-found > /dev/null
kubectl delete sovereigntypolicy e2e-eu-only \
  --context "kind-${CLUSTER}" \
  --ignore-not-found > /dev/null
pass "cleanup complete"

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo "────────────────────────────────────────"
echo "  passed: ${PASS}  failed: ${FAIL}"
echo ""
[[ $FAIL -eq 0 ]] && echo "  all checks passed ✓" && exit 0 \
                  || echo "  some checks failed" && exit 1