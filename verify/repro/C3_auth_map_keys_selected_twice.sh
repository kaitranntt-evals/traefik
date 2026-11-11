#!/bin/sh
# Run from the repository root: sh verify/repro/C3_auth_map_keys_selected_twice.sh  (exit 0 and "--- PASS" mean C3 reproduced).
set -eu
REMOTE=${REMOTE:-https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares}
BRANCH=evalon/traefik-se-fc2341ab
HERE=$(cd "$(dirname "$0")" && pwd)
WT=${WT:-$(mktemp -d)/claim}
if [ ! -d "$WT" ]; then
  git fetch "$REMOTE" "$BRANCH"
  git worktree add --detach "$WT" FETCH_HEAD
fi
cp "$HERE"/C3/*_test.go "$WT/pkg/provider/kubernetes/ingress-nginx/"
cd "$WT"
go test -v -count=1 -timeout=5m -run '^TestRepro_C3_' ./pkg/provider/kubernetes/ingress-nginx
