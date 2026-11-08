#!/bin/sh
# Run from a checkout of branch evalon/traefik-ga-91454014 (kaitranntt-evals/traefik-gateway-listener-isolation): sh verify/repro/c5_crossed_listener_test_bypasses_snicheck.sh
# Shows (1) the authored HTTPS subtests expecting 200 for SNI bar.example.com + Host foo.example.com + /exact/Call pass, and
# (2) the test handler is a bare httpmuxer (no snicheck middleware, no production router manager).
set -eu
T=pkg/provider/kubernetes/gateway/listener_test.go
echo "== handler construction and crossed-name request in $T"
grep -n 'httpmuxer.NewMuxer\|requestdecorator.New(nil).ServeHTTP(recorder, req, muxer.ServeHTTP)\|ServerName: "bar.example.com"\|{host: "foo.example.com", service: "exact", listener: "exact"}\|assert.Equal(t, http.StatusOK, recorder.Code)' "$T"
echo "== snicheck / server router imports in the test (expect none)"
grep -c 'snicheck\|pkg/server/router' "$T" || true
echo "== run the authored test"
go test -count=1 -race -v -timeout=5m -run '^TestListenerIsolation$' ./pkg/provider/kubernetes/gateway 2>&1 | grep -E -- '--- (PASS|FAIL): TestListenerIsolation/.*HTTPS.*/foo.example.com/exact|^(ok|FAIL)'
