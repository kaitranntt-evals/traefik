#!/bin/bash
# Run: verify/repro/run_retain.sh <worktree> <logfile> — installs fixtures + probes, runs the retained-configuration (post-callback ownership) probe on one branch.
W=$1; LOG=$2; R=$(cd "$(dirname "$0")" && pwd)
"$R/install.sh" "$W"
cd "$W" || exit 1
{
  echo "### branch HEAD: $(git rev-parse HEAD)"
  echo "\$ go test -tags verify -count=1 -v -run '^TestVerify_Retain' ./pkg/provider/kubernetes/crd"
  go test -tags verify -count=1 -v -run '^TestVerify_Retain' ./pkg/provider/kubernetes/crd 2>&1 | grep -vE '^=== (RUN|PAUSE|CONT|NAME)'
} > "$LOG" 2>&1
