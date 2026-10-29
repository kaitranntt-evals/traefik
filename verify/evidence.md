## C1

Target branch: `evalon/traefik-en-d6261075` (repo `kaitranntt-evals/traefik-entrypoint-reject-connect-requests`, HEAD `45a82d7d5`), checked out in a worktree at `~/repos/wt-c1`.

### Newly added tests and their transports

```sh
cd ~/repos/wt-c1 && git diff 246d9bfe7 HEAD --name-only -- '*_test.go'
```

```console
pkg/server/server_entrypoint_connect_test.go   # new file: TestDenyConnect, TestHTTPConnect
pkg/server/server_entrypoint_tcp_test.go       # modified expectations only (TestRequestTargetForms, TestHTTP2RequestTLS)
```

```sh
grep -n "CloseIdleConnections\|DisableKeepAlives\|NewRoundTripperManager\|transports.Update" pkg/server/server_entrypoint_connect_test.go
```

```console
156:	t.Cleanup(http1Transport.CloseIdleConnections)
158:	t.Cleanup(https1Transport.CloseIdleConnections)
165:	t.Cleanup(h2cTransport.CloseIdleConnections)
167:	t.Cleanup(h2Transport.CloseIdleConnections)
219:			transports := service.NewRoundTripperManager()
220:			transports.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})
```

Part A — the five client-side transports the test uses to reach the entrypoint (`http1Transport`, `https1Transport`, `h2cTransport`, `h2Transport`, `h3Transport`) all register `CloseIdleConnections` (or `Close` for HTTP/3) via `t.Cleanup` (lines 156–169). This part of the claim does not hold.

Part B — the backend client transport is constructed by the test itself, once per `routing` subtest, through `service.NewRoundTripperManager()` + `Update({"default@internal": {}})` (lines 219–220). `createRoundTripper` builds an `http.Transport` with keep-alives enabled (`DisableKeepAlives` unset, `IdleConnTimeout: 90s`) wrapped in `smartRoundTripper`/`KerberosRoundTripper`. Nothing closes it: `RoundTripperManager` has only `Update` and `Get`:

```sh
grep -n "^func (r \*RoundTripperManager)" pkg/server/service/roundtripper.go
```

```console
48:func (r *RoundTripperManager) Update(newConfigs map[string]*dynamic.ServersTransport) {
89:func (r *RoundTripperManager) Get(name string) (http.RoundTripper, error) {
```

The ForwardAuth middleware client (`pkg/middlewares/auth/forward.go:76-91`) uses `http.DefaultTransport` unless `config.TLS` is set (the test sets none), so it is not test-owned and is out of scope.

### Instrumented run of the solution's own TestHTTPConnect

Patch: `verify/repro/c1_instrument_TestHTTPConnect.patch` (adds a `ConnState` hook on the `backend` httptest server and logs the number of idle keep-alive backend connections at the end of each routing subtest and at cleanup; production code untouched).

```sh
cd ~/repos/wt-c1 && git apply ~/repos/traefik/verify/repro/c1_instrument_TestHTTPConnect.patch
go test -count=1 -v -run '^TestHTTPConnect$' ./pkg/server 2>&1 | grep -E "VERIFY C1|^(--- |PASS|FAIL|ok)"
git checkout -- pkg/server/server_entrypoint_connect_test.go
```

```console
    server_entrypoint_connect_test.go:254: VERIFY C1: end of routing subtest "matching router": idle keep-alive backend connections still open=1
    server_entrypoint_connect_test.go:254: VERIFY C1: end of routing subtest "unmatched router": idle keep-alive backend connections still open=1
    server_entrypoint_connect_test.go:254: VERIFY C1: end of routing subtest "no routers": idle keep-alive backend connections still open=1
    server_entrypoint_connect_test.go:138: VERIFY C1: at test cleanup (before backend.Close): RoundTripperManagers created=3, CloseIdleConnections calls on them=0 (no such method), idle keep-alive backend connections still open=1
--- PASS: TestHTTPConnect (0.87s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	0.907s
```

