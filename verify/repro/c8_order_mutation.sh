#!/usr/bin/env bash
# Run: verify/repro/c8_order_mutation.sh <worktree of evalon/traefik-se-01f93250> — applies two order-only mutations to the auth-map user generation and runs the committed tests against each.
set -u
WT=${1:?worktree}
HERE=$(cd "$(dirname "$0")" && pwd)
PKG=pkg/provider/kubernetes/ingress-nginx
cd "$WT" || exit 1
cp "$HERE/c8_probe_test.go" $PKG/zz_verify_c8_test.go
trap 'git checkout -- $PKG/build.go; rm -f $PKG/zz_verify_c8_test.go' EXIT

run() {
  echo "##### $1"
  git --no-pager diff --stat -- $PKG/build.go
  go test -tags verify -count=1 -run 'TestVerifyC8' -v ./$PKG/ 2>&1 | grep -E '^(C8|ok|FAIL|---)'
  echo "--- committed auth tests: go test -race -count=1 -run 'TestAuthSecretReferences|TestLoadIngresses|Test_basicAuthUsers' -v ./$PKG/"
  go test -race -count=1 -run 'TestAuthSecretReferences|TestLoadIngresses|Test_basicAuthUsers' -v ./$PKG/ 2>&1 | grep -E '^(--- |    --- (FAIL|PASS): TestAuthSecretReferences|ok|FAIL|PASS)'
  echo "--- whole package: go test -count=1 ./$PKG/..."
  go test -count=1 ./$PKG/... 2>&1 | tail -3
}

run "UNMODIFIED"

# Mutation 1: same users, reversed sequence.
sed -i 's|^\t\tslices.Sort(keys)$|\t\tslices.Sort(keys)\n\t\tslices.Reverse(keys)|' $PKG/build.go
git --no-pager diff -- $PKG/build.go
run "MUTATION 1: reversed order"
git checkout -- $PKG/build.go

# Mutation 2: same users, sequence left to the map iteration, i.e. changing between loads.
sed -i 's|^\t\tslices.Sort(keys)$|\t\t_ = slices.Sort[[]string]|' $PKG/build.go
git --no-pager diff -- $PKG/build.go
run "MUTATION 2: unsorted (map iteration order, changes between loads)"
