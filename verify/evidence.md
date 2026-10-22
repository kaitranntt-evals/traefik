## Setup

Audited branch: `origin/traefik-entrypoint-reject-connect-requests-perfect` at `3d4a73552`, checked out as `verify/traefik-entrypoint-reject-connect-requests-v-417004ed`. Base commit `246d9bfe7` (v2.11.58). Toolchain: `go version go1.26.3 linux/amd64`.

Claim-target branches live in a second repository and were fetched into separate worktrees:

```sh
cd ~/repos/traefik
git remote add evalon https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests
git fetch evalon evalon/traefik-en-896720cf evalon/traefik-en-a6408acd evalon/traefik-en-c340e9a5
git worktree add ../wt-896720cf evalon/evalon/traefik-en-896720cf   # c6b9e76b8
git worktree add ../wt-a6408acd evalon/evalon/traefik-en-a6408acd   # ed117ed75
git worktree add ../wt-c340e9a5 evalon/evalon/traefik-en-c340e9a5   # 5704f7a28
```

Eval fixture: `~/eval_tests/tests/eval_connect_test.go` (from the attached archive), copied to `pkg/server/eval_connect_test.go` where used.

## C1

Branch: `evalon/traefik-en-896720cf` (worktree `~/repos/wt-896720cf`).

### wrapper_composition — CONFIRMED

```sh
cd ~/repos/wt-896720cf && git diff 246d9bfe7 -- pkg/server/server_entrypoint_tcp.go
```

```diff
@@ -600,6 +600,9 @@ func createHTTPServer(ctx context.Context, ln net.Listener, configuration *stati
 		handler = newKeepAliveMiddleware(handler, configuration.Transport.KeepAliveMaxRequests, configuration.Transport.KeepAliveMaxTime)
 	}
 
+	// Reject CONNECT before routing, entrypoint middlewares, and access logging, including when no router is configured.
+	handler = denyConnect(handler)
+
 	var protocols http.Protocols
```

Wrapper order in `createHTTPServer` (later wrap = outer = executed first):

```sh
grep -n "handler = " pkg/server/server_entrypoint_tcp.go | sed -n 1,20p
```

```console
600:		handler = newKeepAliveMiddleware(handler, ...)
604:	handler = denyConnect(handler)
614:		handler = encodeQuerySemicolons(handler)
616:		handler = http.AllowQuerySemicolons(handler)
619:	handler = routingPath(handler)
624:		handler = sanitizePath(handler)
627:	handler = normalizePath(handler)
629:	handler = denyFragment(handler)
639:			handler = removeHeadersWithUnderscores(handler)
641:			handler = rejectHeadersWithUnderscores(handler)
648:		handler = removeAliasingHeaders(handler)
650:		handler = rejectAliasingHeaders(handler)
657:	handler = denyOpaque(handler)
659:	handler = populateRequestTLS(handler)
```

`denyConnect` (604) is wrapped before — i.e. placed *inside* — `denyFragment` (629) and every configurable header-policy handler (639–650). The request therefore traverses header policy and fragment validation before reaching `denyConnect`.

For comparison, the audited branch (`~/repos/traefik`, `git diff master...HEAD -- pkg/server/server_entrypoint_tcp.go`) inserts `handler = denyConnect(handler)` after the alias-header `switch` and immediately before `handler = denyOpaque(handler)`, i.e. outside both.

### request_behavior — CONFIRMED

Eval fixture on the target branch:

```sh
cd ~/repos/wt-896720cf && cp ~/eval_tests/tests/eval_connect_test.go pkg/server/eval_connect_test.go
go test -race -count=1 -timeout=120s ./pkg/server -run '^TestEvalConnect_HeaderPolicy_Precedence$' -v
```