Only the "matching router" subtest forwards to the backend; the idle connection it pooled stays open through the two following subtests (whose managers never touch the backend) and is still open when the top-level cleanup runs. It is only torn down server-side by `backend.Close()`.

### Standalone repro

`verify/repro/c1_backend_transport_never_closed_test.go` mirrors the test's backend wiring, proxies one GET, then checks the backend's connection table and whether any object the test holds exposes `CloseIdleConnections`/`Close`.

```sh
cd ~/repos/wt-c1 && cp ~/repos/traefik/verify/repro/c1_backend_transport_never_closed_test.go pkg/server/
go test -count=1 -v -run '^TestVerifyC1_BackendTransportNeverClosed$' ./pkg/server
rm pkg/server/c1_backend_transport_never_closed_test.go
```

```console
=== RUN   TestVerifyC1_BackendTransportNeverClosed
    c1_backend_transport_never_closed_test.go:89: idle keep-alive connections held open on the backend after the proxied request: 1
    c1_backend_transport_never_closed_test.go:99: RoundTripperManager has CloseIdleConnections=false Close=false; round tripper *service.KerberosRoundTripper has CloseIdleConnections=false; inner *service.smartRoundTripper has CloseIdleConnections=false
--- PASS: TestVerifyC1_BackendTransportNeverClosed (0.50s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	0.535s
```

### Baseline (the new tests themselves pass under -race)

```sh
cd ~/repos/wt-c1 && go test -race -count=1 -timeout=60s -run '^(TestDenyConnect|TestHTTPConnect)$' ./pkg/server
```

```console
ok  	github.com/traefik/traefik/v2/pkg/server	1.351s
```

### Impact reasoning

