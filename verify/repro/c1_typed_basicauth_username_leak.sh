#!/bin/bash
# Run: WT=<worktree of evalon/traefik-se-902c56a6> bash verify/repro/c1_typed_basicauth_username_leak.sh   (needs ~/eval_tests/tests from eval_tests.zip)
set -u
cd "${WT:?set WT to a checkout of evalon/traefik-se-902c56a6}"
cp ~/eval_tests/tests/eval_crd_behavior_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/crd/
go test -count=1 -run 'TestEval_CRD_TypedBasicAuth_SecretRetention' -v ./pkg/provider/kubernetes/crd
rm pkg/provider/kubernetes/crd/eval_crd_behavior_test.go pkg/provider/kubernetes/crd/export_eval_test.go
