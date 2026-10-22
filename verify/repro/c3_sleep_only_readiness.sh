#!/bin/sh
# Run from a checkout of evalon/traefik-en-c340e9a5: sh verify/repro/c3_sleep_only_readiness.sh
# Prints the startup-to-request sequence of TestHTTP3DenyConnect (no readiness signal/probe, only time.Sleep(time.Second)),
# then runs the test as-is and with the sleep deleted to show elapsed time is the only thing gating the request.
set -eu
F=pkg/server/server_entrypoint_tcp_http3_test.go
START=$(grep -n '^func TestHTTP3DenyConnect' "$F" | cut -d: -f1)
awk -v s="$START" 'NR>=s+35 && NR<=s+65' "$F"
echo "--- readiness primitives between Start and RoundTrip:"; awk -v s="$START" 'NR>=s && NR<=s+65' "$F" | grep -n 'Sleep\|<-\|Eventually\|ready\|Dial' || true
go test -count=1 -timeout=60s ./pkg/server -run '^TestHTTP3DenyConnect$' -v 2>&1 | grep -a '^--- \|^ok\|^FAIL'
trap 'git checkout "$F"' EXIT
sed -i '/^func TestHTTP3DenyConnect/,/^}/{s/^\ttime\.Sleep(time\.Second)$/\t\/\/ sleep removed by audit/}' "$F"
grep -n 'sleep removed by audit' "$F"
go test -count=1 -timeout=60s ./pkg/server -run '^TestHTTP3DenyConnect$' -v 2>&1 | grep -a '^--- \|^ok\|^FAIL'
