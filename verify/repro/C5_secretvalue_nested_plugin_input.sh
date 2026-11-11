#!/bin/sh
# Run from the repository root: sh verify/repro/C5_secretvalue_nested_plugin_input.sh  (exit 0 and "--- PASS" mean C5 reproduced).
set -eu
REMOTE=${REMOTE:-https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares}
BRANCH=evalon/traefik-se-2af317a2
HERE=$(cd "$(dirname "$0")" && pwd)
WT=${WT:-$(mktemp -d)/claim}
if [ ! -d "$WT" ]; then
  git fetch "$REMOTE" "$BRANCH"
  git worktree add --detach "$WT" FETCH_HEAD
fi
cp "$HERE"/C5/*_test.go "$WT/pkg/provider/kubernetes/crd/"
cd "$WT"
go test -v -race -count=1 -timeout=5m -run '^TestRepro_C5_' ./pkg/provider/kubernetes/crd
