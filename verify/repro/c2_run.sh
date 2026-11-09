#!/usr/bin/env bash
# Run: bash verify/repro/c2_run.sh <worktree of evalon/traefik-ga-fd783057> <dir of the extracted eval_tests.zip> [count]  (prints the sibling fallbacks and both crossing directions through production HTTPS handling; restores the worktree afterwards)
set -u
WT=$(cd "$1" && pwd)
EVAL=$(cd "$2" && pwd)
COUNT=${3:-10}
HERE=$(cd "$(dirname "$0")" && pwd)
PKG=pkg/provider/kubernetes/gateway
cd "$WT" || exit 1

restore() { rm -f $PKG/eval_https_test.go $PKG/eval_isolation_test.go $PKG/export_eval_test.go $PKG/c2_sibling_gateway_fallback_test.go; }
trap restore EXIT

cp "$EVAL"/tests/*.go "$HERE"/c2_sibling_gateway_fallback_test.go $PKG/

run() { go test -tags verify -count="$1" -v -run 'TestVerifyC2' ./$PKG 2>&1 | grep -E 'FALLBACK|SINGLE|SIBLING|RESULT|TLS options|^(---|ok|FAIL|PASS)' | sed -E 's/^ *c2_sibling_gateway_fallback_test.go:[0-9]+: //' | cut -c1-400; }

echo "##### branch HEAD $(git rev-parse HEAD)"
echo "##### 1. one run (both strict-SNI modes of the harness)"
run 1
echo "##### 2. tally of the two crossing directions over $COUNT runs"
run "$COUNT" | grep RESULT | sort | uniq -c
restore
echo "##### worktree after restore: $(git status --short | wc -l) modified files"
