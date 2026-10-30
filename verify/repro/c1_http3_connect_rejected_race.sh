#!/usr/bin/env bash
# Run from ~/repos/traefik: bash verify/repro/c1_http3_connect_rejected_race.sh [RUNS=20]  — fetches evalon/traefik-en-f1b2f540 into a worktree, builds pkg/server tests with -race, runs TestHTTP3ConnectRejected RUNS times, counts "WARNING: DATA RACE" reports.
set -euo pipefail

RUNS="${1:-20}"
REMOTE_URL="https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests"
BRANCH="evalon/traefik-en-f1b2f540"
WT="${WT:-$HOME/wt-f1b2f540}"
OUT="${OUT:-/tmp/c1_race_logs}"

if ! git remote get-url claimrepo >/dev/null 2>&1; then
    git remote add claimrepo "$REMOTE_URL"
fi
git fetch claimrepo "$BRANCH"
if [ ! -d "$WT" ]; then
    git worktree add "$WT" "claimrepo/$BRANCH"
fi

mkdir -p "$OUT"
(cd "$WT" && go test -c -race -o "$OUT/server_race.test" ./pkg/server/)

races=0
fails=0
for i in $(seq 1 "$RUNS"); do
    log="$OUT/run_$i.log"
    (cd "$WT/pkg/server" && "$OUT/server_race.test" -test.run '^TestHTTP3ConnectRejected$' -test.count=1 >"$log" 2>&1) || fails=$((fails + 1))
    if grep -q "WARNING: DATA RACE" "$log"; then
        races=$((races + 1))
    fi
    sleep 0.3
done

echo "runs=$RUNS race_reports=$races failed_runs=$fails"
first="$(grep -l "WARNING: DATA RACE" "$OUT"/run_*.log | head -1 || true)"
if [ -n "$first" ]; then
    echo "--- first race report ($first) ---"
    grep -n -A3 "WARNING: DATA RACE\|Previous read at\|Previous write at" "$first" | head -20
    exit 1
fi
echo "no race reported in $RUNS runs"