```console
=== RUN   TestEvalConnect_HeaderPolicy_Precedence/AliasHeadersStrategy_Reject
    eval_connect_test.go:684: Error: Not equal: expected: 501  actual  : 400
        Messages:   	CONNECT carrying X_Test must receive 501 Not Implemented
=== RUN   TestEvalConnect_HeaderPolicy_Precedence/UnderscoreHeadersStrategy_Reject
    eval_connect_test.go:684: Error: Not equal: expected: 501  actual  : 400
        Messages:   	CONNECT carrying X_Underscore must receive 501 Not Implemented
=== RUN   TestEvalConnect_HeaderPolicy_Precedence/AliasHeadersStrategy_Delete
    eval_connect_test.go:686: Error: Not equal: expected: "test-value"  actual  : ""
        Messages:   	CONNECT request headers must not be mutated or deleted by entrypoint header policies
=== RUN   TestEvalConnect_HeaderPolicy_Precedence/UnderscoreHeadersStrategy_Delete
    eval_connect_test.go:686: Error: Not equal: expected: "test-value"  actual  : ""
--- FAIL: TestEvalConnect_HeaderPolicy_Precedence (0.00s)
FAIL	github.com/traefik/traefik/v2/pkg/server	0.101s
```

Live probe over real TCP against the served entrypoint stack (`createHTTPServer` + `Server.Serve`), repro file `verify/repro/c1_header_policy_connect_test.go`:

```sh
cp ~/repos/traefik/verify/repro/c1_header_policy_connect_test.go ~/repos/wt-896720cf/pkg/server/zz_audit_c1_test.go
cd ~/repos/wt-896720cf && go test -count=1 -timeout=60s ./pkg/server -run '^TestAuditC1_' -v
```

```console
=== RUN   TestAuditC1_HeaderPolicy/aliasHeadersStrategy=reject
    zz_audit_c1_test.go:78: HTTP/1.1 CONNECT + X_Test header -> 400 (downstream="")
        	Error:      	Not equal: expected: 501  actual  : 400
=== RUN   TestAuditC1_HeaderPolicy/underscoreHeadersStrategy=reject
    zz_audit_c1_test.go:78: HTTP/1.1 CONNECT + X_Test header -> 400 (downstream="")
        	Error:      	Not equal: expected: 501  actual  : 400
=== RUN   TestAuditC1_Fragment
    zz_audit_c1_test.go:105: HTTP/1.1 CONNECT /tunnel#frag -> 400
        	Error:      	Not equal: expected: 501  actual  : 400
--- FAIL: TestAuditC1_HeaderPolicy (0.00s)
--- FAIL: TestAuditC1_Fragment (0.00s)
FAIL	github.com/traefik/traefik/v2/pkg/server	0.036s
```

(The h2c probe in each sub-test is not reached because `require` stops the sub-test at the HTTP/1.1 400.)

Same probe on the audited branch (control):

```sh
cp ~/repos/traefik/verify/repro/c1_header_policy_connect_test.go ~/repos/traefik/pkg/server/zz_audit_c1_test.go
cd ~/repos/traefik && go test -count=1 -timeout=60s ./pkg/server -run '^TestAuditC1_' -v
```

```console
    zz_audit_c1_test.go:78: HTTP/1.1 CONNECT + X_Test header -> 501 (downstream="")
    zz_audit_c1_test.go:94: h2c CONNECT + X_Test header -> 501 (proto HTTP/2.0)
    zz_audit_c1_test.go:78: HTTP/1.1 CONNECT + X_Test header -> 501 (downstream="")
    zz_audit_c1_test.go:94: h2c CONNECT + X_Test header -> 501 (proto HTTP/2.0)
    zz_audit_c1_test.go:105: HTTP/1.1 CONNECT /tunnel#frag -> 501
    zz_audit_c1_test.go:105: HTTP/1.1 CONNECT http://backend.example.com/tunnel#frag -> 501
--- PASS: TestAuditC1_HeaderPolicy (0.00s)
--- PASS: TestAuditC1_Fragment (0.00s)
```

