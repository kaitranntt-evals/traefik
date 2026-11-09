#!/usr/bin/env bash
# Run from the repo root: `bash verify/repro/c1_repro.sh` (adds a detached worktree of evalon/traefik-ga-730e07a9, applies the env-gated mutation patch, runs the four-listener subtests with and without the `other` listener status record).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
REPO_URL="${CLAIM_REPO_URL:-$(git -C "$ROOT" remote get-url origin | sed 's#/traefik\(\.git\)\?$#/traefik-gateway-listener-isolation#')}"
WT="${WT:-$ROOT/../verify-c1-wt}"
PKG=./pkg/provider/kubernetes/gateway
RUN='^TestListenerIsolation$/^(HTTPRoute|GRPCRoute)$/^(HTTP|HTTPS)$/^fallback_excludes_all_more_specific_listeners$'
FILTER='^(--- |\s+--- (FAIL|PASS): [^/]*/[^/]*/[^/]*/[^/]*$|ok|FAIL|PASS|VERIFY-C1|panic:)'

git -C "$ROOT" fetch "$REPO_URL" evalon/traefik-ga-730e07a9
[ -d "$WT" ] || git -C "$ROOT" worktree add --detach "$WT" FETCH_HEAD
cd "$WT"
git checkout -- pkg/provider/kubernetes/gateway/kubernetes.go
git apply "$ROOT/verify/repro/c1_drop_other_listener.patch"

echo "### 1. original output (four-listener subtests, test file untouched)"
VERIFY_C1_TRACE=1 go test -count=1 -v -run "$RUN" $PKG 2>&1 | grep -E "$FILTER" | sort | uniq -c

echo "### 2. \`other\` listener record removed from the actual Gateway status (expectations untouched)"
VERIFY_C1_DROP_OTHER=all VERIFY_C1_TRACE=1 go test -count=1 -v -run "$RUN" $PKG 2>&1 | grep -E "$FILTER" | sort | uniq -c

echo "### 3. same removal limited to four-listener Gateways, whole package"
VERIFY_C1_DROP_OTHER=four go test -count=1 $PKG/... 2>&1 | tail -3

echo "### 4. control: removal for every Gateway, whole TestListenerIsolation (two-listener cases index Listeners[1])"
VERIFY_C1_DROP_OTHER=all go test -count=1 -run '^TestListenerIsolation$' $PKG 2>&1 | grep -E '^(panic:|--- FAIL|ok|FAIL)' || true
