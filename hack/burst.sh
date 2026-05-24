#!/usr/bin/env bash
# Real-churn context shot: delete a large slice of kwok-backed pods at once to
# create a correlated event storm (the "gang failure / preemption" analogue).
# This produces REAL workqueue backlog and brief REAL cache lag — use as the
# "storms are real" establishing shot, not the precise causal demo.
# Usage: ./burst.sh <count>
set -euo pipefail
COUNT="${1:-200}"
echo "killing ${COUNT} pods simultaneously to trigger a reconcile storm..."
kubectl get pods -A -l app.kubernetes.io/managed-by=fakeworkload-controller \
  -o name | head -n "${COUNT}" | xargs -P 50 -n 1 kubectl delete --wait=false
echo "burst issued."
