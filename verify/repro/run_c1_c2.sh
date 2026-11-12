#!/bin/bash
# Run: verify/repro/run_c1_c2.sh <worktree> <logfile> — installs fixtures + probes, runs the C1/C2 fixtures and the verify probes on one claim branch.
W=$1; LOG=$2; R=$(cd "$(dirname "$0")" && pwd)
"$R/install.sh" "$W"
cd "$W" || exit 1
{
  echo "### branch HEAD: $(git rev-parse HEAD)"
  echo "### fixtures (byte-exact): go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(SuccessExecution|TypedSecretField|NestedExecution|EscapedKeyExecution|InputMutationIsolation|BuildErrorRedaction|CachedErrorRedaction)\$' ./pkg/provider/kubernetes/crd"
  go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(SuccessExecution|TypedSecretField|NestedExecution|EscapedKeyExecution|InputMutationIsolation|BuildErrorRedaction|CachedErrorRedaction)$' ./pkg/provider/kubernetes/crd 2>&1 | grep -vE '^=== (RUN|PAUSE|CONT|NAME)'
  echo "### probes: go test -tags verify -count=1 -v -run '^TestVerify_C[12]_' ./pkg/provider/kubernetes/crd"
  go test -tags verify -count=1 -v -run '^TestVerify_C[12]_' ./pkg/provider/kubernetes/crd 2>&1 | grep -vE '^=== (RUN|PAUSE|CONT|NAME)'
} > "$LOG" 2>&1
