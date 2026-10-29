## C1

Claim: the added/modified HTTP/3 tests under `pkg/server/` leave a test-owned server UDP socket without an effective release path. Adjudicated per branch, each in its own worktree (`~/wt/ec41c631`, `~/wt/e97bf2b0`) created from the `kaitranntt-evals/traefik-entrypoint-reject-connect-requests` remote.

### Shared facts (both branches)

Installed HTTP/3 dependency on both branches (and on the base commit `246d9bfe7`):

```console
$ grep 'quic-go/quic-go' go.mod
	github.com/quic-go/quic-go v0.59.1
$ go list -m -f '{{.Dir}}' github.com/quic-go/quic-go
/home/ubuntu/go/pkg/mod/github.com/quic-go/quic-go@v0.59.1
```

Server-side UDP socket acquisition (`pkg/server/server_entrypoint_tcp_http3.go`, unchanged on both branches): `newHTTP3Server` calls `net.ListenPacket("udp", ...)` and stores it as `http3conn`; `Start()` calls `e.Serve(e.http3conn)`; `http3server.Shutdown` only calls `e.Server.Close()`. `TCPEntryPoint.Shutdown` calls `shutdownServer(e.http3Server)` and nothing else touches `http3conn`.

The installed quic-go documents and implements that `Serve(conn)` does not take ownership of the conn:

```console
$ grep -n 'func (s \*Server) Serve(' -B 3 $QG/http3/server.go
208-// Serve an existing UDP connection.
209-// It is possible to reuse the same connection for outgoing connections.
210-// Closing the server does not close the connection.
211:func (s *Server) Serve(conn net.PacketConn) error {
$ grep -n 'func (s \*Server) Close()' -A 18 $QG/http3/server.go | grep -n createdLocally
587:		if l.createdLocally {
```

(`Close()` only closes listeners it created locally — `Serve(conn)` listeners are not.)

### Probe

The committed repro files carry a `//go:build verify_c1` constraint so `go build ./...`/`go vet ./...` on this branch ignore them; copy them into `pkg/server/` of the audited branch and run with `-tags verify_c1` (the first runs below were made before the tag was added, hence line numbers differ by 3; a replay from the committed files on both branches gave the same `surviving teardown` counts — on e97bf2b0 the replay showed `after runtime.GC(): surviving: 1` for `TestConnectRejected`, i.e. whether the runtime finalizer happens to reclaim a socket varies run to run).

