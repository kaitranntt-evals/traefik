#!/bin/sh
# Run from a checkout of evalon/traefik-en-a6408acd: sh verify/repro/c2_withhold_response.sh
# Applies a one-line mutation making denyConnect withhold its response, then shows the new h2c and HTTP/3 CONNECT tests
# hang until the suite -timeout instead of failing on a test-local bound.
set -eu
git apply "$(dirname "$0")/c2_withhold_response.patch"
trap 'git checkout pkg/server/server_entrypoint_tcp.go' EXIT
go test -count=1 -timeout=30s ./pkg/server -run '^TestConnectRejected$/^HTTP/2_CONNECT' -v 2>&1 | grep -a 'RUN\|PASS\|FAIL\|timed out\|server_entrypoint_tcp_test.go' || true
go test -count=1 -timeout=30s ./pkg/server -run '^TestHTTP3ConnectRejected$' -v 2>&1 | grep -a 'RUN\|PASS\|FAIL\|timed out\|server_entrypoint_tcp_http3_test.go' || true
