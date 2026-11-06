#!/bin/bash
# Run: bash verify/repro/c2_mutations/run_mut.sh <worktree> <mX_<branch>.py> <label> <file-to-revert>   (patch scripts live beside this file; diffs show the exact mutations)
# usage: run_mut.sh <worktree> <patchscript.py> <label> <files...>
wt=$1; patch=$2; label=$3; shift 3
cd "$wt" || exit 1
python3 "$(dirname "$0")/$patch" || { echo "PATCH FAILED $label"; exit 1; }
git diff > /tmp/${label}.diff
echo "### $label"
go test -count=1 ./pkg/config/dynamic ./pkg/server/middleware ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx 2>&1 | grep -E "^(--- FAIL|FAIL|ok|panic|\s+--- FAIL)" 
git checkout -- "$@"
