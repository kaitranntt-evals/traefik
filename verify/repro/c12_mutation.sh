#!/bin/bash
# Run: bash verify/repro/c12_mutation.sh <clean worktree of evalon/traefik-se-6226f21f>  -- mutates auth-map key iteration to random map order and reruns the committed tests.
set -u
W=$1; R=$(cd "$(dirname "$0")" && pwd); P=pkg/provider/kubernetes/ingress-nginx
cd "$W" || exit 1
run(){ go test -count=20 "$@" ./$P 2>&1 | grep -E '^(--- FAIL|FAIL|ok)' | sort | uniq -c; }
echo "== committed tests that mention auth-map"; grep -n 'auth-map' $P/*_test.go
echo "== 1. committed tests, sorted iteration (as committed)"; run
sed -i -E 's/for _, (key|user) := range slices.Sorted\(maps.Keys\(secret.Data\)\) \{/for \1 := range maps.Keys(secret.Data) {/' $P/build.go
echo "== mutation"; git diff --stat -- $P/build.go; git diff -U0 -- $P/build.go | grep '^[-+][^-+]'
echo "== 2. committed tests, auth-map keys iterated in random map order"; run
grep -q "func (p \*Provider) resolveBasicAuth" $P/*.go || { git checkout -- $P/build.go; echo "== control probe skipped: it targets the 6226f21f helper API"; git status --short; exit 0; }
cp "$R/c12_order_probe_test.go" $P/
echo "== 3. control: order-sensitive multi-key probe under the same mutation"; run -run '^TestVerifyC12_OrderProbe$' | sed 's/^ *[0-9]* --- FAIL/     N --- FAIL/'
git checkout -- $P/build.go
echo "== 4. control: same probe on the committed code"; run -run '^TestVerifyC12_OrderProbe$' | sed 's/^ *[0-9]* --- FAIL/     N --- FAIL/'
rm $P/c12_order_probe_test.go; git status --short
