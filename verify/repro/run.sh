#!/usr/bin/env bash
# Run from the root of the checkout under test: EVAL_TESTS=<dir with the extracted eval_tests.zip tests/*.go> <path to>/verify/repro/run.sh c1c2|c3
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
pkg=pkg/provider/kubernetes/gateway

case "${1:-}" in
  c1c2) pattern='^TestVerifyC1C2_' ;;
  c3) pattern='^TestVerifyC3_' ;;
  *) echo "usage: EVAL_TESTS=<dir> $0 c1c2|c3" >&2; exit 2 ;;
esac

# The probes reuse the helpers of the archived eval fixtures, byte-exact, at the path assessment provisions them.
cp "${EVAL_TESTS:?set EVAL_TESTS to the directory holding eval_https_test.go, eval_isolation_test.go, export_eval_test.go}"/{eval_https_test.go,eval_isolation_test.go,export_eval_test.go} "$pkg/"
cp "$here"/verify_*_test.go "$pkg/"

go test -tags verify -count=1 -v -run "$pattern" "./$pkg"
