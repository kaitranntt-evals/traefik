#!/bin/sh
# Run from the repository root: sh verify/repro/C2_29c47f2b_empty_secret_diagnostic.sh  (exit 0 and "--- PASS" mean C2 reproduced on evalon/traefik-se-29c47f2b).
set -eu
REMOTE=${REMOTE:-https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares}
BRANCH=evalon/traefik-se-29c47f2b
HERE=$(cd "$(dirname "$0")" && pwd)
WT=${WT:-$(mktemp -d)/claim}
if [ ! -d "$WT" ]; then
  git fetch "$REMOTE" "$BRANCH"
  git worktree add --detach "$WT" FETCH_HEAD
fi
cp "$HERE"/C2_29c47f2b/*_test.go "$WT/pkg/server/middleware/"
cd "$WT"
# The branch's own regression test for the empty Secret passes while expecting a diagnostic without the reference.
go test -v -count=1 -timeout=5m -run '^TestSecrets_RedactError$/empty_value' ./pkg/config/dynamic
go test -v -count=1 -timeout=5m -run '^TestRepro_C2_' ./pkg/server/middleware
