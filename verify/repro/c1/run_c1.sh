#!/usr/bin/env bash
# Run: verify/repro/c1/run_c1.sh <claim-branch-worktree> '<go test -run regex>' [record ...]   (no records = dump the generated records and run the unmutated baseline)
# Installs verify_omit.go + a one-line hook before `return conf, statusReport, nil` in loadConfigurationFromGateways,
# builds the package test binary once, then runs it once per omitted record (VERIFY_OMIT) and prints PASS/FAIL per record.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
wt=$1; run=$2; shift 2
pkg=$wt/pkg/provider/kubernetes/gateway
cp "$here/verify_omit.go.txt" "$pkg/verify_omit.go"
grep -q 'verifyOmit(conf, statusReport)' "$pkg/kubernetes.go" || \
  sed -i 's/^\treturn conf, statusReport, nil$/\tverifyOmit(conf, statusReport)\n\treturn conf, statusReport, nil/' "$pkg/kubernetes.go"
(cd "$pkg" && go test -c -o /tmp/c1-$(basename "$wt").test . )
bin=/tmp/c1-$(basename "$wt").test
cd "$pkg"
if [ $# -eq 0 ]; then
  rm -f /tmp/c1-dump.txt
  if VERIFY_DUMP=/tmp/c1-dump.txt "$bin" -test.run "$run" -test.count=1 >/tmp/c1-base.log 2>&1; then echo "BASELINE PASS"; else echo "BASELINE FAIL"; tail -30 /tmp/c1-base.log; fi
  sort -u /tmp/c1-dump.txt
  exit 0
fi
for rec in "$@"; do
  if VERIFY_OMIT="$rec" "$bin" -test.run "$run" -test.count=1 >/tmp/c1-run.log 2>&1; then
    echo "PASS  omit=$rec"
  else
    echo "FAIL  omit=$rec  ($(grep -c -- '--- FAIL' /tmp/c1-run.log) failing (sub)tests; $(grep -m1 -E 'panic:|Error:' /tmp/c1-run.log | tr -s ' \t' ' '))"
  fi
done
