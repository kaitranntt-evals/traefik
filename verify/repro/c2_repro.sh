#!/usr/bin/env bash
# Run from the repo root: `EVAL_TESTS_DIR=/path/to/extracted/eval_tests/tests bash verify/repro/c2_repro.sh` (adds a detached worktree of evalon/traefik-ga-968ccc91, copies the eval fixtures and the two probes, prints the VERIFY-C2 lines).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
REPO_URL="${CLAIM_REPO_URL:-$(git -C "$ROOT" remote get-url origin | sed 's#/traefik\(\.git\)\?$#/traefik-gateway-listener-isolation#')}"
WT="${WT:-$ROOT/../verify-c2-wt}"
: "${EVAL_TESTS_DIR:?set EVAL_TESTS_DIR to the extracted eval_tests.zip tests/ directory}"

git -C "$ROOT" fetch "$REPO_URL" evalon/traefik-ga-968ccc91
[ -d "$WT" ] || git -C "$ROOT" worktree add --detach "$WT" FETCH_HEAD
cd "$WT"
cp "$EVAL_TESTS_DIR"/eval_https_test.go "$EVAL_TESTS_DIR"/eval_isolation_test.go "$EVAL_TESTS_DIR"/export_eval_test.go pkg/provider/kubernetes/gateway/
cp "$ROOT/verify/repro/c2_reachability_probe_test.go" pkg/provider/kubernetes/gateway/
cp "$ROOT/verify/repro/c2_classification_probe_test.go" pkg/middlewares/gatewayapi/misdirected/

echo "### 1. hostname classification: misdirected-request middleware vs HTTP Host matcher"
go test -tags verify -count=1 -v -run TestVerifyC2Classification ./pkg/middlewares/gatewayapi/misdirected/ 2>&1 |
	grep -E 'VERIFY-C2|^(ok|FAIL|---)' | sed -E 's/^ *c2_classification_probe_test.go:[0-9]+: //'

echo "### 2. reachability through the production router chain and a real TLS entry point"
go test -tags verify -count=1 -v -run "${RUN:-TestVerifyC2Reachability}" ./pkg/provider/kubernetes/gateway/ 2>&1 |
	grep -E 'VERIFY-C2|^(ok|FAIL|---|\s+---)' | sed -E 's/^ *c2_reachability_probe_test.go:[0-9]+: //'
