#!/usr/bin/env bash
# Run: bash verify/repro/c3_run.sh <worktree of evalon/traefik-ga-a6535cd8> <dir of the extracted eval_tests.zip>  (baseline comparison, then the SNICheck mutation; restores the worktree afterwards)
set -u
WT=$(cd "$1" && pwd)
EVAL=$(cd "$2" && pwd)
HERE=$(cd "$(dirname "$0")" && pwd)
PKG=pkg/provider/kubernetes/gateway
SNI=pkg/middlewares/snicheck/snicheck.go
AUTHORED='^TestListenerIsolation_HTTPSDetectMisdirectedRequests$'
cd "$WT" || exit 1

restore() { git checkout -q -- $SNI; rm -f $PKG/eval_https_test.go $PKG/eval_isolation_test.go $PKG/export_eval_test.go $PKG/c3_export_verify_test.go $PKG/c3_testlocal_status_vs_production_test.go; }
trap restore EXIT

cp "$EVAL"/tests/*.go "$HERE"/c3_export_verify_test.go "$HERE"/c3_testlocal_status_vs_production_test.go $PKG/

authored() { go test -count=1 -v -run "$AUTHORED" ./$PKG 2>&1 | grep -E '^(=== RUN|--- |PASS|FAIL|ok)'; }
probe() {
  go test -tags verify -count=1 -v -run 'TestVerifyC3' ./$PKG 2>&1 |
    grep -E 'c3_testlocal|^(---|ok|FAIL|PASS)|default TLSOptions will be applied' |
    sed -E 's/^ *c3_testlocal_status_vs_production_test.go:[0-9]+: //' | cut -c1-330
}

echo "##### branch HEAD $(git rev-parse HEAD)"
echo "##### 1. where the authored test gets its asserted status"
grep -n 'statusCode' $PKG/listener_isolation_test.go
echo "##### 2. authored test, untouched"
authored
echo "##### 3. test-local status vs production handling, untouched production code"
probe

echo "##### 4. mutation: production SNICheck never answers 421"
sed -i 's/if s.tlsOptionsName != tlsOptionsNameUsed {/if false \&\& s.tlsOptionsName != tlsOptionsNameUsed {/' $SNI
git diff -U0 -- $SNI | grep -E '^[-+][^-+]'
echo "##### 4a. authored test under the mutation"
authored
echo "##### 4b. test-local status vs production handling under the mutation"
probe
restore
echo "##### worktree after restore: $(git status --short | wc -l) modified files"
