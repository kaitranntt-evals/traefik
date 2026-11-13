#!/bin/sh
# Run: verify/repro/run.sh '<-run regex>' [repo root, default .]  (needs the eval fixtures from eval_tests.zip in $FIXTURES, default ~/fx/tests)
set -e
ROOT=${2:-.}; G=$ROOT/pkg/provider/kubernetes/gateway
cp "${FIXTURES:-$HOME/fx/tests}"/*.go "$G"/
cp "$(dirname "$0")"/verify_*_test.go "$G"/
cd "$ROOT" && go test -tags verify -count=1 -v -run "$1" ./pkg/provider/kubernetes/gateway 2>&1 | grep -v '^{"level"\|^[0-9]\{4\}-[0-9TZ:+-]* [A-Z]\{3\} ' 
