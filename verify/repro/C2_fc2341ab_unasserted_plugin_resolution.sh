#!/bin/sh
# Run from the repository root: sh verify/repro/C2_fc2341ab_unasserted_plugin_resolution.sh  (exit 0 and two "SURVIVED" lines mean C2 reproduced on evalon/traefik-se-fc2341ab).
set -eu
REMOTE=${REMOTE:-https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares}
BRANCH=evalon/traefik-se-fc2341ab
HERE=$(cd "$(dirname "$0")" && pwd)
WT=${WT:-$(mktemp -d)/claim}
if [ ! -d "$WT" ]; then
  git fetch "$REMOTE" "$BRANCH"
  git worktree add --detach "$WT" FETCH_HEAD
fi
cd "$WT"

# The branch's own tests, on the packages that hold its Secret-related tests.
run_tests() {
  go test -count=1 -timeout=10m ./pkg/config/dynamic/... ./pkg/provider/kubernetes/crd ./pkg/server/middleware ./pkg/plugins
}

echo "== the values the provider tests asserted before the change, in the branch's test files:"
grep -rn 'this_is_the_very_deep_secret\|secret_data1\|admin_password' pkg --include='*_test.go' || echo "(none)"

echo "== unmutated"
run_tests

status=0
# M4 and M5 break only the build-time resolution of one plugin shape; M6 is the control (a shape that is asserted).
for mutation in M4-list-of-map-not-resolved-at-build M5-deep-nested-not-resolved-at-build M6-array-not-resolved-at-build; do
  echo "== $mutation"
  git apply "$HERE/C2_fc2341ab/$mutation.patch"
  if run_tests; then result=SURVIVED; else result=KILLED; fi
  git checkout -- pkg/config/dynamic/secrets.go
  echo "$mutation: $result"
  case "$mutation" in
    M6-*) [ "$result" = KILLED ] || status=1 ;;
    *) [ "$result" = SURVIVED ] || status=1 ;;
  esac
done
exit $status
