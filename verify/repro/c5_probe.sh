#!/bin/bash
# Run: verify/repro/c5_probe.sh <fa481ef5-worktree> — (after install.sh) runs the provided plugin-execution fixtures unmodified, then under production mutations (reverted afterwards) to show which of them observe production integer decoding and post-callback configuration ownership.
W=$1; cd "$W" || exit 1
P=./pkg/provider/kubernetes/crd
FX='^TestEval_ProviderToBuilder_Plugin(TypedSecretField|SuccessExecution|NestedExecution|EscapedKeyExecution|InputMutationIsolation)$'
trap 'git checkout -q -- pkg/plugins/middlewareyaegi.go pkg/server/middleware/plugins.go' EXIT
filter() { grep -E '^\s*(--- (PASS|FAIL)|ok|FAIL|PASS)|Error Trace|RESULT|published AFTER|TYPED' | sed -E 's/\([0-9.]+s\)//' | sort | uniq -c | sort -k2; }
run() {
  echo "\$ go test -count=1 -v -run '$FX' $P   # provided fixtures"
  go test -count=1 -v -run "$FX" $P 2>&1 | filter
  echo "\$ go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' $P   # audit probes"
  go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' $P 2>&1 | filter
}
echo "### branch HEAD: $(git rev-parse HEAD)"
echo "### provided typed-Secret probe (eval_local_plugin.go) and every eval_reject assignment in the provided tests"
grep -n 'DecodeReject\|strconv.Atoi\|SecretIsInt: ' $P/plugins-local/src/evalplugin/evalplugin.go
grep -n '"eval_reject"' $P/eval_provider_to_builder_runtime_test.go
echo "### where the provided plugin writes to its configuration, and what it keeps (eval_local_plugin.go)"
grep -n 'evalMutateNested(config.Remain)\|^func CreateConfig\|return &Config{}\|^func New\|rw.Write(body)' $P/plugins-local/src/evalplugin/evalplugin.go
echo "### ownership assertions of the provided isolation test"
grep -n 'assert.JSONEq(t, string(expected)' $P/eval_provider_to_builder_runtime_test.go
echo; echo "### BASELINE (unmodified production)"; run
all() { go test -count=1 -v -run '^TestEval_' $P 2>&1 | grep -E '^\s*--- (PASS|FAIL)' | sed -E 's/ \([0-9.]+s\)//' | sort; }
all > /tmp/c5-all-base.$$
echo; echo "### MUTATION M1 — production: pkg/plugins/middlewareyaegi.go 'WeaklyTypedInput: true' -> false (a resolved Secret string can no longer be decoded into an int field)"
sed -i 's/WeaklyTypedInput: true,/WeaklyTypedInput: false,/' pkg/plugins/middlewareyaegi.go; git diff -U0 -- pkg/plugins/middlewareyaegi.go | grep '^[+-][^+-]'; run
all > /tmp/c5-all-m1.$$
echo; echo "### every provided TestEval_ test of $P: result lines at BASELINE vs under M1"
echo "\$ go test -count=1 -v -run '^TestEval_' $P   # run at baseline and under M1, '--- PASS/FAIL' lines sorted and diffed"
echo "baseline: $(grep -c PASS /tmp/c5-all-base.$$) PASS, $(grep -c FAIL /tmp/c5-all-base.$$) FAIL; under M1: $(grep -c PASS /tmp/c5-all-m1.$$) PASS, $(grep -c FAIL /tmp/c5-all-m1.$$) FAIL"
diff /tmp/c5-all-base.$$ /tmp/c5-all-m1.$$ && echo "diff: IDENTICAL (no provided test changes result when production can no longer decode a string into an int field)"
rm -f /tmp/c5-all-base.$$ /tmp/c5-all-m1.$$
git checkout -q -- pkg/plugins/middlewareyaegi.go