Three `RoundTripperManager`-owned backend transports (keep-alives on, 90 s idle timeout) are created and abandoned by `TestHTTPConnect` with no teardown path at all — the type offers no `CloseIdleConnections`/`Close`, so the test could not register one without reaching into unexported fields. The pooled HTTP/1.1 connection and its `persistConn` read/write goroutines outlive the subtest that created them and are only reclaimed when `backend.Close()` runs at the end of the test. The test does not hang or fail (`httptest.Server.Close` closes idle connections server-side), so this is a test-hygiene leak, and the same `NewRoundTripperManager()`-without-teardown pattern already exists in the pre-existing `pkg/server/routerfactory_test.go` and `server_entrypoint_tcp_http3_test.go`. Part A of the claim (the test's own `http.Transport`/`http2.Transport`/`http3.Transport` clients) does not hold: every one of them is closed through `t.Cleanup`.

## C2

Target branch: `evalon/traefik-en-2d7182b0` (repo `kaitranntt-evals/traefik-entrypoint-reject-connect-requests`, HEAD `0f1c31d39`), checked out in a worktree at `~/repos/wt-c2`.

### The comment

```sh
cd ~/repos/wt-c2 && sed -n 774,780p pkg/server/server_entrypoint_tcp.go
```

```console
// denyConnect rejects the CONNECT requests, whatever the HTTP version of the client.
// Traefik does not support the tunneling a CONNECT request asks for: the backend is reached over HTTP/1.1,
// and a tunnel accepted by the backend would turn the client stream into a raw byte stream
// going unnoticed by the routing and the middlewares.
// As the rejection happens before the routing, it does not depend on a router matching the request,
// and the rejected requests are not reported in the access logs.
func denyConnect(h http.Handler) http.Handler {
```

### Backend transport configuration on the same branch

```sh
grep -n "newSmartRoundTripper\|DisableHTTP2\|ConfigureTransports\|AllowHTTP\|RegisterProtocol" pkg/server/service/roundtripper.go pkg/server/service/smart_roundtripper.go
```

```console
pkg/server/service/roundtripper.go:153:	// Return directly HTTP/1.1 transport when HTTP/2 is disabled
pkg/server/service/roundtripper.go:154:	if cfg.DisableHTTP2 {
pkg/server/service/roundtripper.go:163:	rt, err := newSmartRoundTripper(transport, cfg.ForwardingTimeouts)
pkg/server/service/smart_roundtripper.go:14:func newSmartRoundTripper(transport *http.Transport, forwardingTimeouts *dynamic.ForwardingTimeouts) (*smartRoundTripper, error) {
pkg/server/service/smart_roundtripper.go:17:	transportHTTP2, err := http2.ConfigureTransports(transport)
pkg/server/service/smart_roundtripper.go:32:			AllowHTTP: true,
pkg/server/service/smart_roundtripper.go:41:	transport.RegisterProtocol("h2c", transportH2C)
```

By default (`DisableHTTP2=false`) `createRoundTripper` returns the `smartRoundTripper`, whose main transport is HTTP/2-enabled via ALPN and which registers an `h2c` protocol handler (`http2.Transport{AllowHTTP: true}`) for `h2c://` server URLs. The branch's own migration guide (`docs/content/migration/v2.md:1143-1168`) documents `url: "h2c://..."` as the way "to reach a backend over unencrypted HTTP/2".

### Observed: backend forwarding negotiates HTTP/2 (TLS) and h2c

`verify/repro/c2_backend_forwarding_uses_http2_test.go` builds the production `service.Manager` with the default `ServersTransport` and proxies one GET to each of three backends that record `req.Proto`.

```sh
cd ~/repos/wt-c2 && cp ~/repos/traefik/verify/repro/c2_backend_forwarding_uses_http2_test.go pkg/server/service/
go test -count=1 -v -run '^(TestVerifyC2_BackendForwardingUsesHTTP2|TestDisableHTTP2)$' ./pkg/server/service
rm pkg/server/service/c2_backend_forwarding_uses_http2_test.go
```

```console
=== RUN   TestVerifyC2_BackendForwardingUsesHTTP2
    c2_backend_forwarding_uses_http2_test.go:79: service "h2" (server URL https://127.0.0.1:34587): backend received request over HTTP/2.0
    c2_backend_forwarding_uses_http2_test.go:79: service "h2c" (server URL h2c://127.0.0.1:34741): backend received request over HTTP/2.0
    c2_backend_forwarding_uses_http2_test.go:79: service "h1" (server URL http://127.0.0.1:46533): backend received request over HTTP/1.1
--- PASS: TestVerifyC2_BackendForwardingUsesHTTP2 (0.01s)
=== RUN   TestDisableHTTP2
...
--- PASS: TestDisableHTTP2 (0.00s)
    --- PASS: TestDisableHTTP2/HTTP1_capable_client_with_HTTP1_server (0.00s)
    --- PASS: TestDisableHTTP2/HTTP1_capable_client_with_HTTP2_server (0.00s)
    --- PASS: TestDisableHTTP2/HTTP2_capable_client_with_HTTP1_server (0.00s)
    --- PASS: TestDisableHTTP2/HTTP2_capable_client_with_HTTP2_server (0.00s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server/service	0.039s
```

The pre-existing `TestDisableHTTP2` case "HTTP2 capable client with HTTP2 server" (`expectedProto: "HTTP/2.0"`, `pkg/server/service/roundtripper_test.go:249-253`) passes on this branch too.

### Impact reasoning

The comment states as the rationale for the rejection that "the backend is reached over HTTP/1.1". On the same branch the default backend transport negotiates HTTP/2 over TLS (ALPN) and speaks HTTP/2 cleartext to `h2c://` servers; HTTP/1.1-only forwarding is the opt-in `disableHTTP2` path. The rejection itself is correct and unaffected; the problem is a misleading `why` comment in the security-relevant code path (a maintainer reading it could conclude CONNECT over an HTTP/2 backend is impossible and relax the guard, or reason wrongly about `h2c` backends). `pr.Out.ProtoMajor/ProtoMinor = 1/1` in `pkg/server/service/proxy.go` is likely the source of the confusion: it only normalises the outgoing request struct; the wire protocol is chosen by the transport.
