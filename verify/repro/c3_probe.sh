#!/usr/bin/env bash
# Run: verify/repro/c3_probe.sh <claim-branch-worktree> — runs the branch's own modified TestLoadIngressRoutes plugin table cases, then re-runs them under fixture/test/production mutations (each reverted afterwards) to show which assertions are live.
set -u
WT=$(cd "$1" && pwd)
cd "$WT"
PKG=./pkg/provider/kubernetes/crd
T=pkg/provider/kubernetes/crd/kubernetes_test.go
FX=pkg/provider/kubernetes/crd/fixtures/with_plugin_read_array_of_map_contain_secret.yml
S=pkg/config/dynamic/secrets.go
RUN='^TestLoadIngressRoutes$/^Simple_Ingress_Route,_with_test_middleware_read_config_from_(secret|deep_secret|an_array_of_secret)(#[0-9]+)?$'
echo "### branch HEAD: $(git rev-parse HEAD)"
echo "### git status (must be clean before): $(git status --short | wc -l) changed files"
run() { # label
  echo; echo "### $1"
  echo "\$ go test -count=1 -v -run '$RUN' $PKG"
  go test -count=1 -v -run "$RUN" $PKG 2>&1 | grep -vE '^\s*$' | grep -E '^(=== RUN|\s*--- |PASS|FAIL|ok|---|\s+(Error|Diff|[-+] )|\s+[-+]\s|.*(admin|user_password|urn:k8s|test.secret))' | head -${2:-60}
}
restore() { git checkout -- "$T" "$FX" "$S"; }
trap restore EXIT

run "BASELINE (unmodified branch)"

echo; echo "### modified test hunk under audit:"; git diff af6d927c HEAD -- "$T" | sed -n '1,16p'

sed -i 's/name: admin$/name: admin-MUTATED-SIBLING/' "$FX"
run "MUTATION F1 — fixture: literal sibling 'name: admin' -> 'name: admin-MUTATED-SIBLING' (does an assertion check the preserved content-bearing sibling of an indexed entry?)" 40
restore

sed -i 's/^  key1: .*/  key1: TVVUQVRFRC1WQUxVRQ==/' "$FX"   # base64("MUTATED-VALUE")
run "MUTATION F2 — fixture: Secret key1 value -> 'MUTATED-VALUE' (does an assertion check the resolved value of indexed entry [0]?)" 40
restore

python3 - "$FX" <<'PY'
import sys,re
p=sys.argv[1]; s=open(p).read()
s=s.replace("  plugin:\n    test-secret:","  plugin:\n    test_secret:")
open(p,"w").write(s)
PY
run "MUTATION F3 — fixture: dashed plugin configuration-map key 'test-secret' -> 'test_secret' (does an assertion check the preserved special-character key above the resolved value?)" 40
restore

python3 - "$T" <<'PY'
import sys,re
p=sys.argv[1]; s=open(p).read()
n=0
for old,new in [("conf.HTTP.Middlewares[name] = middleware.Resolve()","conf.HTTP.Middlewares[name] = middleware"),
                ("conf.HTTP.Middlewares[name] = middleware.ResolveSecrets()","conf.HTTP.Middlewares[name] = middleware"),
                ("resolved := middleware.ResolveSecrets()","resolved := middleware.DeepCopy()")]:
    if old in s: s=s.replace(old,new); n+=1
assert n==1, n
open(p,"w").write(s)
PY
run "MUTATION T1 — test: drop the added Resolve call before assert.Equal (is resolution on the asserted path?)" 40
restore

python3 - "$S" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
muts=[("\tcase reflect.Slice:\n\t\tfor i := 0; i < v.Len(); i++ {","\tcase reflect.Slice:\n\t\tfor i := 0; i < 0; i++ {"),
      ("\tcase reflect.Slice:\n\t\tfor i := range value.Len() {","\tcase reflect.Slice:\n\t\tfor i := range 0 {\n\t\t\t_ = value.Len()"),
      ("\tcase []any:\n\t\tfor i, value := range v {","\tcase []any:\n\t\tfor i, value := range v[:0] {")]
n=0
for old,new in muts:
    if old in s: s=s.replace(old,new,1); n+=1
assert n==1, n
open(p,"w").write(s)
PY
echo; echo "### production mutation P1 diff:"; git diff -- "$S" | grep -E '^[-+][^-+]'
run "MUTATION P1 — production: resolver skips list (indexed) entries" 40
restore
trap - EXIT
echo; echo "### git status (must be clean after): $(git status --short | wc -l) changed files"