`verify/repro/c1/verify_c1_probe_test.go` (plus a per-branch `verify_c1_wrapped_<branch>_test.go` naming the branch's HTTP/3 tests) was copied into `pkg/server/` of each worktree. It:

1. enumerates UDP sockets owned by the test process (`/proc/self/fd` → `socket:[inode]` joined with `/proc/self/net/udp{,6}`) before and after running the branch's HTTP/3 tests as subtests via `t.Run(name, fn)` — so the wrapped test's `t.Cleanup` teardown has fully run before the "after" snapshot — then forces `runtime.GC()` and re-counts;
2. probes the owner teardown alone: `NewTCPEntryPoint` with `HTTP3`, `Start`, `Shutdown`, then calls `http3conn.SetReadDeadline` (returns `use of closed network connection` on a closed `*net.UDPConn`, `nil` if still open) and finally `http3conn.Close()`.

Client-side QUIC sockets (`quic.DialAddr`, bound to `0.0.0.0`) are distinguishable from the server socket (`net.ListenPacket("udp", "127.0.0.1:0")`); only `127.0.0.1` sockets were reported in every run, i.e. the client `http3.Transport.Close()` cleanup works and the survivors are the server sockets.

### Branch [evalon/traefik-en-ec41c631](https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests/tree/evalon/traefik-en-ec41c631) — CONFIRMED

Added HTTP/3 test: `TestHTTP3DenyConnect` in `pkg/server/server_entrypoint_tcp_http3_test.go`. Teardown registered by the test:

```go
ctx := t.Context()
go entryPoint.Start(ctx)
entryPoint.SwitchRouter(router)

t.Cleanup(func() { entryPoint.Shutdown(ctx) })
...
t.Cleanup(func() { _ = transport.Close() })
```

No `http3conn.Close()` anywhere in the file (`grep -n http3conn` only hits the `LocalAddr()` read at line ~500). Run:

```console
$ cd ~/wt/ec41c631 && cp ~/probe/verify_c1_probe_test.go ~/probe/verify_c1_wrapped_ec41c631_test.go pkg/server/
$ go test -tags verify_c1 -count=1 -v ./pkg/server -run '^TestVerifyC1_'
=== RUN   TestVerifyC1_WrappedTests
=== RUN   TestVerifyC1_WrappedTests/TestHTTP3DenyConnect
=== RUN   TestVerifyC1_WrappedTests/TestHTTP3DenyConnect/CONNECT
=== RUN   TestVerifyC1_WrappedTests/TestHTTP3DenyConnect/GET
    verify_c1_probe_test.go:103: [TestHTTP3DenyConnect] UDP sockets before=0 after=1; surviving teardown: 1
    verify_c1_probe_test.go:105: [TestHTTP3DenyConnect] LEAKED inode=83034 local=127.0.0.1:41392
    verify_c1_probe_test.go:112: [TestHTTP3DenyConnect] after runtime.GC(): surviving: 1
--- PASS: TestVerifyC1_WrappedTests (1.21s)
    --- PASS: TestVerifyC1_WrappedTests/TestHTTP3DenyConnect (1.00s)
=== RUN   TestVerifyC1_ShutdownLeavesHTTP3ConnOpen
    verify_c1_probe_test.go:149: server UDP socket (http3conn) bound at 127.0.0.1:56442
    verify_c1_probe_test.go:156: after entryPoint.Shutdown: http3conn.SetReadDeadline error = <nil> (nil means the socket is still open)
    verify_c1_probe_test.go:160: after entryPoint.Shutdown: process still holds UDP socket inode=82174 local=127.0.0.1:56442
    verify_c1_probe_test.go:164: explicit http3conn.Close() error = <nil> (nil means this call is what actually released it)
    verify_c1_probe_test.go:169: RESULT: entryPoint.Shutdown did NOT close the HTTP/3 server UDP socket; only the explicit Close released it
--- PASS: TestVerifyC1_ShutdownLeavesHTTP3ConnOpen (0.70s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	1.945s
```

The test itself passes under the eval's race command:

```console
$ go test -race -count=1 -timeout=60s ./pkg/server -run '^TestHTTP3DenyConnect$'
ok  	github.com/traefik/traefik/v2/pkg/server	2.100s
```

Impact reasoning: the test acquires exactly one server UDP socket (`127.0.0.1:41392`); after its complete `t.Cleanup` teardown that socket is still an open fd of the test process, and it is still open after two forced GCs. The only release path left is process exit (or an eventual, non-deterministic `netFD` finalizer once the `*TCPEntryPoint` becomes unreachable). No explicit close and no owner teardown closes it — `entryPoint.Shutdown` → `http3server.Shutdown` → `http3.Server.Close()` leaves a `Serve(conn)` socket open by design of the installed quic-go.

### Branch [evalon/traefik-en-e97bf2b0](https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests/tree/evalon/traefik-en-e97bf2b0) — CONFIRMED

Added HTTP/3-capable tests in `pkg/server/server_entrypoint_connect_test.go`: `TestConnectRejected` (2 sub-cases, each starting an entry point) and `TestConnectRejectedHTTP1NoTunnel`, both through the helper `startConnectTestEntryPoint`:

```go
ctx := t.Context()
go entryPoint.Start(ctx)
entryPoint.SwitchRouter(router)

t.Cleanup(func() { entryPoint.Shutdown(context.Background()) })

return entryPoint.listener.Addr().String(), entryPoint.http3Server.http3conn.LocalAddr().String()
```

`grep -n http3conn` in the file only hits that `LocalAddr()` read (line 300); the client cleanup at lines 347-353 closes the four client transports and `h3.Close()` only. Run:

```console
$ cd ~/wt/e97bf2b0 && cp ~/probe/verify_c1_probe_test.go ~/probe/verify_c1_wrapped_e97bf2b0_test.go pkg/server/
$ go test -tags verify_c1 -count=1 -v ./pkg/server -run '^TestVerifyC1_'
    verify_c1_probe_test.go:103: [TestConnectRejected] UDP sockets before=0 after=2; surviving teardown: 2
    verify_c1_probe_test.go:105: [TestConnectRejected] LEAKED inode=80117 local=127.0.0.1:53981
    verify_c1_probe_test.go:105: [TestConnectRejected] LEAKED inode=83052 local=127.0.0.1:37649
    verify_c1_probe_test.go:112: [TestConnectRejected] after runtime.GC(): surviving: 2
    verify_c1_probe_test.go:103: [TestConnectRejectedHTTP1NoTunnel] UDP sockets before=2 after=3; surviving teardown: 1
    verify_c1_probe_test.go:105: [TestConnectRejectedHTTP1NoTunnel] LEAKED inode=81194 local=127.0.0.1:44315
    verify_c1_probe_test.go:112: [TestConnectRejectedHTTP1NoTunnel] after runtime.GC(): surviving: 0
--- PASS: TestVerifyC1_WrappedTests (0.94s)
    --- PASS: TestVerifyC1_WrappedTests/TestConnectRejected (0.53s)
    --- PASS: TestVerifyC1_WrappedTests/TestConnectRejectedHTTP1NoTunnel (0.00s)
=== RUN   TestVerifyC1_ShutdownLeavesHTTP3ConnOpen
    verify_c1_probe_test.go:149: server UDP socket (http3conn) bound at 127.0.0.1:36161
    verify_c1_probe_test.go:156: after entryPoint.Shutdown: http3conn.SetReadDeadline error = <nil> (nil means the socket is still open)
    verify_c1_probe_test.go:160: after entryPoint.Shutdown: process still holds UDP socket inode=79362 local=127.0.0.1:36161
    verify_c1_probe_test.go:164: explicit http3conn.Close() error = <nil> (nil means this call is what actually released it)
    verify_c1_probe_test.go:169: RESULT: entryPoint.Shutdown did NOT close the HTTP/3 server UDP socket; only the explicit Close released it
--- PASS: TestVerifyC1_ShutdownLeavesHTTP3ConnOpen (0.70s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	1.670s
```

The tests themselves pass under the eval's race command:

```console
$ go test -race -count=1 -timeout=60s ./pkg/server -run '^(TestConnectRejected|TestConnectRejectedHTTP1NoTunnel)$'
ok  	github.com/traefik/traefik/v2/pkg/server	1.644s
```

Impact reasoning: every entry point started by these tests (three in total) leaves its server UDP socket open after the test's full teardown. The one socket that disappeared after a forced `runtime.GC()` in the third snapshot was reclaimed by the runtime's `netFD` finalizer, not by any test or owner code — the two sockets from `TestConnectRejected` survived the same forced GC in the previous snapshot — so the GC is not an effective release path; the deterministic release is process exit.

### Contrast: the eval fixture closes the socket itself

The eval fixture `tests/eval_connect_test.go` (provisioned at `pkg/server/eval_connect_test.go`) registers, in both HTTP/3 tests:

```go
t.Cleanup(func() {
    entryPoint.Shutdown(ctx)
    if entryPoint.http3Server != nil && entryPoint.http3Server.http3conn != nil {
        _ = entryPoint.http3Server.http3conn.Close()
    }
})
```

Wrapping those two fixture tests with the same probe on the audited `verify/...` checkout (fixture copied byte-exact into `pkg/server/`, then removed again before committing):

```console
$ go test -tags verify_c1 -count=1 -v ./pkg/server -run '^TestVerifyC1_WrappedTests$'
    verify_c1_probe_test.go:103: [TestEvalConnect_HTTP3_QUIC] UDP sockets before=0 after=0; surviving teardown: 0
    verify_c1_probe_test.go:112: [TestEvalConnect_HTTP3_QUIC] after runtime.GC(): surviving: 0
    verify_c1_probe_test.go:103: [TestEvalConnect_HTTP3_ExtendedConnect] UDP sockets before=0 after=0; surviving teardown: 0
    verify_c1_probe_test.go:112: [TestEvalConnect_HTTP3_ExtendedConnect] after runtime.GC(): surviving: 0
--- PASS: TestVerifyC1_WrappedTests (0.45s)
ok  	github.com/traefik/traefik/v2/pkg/server	0.477s
```

So an explicit `http3conn.Close()` after `Shutdown` is a sufficient fix on both audited branches.

## C2

Claim: the added/modified tests using `serveProxy` under `pkg/server/` on [evalon/traefik-en-0f9b28ca](https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests/tree/evalon/traefik-en-0f9b28ca) create a new test-owned backend keep-alive transport without pool cleanup. Adjudicated in worktree `~/wt/0f9b28ca`.

### What the branch contains

```console
$ grep -rn 'serveProxy' pkg/server --include=*_test.go
pkg/server/service/connect_test.go:48:			addr := serveProxy(t, backendURL)
pkg/server/service/connect_test.go:72:// serveProxy exposes the Traefik proxy handler over a httptest server,
pkg/server/service/connect_test.go:74:func serveProxy(t *testing.T, target *url.URL) string {
```

The only caller is the added test `TestConnect_Rejected` (3 parallel sub-cases). `serveProxy` itself is byte-identical to the base commit:

```console
$ diff <(git show 246d9bfe790fcd91088e25121054871e1aac1000:pkg/server/service/connect_test.go | sed -n '/^func serveProxy/,/^}/p') \
       <(sed -n '/^func serveProxy/,/^}/p' pkg/server/service/connect_test.go) && echo "serveProxy IDENTICAL to base"
serveProxy IDENTICAL to base
```

Its backend transport is the shared default, passed straight through:

```go
proxy, err := buildProxy(new(true), nil, http.DefaultTransport, newBufferPool())
```

and `buildProxy` (`pkg/server/service/proxy.go:108`) sets `Transport: roundTripper` on the `httputil.ReverseProxy` without cloning or wrapping it. The only `*http.Transport` values the test creates are client-side (`&http.Transport{Protocols: protocols}` for talking to the httptest server), each with `t.Cleanup(transport.CloseIdleConnections)`.

### Probe

The committed repro file carries a `//go:build verify_c2` constraint; copy it into `pkg/server/service/` of the audited branch and run with `-tags verify_c2` (a replay from the committed file gave identical output, line numbers +3).

`verify/repro/c2/verify_c2_probe_test.go` copied into `pkg/server/service/`: (1) builds the proxy exactly as `serveProxy` does, unwraps the `connectHandler`/`h2cUpgradeHandler` wrappers and checks pointer identity of the inner `ReverseProxy.Transport` with `http.DefaultTransport`; (2) runs `TestConnect_Rejected` as a subtest and enumerates process-owned TCP sockets (`/proc/self/fd` joined with `/proc/self/net/tcp{,6}`) and goroutine counts before/after.

```console
$ cd ~/wt/0f9b28ca && cp ~/probe/verify_c2_probe_test.go pkg/server/service/
$ go test -tags verify_c2 -count=1 -v ./pkg/server/service -run '^TestVerifyC2_'
=== RUN   TestVerifyC2_ServeProxyTransportIdentity
    verify_c2_probe_test.go:64: unwrapping *service.connectHandler
    verify_c2_probe_test.go:68: unwrapping *service.h2cUpgradeHandler
    verify_c2_probe_test.go:77: proxy.Transport == http.DefaultTransport: true (proxy.Transport=0x4b43ac0, http.DefaultTransport=0x4b43ac0)
--- PASS: TestVerifyC2_ServeProxyTransportIdentity (0.00s)
=== RUN   TestVerifyC2_WrappedTestSockets
=== RUN   TestVerifyC2_WrappedTestSockets/TestConnect_Rejected
    verify_c2_probe_test.go:100: TCP sockets before=0 after=0; surviving teardown: 0
    verify_c2_probe_test.go:104: goroutines before=5 after=5
--- PASS: TestVerifyC2_WrappedTestSockets (0.21s)
    --- PASS: TestVerifyC2_WrappedTestSockets/TestConnect_Rejected (0.00s)
        --- PASS: TestVerifyC2_WrappedTestSockets/TestConnect_Rejected/HTTP/1 (0.00s)
        --- PASS: TestVerifyC2_WrappedTestSockets/TestConnect_Rejected/HTTP/2 (0.00s)
        --- PASS: TestVerifyC2_WrappedTestSockets/TestConnect_Rejected/HTTP/2_with_payload (0.00s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server/service	0.235s
```

The branch test passes under the eval's race command:

```console
$ go test -race -count=1 -timeout=60s ./pkg/server/service -run '^TestConnect_Rejected$' -v 2>&1 | grep -E '^(--- |PASS|FAIL|ok)'
--- PASS: TestConnect_Rejected (0.00s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server/service	1.082s
```

Conclusion: the `serveProxy` path borrows the process-wide `http.DefaultTransport` (same pointer), creates no backend keep-alive transport, and — since every request is rejected with 501 before the proxy dials (`backendCalled` stays false) — no backend connection is ever pooled: zero TCP sockets and zero goroutines survive teardown. The claim's premise (a *new* test-owned backend transport) does not hold.
