#!/bin/bash
# Run: verify/repro/c7_probe.sh <3efc4f96-worktree> — runs the branch's own added/modified ingress-nginx tests unmodified, then with basicAuthUsers forced to return its auth-map entries in reverse-sorted order (reverted afterwards), plus the unsorted-order audit probe.
W=$1; R=$(cd "$(dirname "$0")" && pwd); cd "$W" || exit 1
P=./pkg/provider/kubernetes/ingress-nginx; F=pkg/provider/kubernetes/ingress-nginx/build.go
cp "$R/c7_order_probe_test.go" "$W/pkg/provider/kubernetes/ingress-nginx/verify_c7_order_probe_test.go"
trap 'git checkout -q -- $F' EXIT
own() {
  echo "\$ go test -count=5 -run '^(TestBasicAuthUsers|TestLoadIngresses)\$' -v $P   # branch's own added/modified tests, eval fixtures excluded"
  go test -count=5 -run '^(TestBasicAuthUsers|TestLoadIngresses)$' -v $P 2>&1 | grep -E '^\s*--- (PASS|FAIL)|^(ok|FAIL|PASS)' | grep -E 'TestBasicAuthUsers|^--- |^(ok|FAIL|PASS)' | sed -E 's/ \([0-9.]+s\)//' | sort | uniq -c
  echo "\$ go test -count=1 $P   # whole package, branch's own tests + archived eval fixtures"
  go test -count=1 $P 2>&1 | grep -E '^(--- FAIL|ok|FAIL|PASS)' | sort | uniq -c
}
probe() {
  echo "\$ go test -tags verify -count=1 -v -run '^TestVerify_C7' $P   # audit probe: order compared without sorting"
  go test -tags verify -count=1 -v -run '^TestVerify_C7' $P 2>&1 | grep -E 'RESULT|^\s*--- |^(ok|FAIL|PASS)|Error:|Messages:'
}
echo "### branch HEAD: $(git rev-parse HEAD)"
echo "### ordering-related lines of the added auth-map test (pkg/provider/kubernetes/ingress-nginx/build_test.go)"
grep -n 'basicAuthUsers(secret\|slices.Sort\|assert.Equal\|ElementsMatch' pkg/provider/kubernetes/ingress-nginx/build_test.go
echo "### every auth-map mention in the package's test files"
grep -n 'auth-map' pkg/provider/kubernetes/ingress-nginx/*_test.go | grep -v '/eval_\|verify_'
echo; echo "### BASELINE (unmodified production)"; own; probe
echo; echo "### MUTATION O1 — production: basicAuthUsers returns the auth-map entries in REVERSE-sorted order (same contents)"
python3 - <<'PY'
p='pkg/provider/kubernetes/ingress-nginx/build.go'; s=open(p).read()
old='''			users = append(users, user+":"+urn)
		}

		return users, secrets, nil'''
new='''			users = append(users, user+":"+urn)
		}
		slices.Sort(users)
		slices.Reverse(users)

		return users, secrets, nil'''
assert s.count(old)==1; open(p,'w').write(s.replace(old,new))
PY
git diff -U0 -- $F | grep '^[+-][^+-]'; own; probe
