## C1

Claim: the added TCP test under `pkg/server/` leaves its test-owned entrypoint listener open after the test and its registered cleanup complete.

Target branch: `evalon/traefik-en-668b3ecf` in repository `kaitranntt-evals/traefik-entrypoint-reject-connect-requests` (not on `origin`; fetched as a second remote). The added TCP test on that branch is `TestConnectTCPRoutingUnaffected` in `pkg/server/server_entrypoint_tcp_test.go` (the claim's "pkg/server/service/connect_test.go" is deleted by that branch and contains no TCP test; the nearest equivalent is the test above).

### Setup

```sh
cd ~/repos/traefik
git remote add claimrepo https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests
git fetch claimrepo evalon/traefik-en-668b3ecf
git worktree add ~/repos/traefik-c1 claimrepo/evalon/traefik-en-668b3ecf
cd ~/repos/traefik-c1
git log --oneline -2
git diff --stat 246d9bfe790fcd91088e25121054871e1aac1000 HEAD
```

```console
29d773ebb chore: apply eval changes
246d9bfe7 Prepare release v2.11.58
 docs/content/migration/v2.md                   |  20 ++
 pkg/server/server_entrypoint_tcp.go            |  22 ++
 pkg/server/server_entrypoint_tcp_http3_test.go |  84 ++++++++
 pkg/server/server_entrypoint_tcp_test.go       | 277 ++++++++++++++++++++++++-
 pkg/server/service/connect.go                  | 130 ------------
 pkg/server/service/connect_test.go             | 222 --------------------
 pkg/server/service/proxy.go                    |   8 +-
```

### Static trace (allocation and teardown paths)

`TestConnectTCPRoutingUnaffected` (added, `pkg/server/server_entrypoint_tcp_test.go` on the target branch):

```go
	entryPoint, err := NewTCPEntryPoint(t.Context(), &static.EntryPoint{
		Address:          "127.0.0.1:0",
		...
	}, nil)
	require.NoError(t, err)

	conn, err := startEntrypoint(t, entryPoint, router)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
```

- Allocation: `NewTCPEntryPoint` → `buildListener` → `net.ListenConfig.Listen(ctx, "tcp", "127.0.0.1:0")` (`pkg/server/server_entrypoint_tcp.go:167`, `:476-499`). The listener is owned by the `TCPEntryPoint` created by the test.
- Start: the pre-existing helper `startEntrypoint` (`server_entrypoint_tcp_test.go:160-178`) runs `go entryPoint.Start(t.Context())` and dials `entryPoint.listener.Addr()`. `Start` (`server_entrypoint_tcp.go:238-293`) loops on `e.listener.Accept()` and never observes `ctx.Done()`; the accept loop only ends when `Accept` returns a non-temporary error (i.e. the listener is closed).
- Teardown: the only cleanup registered by the test is `conn.Close()` on the *client* connection. `startEntrypoint` registers no cleanup. The test never calls `entryPoint.Shutdown`, never closes `entryPoint.listener`, and there is no owning wrapper. By contrast the sibling test added on the same branch, `TestHTTP3ConnectRejected`, does register `t.Cleanup(func() { entryPoint.Shutdown(ctx) })`.

### Runtime evidence: listener survives test + cleanup

Repro: [verify/repro/zz_verify_c1_listener_leak_test.go](repro/zz_verify_c1_listener_leak_test.go). It runs the audited test via `t.Run` (which returns only after the subtest's `t.Cleanup` callbacks complete), then diffs the LISTEN TCP sockets whose inode is held by an fd of the test process (`/proc/self/fd` × `/proc/self/net/tcp{,6}`), dials any new one and writes bytes through it.

```sh
cp verify/repro/zz_verify_c1_listener_leak_test.go ~/repos/traefik-c1/pkg/server/
cd ~/repos/traefik-c1
go test -count=1 -race -timeout=120s ./pkg/server -run '^(TestConnectTCPRoutingUnaffected|TestVerifyC1_TCPTestLeaksListener)$' -v
```

```console
=== RUN   TestConnectTCPRoutingUnaffected
--- PASS: TestConnectTCPRoutingUnaffected (0.00s)
=== RUN   TestVerifyC1_TCPTestLeaksListener
    zz_verify_c1_listener_leak_test.go:66: LISTEN sockets owned by process before: map[127.0.0.1:42897:{}]
=== RUN   TestVerifyC1_TCPTestLeaksListener/TestConnectTCPRoutingUnaffected
=== NAME  TestVerifyC1_TCPTestLeaksListener
    zz_verify_c1_listener_leak_test.go:73: LISTEN sockets owned by process after test+cleanup: map[127.0.0.1:42897:{} 127.0.0.1:46849:{}]
    zz_verify_c1_listener_leak_test.go:81: new LISTEN sockets still open after the audited test completed: [127.0.0.1:46849]
    zz_verify_c1_listener_leak_test.go:95: dial 127.0.0.1:46849 ok; wrote PING, read "PING\r\n" err=<nil> (echo => the leaked entrypoint still accepts and routes)
    zz_verify_c1_listener_leak_test.go:100: VERDICT: listener [127.0.0.1:46849] leaked by TestConnectTCPRoutingUnaffected
--- PASS: TestVerifyC1_TCPTestLeaksListener (0.00s)
    --- PASS: TestVerifyC1_TCPTestLeaksListener/TestConnectTCPRoutingUnaffected (0.00s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	1.080s
```

Reading of the output:

- The "before" snapshot already contains `127.0.0.1:42897`: that is the listener left behind by the standalone `TestConnectTCPRoutingUnaffected` run that preceded the wrapper in the same binary — a first observation of the leak.
- After the wrapped run and its cleanup (`conn.Close()`) completed, a second listener `127.0.0.1:46849` is still in LISTEN state and owned by the test process.
- Dialing it succeeds and the bytes `PING\r\n` are echoed back: the entrypoint's `Start` accept loop and the test's TCP echo router are still live, so closing the client connection did not release anything.

A second run after extending the helper to also scan `/proc/self/net/tcp6` (same repro file, final version), together with the context tests:

```sh
cp verify/repro/zz_verify_c1_*_test.go ~/repos/traefik-c1/pkg/server/
cd ~/repos/traefik-c1
go vet ./pkg/server/
go test -count=1 -race -timeout=120s ./pkg/server -run '^TestVerifyC1_(TCPTestLeaksListener|PreexistingPatternAlsoLeaks)$' -v
```

```console
=== RUN   TestVerifyC1_PreexistingPatternAlsoLeaks
=== RUN   TestVerifyC1_PreexistingPatternAlsoLeaks/TestReadTimeoutWithoutFirstByte
=== NAME  TestVerifyC1_PreexistingPatternAlsoLeaks
    zz_verify_c1_extra_test.go:20: pre-existing TestReadTimeoutWithoutFirstByte also leaves LISTEN socket open: [::]:42127
--- PASS: TestVerifyC1_PreexistingPatternAlsoLeaks (2.02s)
=== RUN   TestVerifyC1_TCPTestLeaksListener
    zz_verify_c1_listener_leak_test.go:70: LISTEN sockets owned by process before: map[[::]:42127:{}]
=== RUN   TestVerifyC1_TCPTestLeaksListener/TestConnectTCPRoutingUnaffected
=== NAME  TestVerifyC1_TCPTestLeaksListener
    zz_verify_c1_listener_leak_test.go:77: LISTEN sockets owned by process after test+cleanup: map[127.0.0.1:38365:{} [::]:42127:{}]
    zz_verify_c1_listener_leak_test.go:85: new LISTEN sockets still open after the audited test completed: [127.0.0.1:38365]
    zz_verify_c1_listener_leak_test.go:99: dial 127.0.0.1:38365 ok; wrote PING, read "PING\r\n" err=<nil> (echo => the leaked entrypoint still accepts and routes)
    zz_verify_c1_listener_leak_test.go:104: VERDICT: listener [127.0.0.1:38365] leaked by TestConnectTCPRoutingUnaffected
--- PASS: TestVerifyC1_TCPTestLeaksListener (0.01s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	3.103s
```

### Runtime evidence: `entryPoint.Shutdown` would release it (fix check)

Repro: [verify/repro/zz_verify_c1_extra_test.go](repro/zz_verify_c1_extra_test.go), `TestVerifyC1_ShutdownReleasesListener` — same setup as the audited test plus `t.Cleanup(func() { entryPoint.Shutdown(t.Context()) })`.

```sh
cd ~/repos/traefik-c1
go test -count=1 -race -timeout=120s ./pkg/server -run '^TestVerifyC1_ShutdownReleasesListener$' -v
```

```console
=== RUN   TestVerifyC1_ShutdownReleasesListener/withShutdown
    zz_verify_c1_extra_test.go:42: listener addr: 127.0.0.1:38551
time="2026-11-01T01:45:26Z" level=error msg="accept tcp 127.0.0.1:38551: use of closed network connection"
time="2026-11-01T01:45:26Z" level=error msg="close tcp 127.0.0.1:38551: use of closed network connection"
=== NAME  TestVerifyC1_ShutdownReleasesListener
    zz_verify_c1_extra_test.go:57: before=map[] after=map[]
--- PASS: TestVerifyC1_ShutdownReleasesListener (0.20s)
```

With `Shutdown` registered, the accept loop exits (`accept tcp ...: use of closed network connection`) and no LISTEN socket owned by the process remains.

### Impact reasoning

- The leak is confined to the `go test` process: one `127.0.0.1:<ephemeral>` listener plus its `Start` goroutine, `http.Server`s and `httpForwarder` goroutines stay alive until the test binary exits. It does not affect Traefik's production code or runtime behaviour; the production `denyConnect` change is untouched by this finding.
- Effects for someone working in the package: goroutine/port leaks accumulate across the `pkg/server` test binary (every other `startEntrypoint`-based test on the base commit, e.g. `TestReadTimeoutWithoutFirstByte`, has the same pre-existing pattern, so the added test follows the local convention rather than introducing a new one), and any future goroutine-leak checker (`goleak`) or listener accounting in this package would flag it.
- The branch's own HTTP/3 test (`TestHTTP3ConnectRejected`) shows the one-line fix used elsewhere on the same branch: `t.Cleanup(func() { entryPoint.Shutdown(t.Context()) })`.

Verdict: CONFIRMED — the entrypoint listener is allocated by `NewTCPEntryPoint` on the test's behalf, no cleanup/`Shutdown`/wrapper teardown closes it, and it remains open and accepting connections after the test and its registered cleanup (`conn.Close()`) complete.
