#!/bin/sh
# Run: sh verify/repro/run.sh <worktree of the branch under test> <dir holding the extracted eval fixtures (eval_tests/tests)> [-run regexp, default ^TestVerify]
# Copies the eval fixtures and the verify probes next to the gateway provider of <worktree>, then runs the probes.
set -e
WT="$1"; FIX="$2"; RUN="${3:-^TestVerify}"
HERE="$(cd "$(dirname "$0")" && pwd)"
PKG="$WT/pkg/provider/kubernetes/gateway"
cp "$FIX"/eval_isolation_test.go "$FIX"/eval_https_test.go "$FIX"/export_eval_test.go "$PKG"/
cp "$HERE"/verify_c1c2_tls_test.go "$HERE"/verify_c3_resolvedrefs_test.go "$PKG"/
cd "$WT" && go test -tags verify -count=1 -v -run "$RUN" ./pkg/provider/kubernetes/gateway 2>&1 | grep -v -E '^(=== |    --- |--- )'