Impact reasoning: on `evalon/traefik-en-896720cf`, any entrypoint configured with `aliasHeadersStrategy: reject` or `underscoreHeadersStrategy: reject` (the configuration the entrypoint's own startup warning asks operators to set) answers a CONNECT carrying such a header with `400 Bad Request` instead of the contractual `501 Not Implemented`; with the `delete` strategies the header is stripped from the CONNECT request before rejection; a CONNECT whose target contains `#` gets `400` from `denyFragment`. The downstream handler is never invoked in any case (`downstream=""`), so no tunnel is opened — the defect is the wrong status code / policy side effects on CONNECT, contradicting "reject CONNECT before routing and before middleware execution" and the 501-everywhere contract. The audited branch does not have this defect.

## C2

Branch: `evalon/traefik-en-a6408acd` (worktree `~/repos/wt-a6408acd`).

### Client / context / transport controls

`pkg/server/server_entrypoint_tcp_test.go`:

```go
// line 1759-1768 (HTTP/2 CONNECT)
client := newH2CClient()
req, err := http.NewRequestWithContext(t.Context(), http.MethodConnect, "http://"+addr, http.NoBody)
resp, err := client.Do(req)

// line 1776-1788 (HTTP/2 CONNECT with tunnel payload)
client := newH2CClient()
pipeReader, pipeWriter := io.Pipe()
req, err := http.NewRequestWithContext(t.Context(), http.MethodConnect, "http://"+addr, pipeReader)
resp, err := client.Do(req)

// line 1972-1983
func newH2CClient() *http.Client {
	return &http.Client{
		Transport: &http2.Transport{
			AllowHTTP: true,
			DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
				var dialer net.Dialer
				return dialer.DialContext(ctx, network, addr)
			},
		},
	}
}
```

No `http.Client.Timeout`, no `ResponseHeaderTimeout`, request context is `t.Context()`. By contrast the HTTP/1.1 sub-test (line 1739) sets `conn.SetReadDeadline(time.Now().Add(10*time.Second))`.

`pkg/server/server_entrypoint_tcp_http3_test.go`, `TestHTTP3ConnectRejected` (line 532+):

```go
transport := &http3.Transport{TLSClientConfig: ..., Dial: ...}
req, err := http.NewRequestWithContext(t.Context(), http.MethodConnect, "https://example.com", http.NoBody)
resp, err := transport.RoundTrip(req)          // line 580
...
req, err = http.NewRequestWithContext(t.Context(), http.MethodConnect, "https://example.com", pipeReader)
resp, err = transport.RoundTrip(req)
```

No deadline on the context or transport. `grep -in "timeout\|time\.\|deadline"` over `startConnectTestEntryPoint` returns nothing.

### Baseline (unmutated)

```sh
cd ~/repos/wt-a6408acd
go test -count=1 -timeout=40s ./pkg/server -run '^TestConnectRejected$/^HTTP/2_CONNECT' -v
go test -count=1 -timeout=60s ./pkg/server -run '^TestHTTP3ConnectRejected$' -v
```

```console
--- PASS: TestConnectRejected (0.00s)
    --- PASS: TestConnectRejected/HTTP/2_CONNECT_with_tunnel_payload (0.02s)
    --- PASS: TestConnectRejected/HTTP/2_CONNECT (0.02s)
ok  	github.com/traefik/traefik/v2/pkg/server	0.094s
--- PASS: TestHTTP3ConnectRejected (0.01s)
ok  	github.com/traefik/traefik/v2/pkg/server	0.038s
```

### Mutation: denyConnect withholds its response

`verify/repro/c2_withhold_response.patch` (applied with `git apply`):

```diff
@@ -779,6 +779,7 @@ func denyConnect(h http.Handler) http.Handler {
 			return
 		}
 
+		<-req.Context().Done() // AUDIT MUTATION: withhold the response.
 		log.WithoutContext().Debugf("Rejecting %s CONNECT request from %s to %q: CONNECT is not supported", req.Proto, req.RemoteAddr, req.Host)
```

#### h2c_response_waits — CONFIRMED

```sh
time go test -count=1 -timeout=30s ./pkg/server -run '^TestConnectRejected$/^HTTP/2_CONNECT' -v
```

```console
=== RUN   TestConnectRejected/HTTP/2_CONNECT
=== RUN   TestConnectRejected/HTTP/2_CONNECT_with_tunnel_payload
panic: test timed out after 30s
goroutine 23 [select]:
golang.org/x/net/http2.(*ClientConn).internalRoundTrip(...)
golang.org/x/net/http2.(*ClientConn).RoundTrip(...)
net/http.(*Client).Do(...)
	/home/ubuntu/repos/wt-a6408acd/pkg/server/server_entrypoint_tcp_test.go:1764 +0x14d
goroutine 24 [select]:
golang.org/x/net/http2.(*ClientConn).internalRoundTrip(...)
net/http.(*Client).Do(...)
	/home/ubuntu/repos/wt-a6408acd/pkg/server/server_entrypoint_tcp_test.go:1784 +0x249
real	0m36.634s
```

Both h2c sub-tests (no-body at line 1764, open-pipe body at line 1784) were still blocked in `client.Do` when the suite `-timeout` fired; nothing test-local ended the wait.

#### http3_response_waits — CONFIRMED

```sh
time go test -count=1 -timeout=30s ./pkg/server -run '^TestHTTP3ConnectRejected$' -v
```

```console
=== RUN   TestHTTP3ConnectRejected
panic: test timed out after 30s
github.com/quic-go/quic-go/http3.(*ClientConn).RoundTrip(...)
github.com/quic-go/quic-go/http3.(*Transport).RoundTrip(...)
	/home/ubuntu/repos/wt-a6408acd/pkg/server/server_entrypoint_tcp_http3_test.go:580 +0x6e7
FAIL	github.com/traefik/traefik/v2/pkg/server	30.084s
real	0m35.500s
```

The first HTTP/3 CONNECT round trip (line 580) blocked until the suite `-timeout`.

Mutation reverted afterwards with `git checkout pkg/server/server_entrypoint_tcp.go` (worktree clean).

Impact reasoning: a regression in which the rejection path stops writing a response (or the server stalls) turns these tests into a package-wide `panic: test timed out` — under the eval's `-timeout=60s` every other `pkg/server` test is reported as failed/aborted with it, and under the Go default (10 m) CI blocks for ten minutes — instead of a localized assertion failure. The HTTP/1.1 sub-test in the same table shows the authors' own bounded pattern (10 s read deadline), which the h2c and HTTP/3 cases lack.

## C3

Branch: `evalon/traefik-en-c340e9a5` (worktree `~/repos/wt-c340e9a5`). No `style.md` exists in the checkout (`find . -name style.md` → nothing); the sleep rule referenced is the task's "Do not introduce ... arbitrary sleeps".

Startup-to-request sequence (`pkg/server/server_entrypoint_tcp_http3_test.go`, `TestHTTP3DenyConnect` starts at line 455):

```go
	ctx := t.Context()
	go entryPoint.Start(ctx)
	entryPoint.SwitchRouter(router)

	t.Cleanup(func() { entryPoint.Shutdown(ctx) })

	// We are racing with the http3Server readiness happening in the goroutine starting the entrypoint.
	time.Sleep(time.Second)                                   // line 497

	certPool := x509.NewCertPool()
	certPool.AppendCertsFromPEM(certContent)

	http3Addr := entryPoint.http3Server.http3conn.LocalAddr().String()

	transport := &http3.Transport{ ... Dial: quic.DialAddr(ctx, http3Addr, tlsCfg, cfg) ... }

	req, err := http.NewRequestWithContext(ctx, http.MethodConnect, "https://example.com", http.NoBody)
	res, err := transport.RoundTrip(req)
```

```sh
sh ~/repos/traefik/verify/repro/c3_sleep_only_readiness.sh   # run inside ~/repos/wt-c340e9a5
```

```console
--- readiness primitives between Start and RoundTrip:
43:	time.Sleep(time.Second)
55:		Dial: func(ctx context.Context, _ string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
56:			return quic.DialAddr(ctx, http3Addr, tlsCfg, cfg)
--- PASS: TestHTTP3DenyConnect (1.01s)
ok  	github.com/traefik/traefik/v2/pkg/server	1.040s
497:	// sleep removed by audit
--- PASS: TestHTTP3DenyConnect (0.01s)
ok  	github.com/traefik/traefik/v2/pkg/server	0.037s
```

Also run with the race detector as-is:

```sh
cd ~/repos/wt-c340e9a5 && go test -race -count=1 -timeout=120s ./pkg/server -run '^TestHTTP3DenyConnect$' -v
```

```console
--- PASS: TestHTTP3DenyConnect (1.02s)
ok  	github.com/traefik/traefik/v2/pkg/server	2.122s
```

Verdict reasoning: the only wait between `go entryPoint.Start(ctx)` and the request is `time.Sleep(time.Second)`; no channel receive, `require.Eventually`, probe dial, or other readiness signal appears (the only `<-` in the file are in the unrelated `TestHTTP30RTT`). The sleep is sole and fixed → CONFIRMED. The test takes 1.01 s with the sleep and 0.01 s without, i.e. the sleep contributes only elapsed time. (`http3Server` is created in `NewTCPEntryPoint`, line 212 of `server_entrypoint_tcp.go`, so `http3conn.LocalAddr()` is valid before `Start`; the UDP socket being pre-bound is why the sleep-less variant still passes here — not evidence of a readiness gate.) The same `// We are racing ... time.Sleep(time.Second)` idiom pre-exists in the four older HTTP/3 tests of that file (lines 135, 187, 270, 388).

## C4

Branch: audited `traefik-entrypoint-reject-connect-requests-perfect` (`3d4a73552`) at `~/repos/traefik`, `go1.26.3 linux/amd64`. `Makefile:test-unit` is `go test -cover "-coverprofile=cover.out" -v $(TESTFLAGS) ./pkg/... ./cmd/...`; the claim's command was run verbatim from the repository root, twice.

```sh
cd ~/repos/traefik && go test -count=1 -timeout=60s -cover -v ./pkg/server/... > /tmp/c4_run1.log 2>&1; echo EXIT=$?
cd ~/repos/traefik && go test -count=1 -timeout=60s -cover -v ./pkg/server/... > /tmp/c4_run2.log 2>&1; echo EXIT=$?
grep -a -n "^ok\|^FAIL\|^--- FAIL\|^panic\|EXIT=" /tmp/c4_run1.log
```

Run 1:

```console
ok  	github.com/traefik/traefik/v2/pkg/server	17.039s	coverage: 71.2% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/cookie	0.018s	coverage: 94.4% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/middleware	0.024s	coverage: 35.1% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/provider	0.017s	coverage: 100.0% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/router	0.774s	coverage: 82.8% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/router/tcp	9.317s	coverage: 76.0% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/router/udp	0.016s	coverage: 86.5% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service	0.259s	coverage: 57.7% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/loadbalancer/failover	0.016s	coverage: 92.0% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/loadbalancer/mirror	0.017s	coverage: 71.6% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/loadbalancer/wrr	0.013s	coverage: 96.8% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/tcp	0.013s	coverage: 84.3% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/udp	0.018s	coverage: 83.0% of statements
EXIT=0
```

Run 2:

```console
ok  	github.com/traefik/traefik/v2/pkg/server	16.395s	coverage: 71.8% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/cookie	0.008s	coverage: 94.4% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/middleware	0.022s	coverage: 35.1% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/provider	0.006s	coverage: 100.0% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/router	0.397s	coverage: 82.8% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/router/tcp	8.590s	coverage: 76.0% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/router/udp	0.016s	coverage: 86.5% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service	0.179s	coverage: 57.7% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/loadbalancer/failover	0.012s	coverage: 92.0% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/loadbalancer/mirror	0.029s	coverage: 71.6% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/loadbalancer/wrr	0.010s	coverage: 96.8% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/tcp	0.020s	coverage: 84.3% of statements
ok  	github.com/traefik/traefik/v2/pkg/server/service/udp	0.012s	coverage: 83.0% of statements
EXIT=0
```

Tallies:

```console
/tmp/c4_run1.log: PASS=132 FAIL=0 SKIP=0 ok_pkgs=13 FAIL_pkgs=0 EXIT=0
/tmp/c4_run2.log: PASS=132 FAIL=0 SKIP=0 ok_pkgs=13 FAIL_pkgs=0 EXIT=0
--- PASS: TestWebSocketTCPClose (0.00s)
--- PASS: TestWebSocketPingPong (0.00s)
--- PASS: TestWebSocketEcho (0.00s)
--- PASS: TestWebSocketPassHost (0.00s)
--- PASS: TestWebSocketServerWithoutCheckOrigin (0.00s)
```

(12 WebSocket-named tests pass in `pkg/server/service`.) No test failure, build failure or timeout → REFUTED.
