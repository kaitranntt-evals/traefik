## C1

Claim: `TestHTTP3ConnectRejected` closes `routedMethods` concurrently with an unsynchronized handler send to that channel, producing a data race under `-race`.

Target branch: [evalon/traefik-en-f1b2f540](https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests/tree/evalon/traefik-en-f1b2f540) (commit `6ee040d72a6a0b0bfe63038238827f96d81223f1`, repo `kaitranntt-evals/traefik-entrypoint-reject-connect-requests`, not `origin`). Checked out into a separate worktree:

```sh
cd ~/repos/traefik
git remote add claimrepo https://github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests
git fetch claimrepo evalon/traefik-en-f1b2f540
git worktree add ~/wt-f1b2f540 claimrepo/evalon/traefik-en-f1b2f540
```

### Channel lifecycle (source inspection)

```sh
cd ~/wt-f1b2f540 && grep -n "routedMethods" pkg/server/server_entrypoint_tcp_http3_test.go
```

```console
555:	routedMethods := make(chan string, 2)
560:		routedMethods <- req.Method
605:	close(routedMethods)
607:	for method := range routedMethods {
```

Line 560 is inside the `http.HandlerFunc` installed via `router.SetHTTPSHandler(...)`; it runs on the HTTP/3 server's per-request goroutine and sends before `rw.WriteHeader(http.StatusOK)`. Line 605 is executed by the test goroutine right after `transport.RoundTrip(GET)` returns. The only ordering between the two is the QUIC/UDP round trip (client receives the response the handler wrote after sending). There is no `sync.WaitGroup`, mutex, done-channel, or `entryPoint.Shutdown` before `close(routedMethods)`; the race detector sees no happens-before edge across the socket.

### Race detector run (3 separate `go test -race` invocations)

```sh
cd ~/wt-f1b2f540
for i in 1 2 3; do go test -race -count=1 -timeout=120s -run '^TestHTTP3ConnectRejected$' ./pkg/server/ > /tmp/race_$i.log 2>&1; echo "run $i exit=$?"; grep -c "WARNING: DATA RACE" /tmp/race_$i.log; done
```

```console
run 1 exit=0
0
run 2 exit=1
1
run 3 exit=0
0
```

Key lines from `/tmp/race_2.log`:

```console
WARNING: DATA RACE
Write at 0x00c00039f9e0 by goroutine 25:
  runtime.closechan()
      /usr/local/go/src/runtime/chan.go:414 +0x0
  github.com/traefik/traefik/v2/pkg/server.TestHTTP3ConnectRejected()
      /home/ubuntu/wt-f1b2f540/pkg/server/server_entrypoint_tcp_http3_test.go:605 +0x1313
  testing.tRunner()

Previous read at 0x00c00039f9e0 by goroutine 60:
  runtime.chansend()
      /usr/local/go/src/runtime/chan.go:176 +0x0
  github.com/traefik/traefik/v2/pkg/server.TestHTTP3ConnectRejected.func1()
      /home/ubuntu/wt-f1b2f540/pkg/server/server_entrypoint_tcp_http3_test.go:560 +0x64
  net/http.HandlerFunc.ServeHTTP()
  github.com/traefik/traefik/v2/pkg/middlewares.(*HTTPHandlerSwitcher).ServeHTTP()
  ...
--- FAIL: TestHTTP3ConnectRejected (1.02s)
    testing.go:1712: race detected during execution of test
FAIL
FAIL	github.com/traefik/traefik/v2/pkg/server	1.122s
```

Note: `go test -race -count=10 -run '^TestHTTP3ConnectRejected$' ./pkg/server/` is not a usable repetition method — the test hardcodes `127.0.0.1:8093`, so iterations 2..10 fail at line 550 with `listen udp 127.0.0.1:8093: bind: address already in use`. Repetition was therefore done with one process per run.

### Frequency (20 separate process runs of the race-built test binary)

```sh
cd ~/wt-f1b2f540 && go test -c -race -o /tmp/server_race.test ./pkg/server/
cd ~/wt-f1b2f540/pkg/server && races=0; fails=0; for i in $(seq 1 20); do /tmp/server_race.test -test.run '^TestHTTP3ConnectRejected$' -test.count=1 > /tmp/bin_$i.log 2>&1 || fails=$((fails+1)); if grep -q "WARNING: DATA RACE" /tmp/bin_$i.log; then races=$((races+1)); fi; sleep 0.3; done; echo "runs=20 races=$races fails=$fails"
for f in $(grep -l "DATA RACE" /tmp/bin_*.log); do grep -o "server_entrypoint_tcp_http3_test.go:[0-9]*" $f | sort -u | tr '\n' ' '; echo; done | sort | uniq -c
grep -h "race detected during execution" /tmp/bin_*.log | sort | uniq -c
```

```console
runs=20 races=8 fails=8
      8 server_entrypoint_tcp_http3_test.go:560 server_entrypoint_tcp_http3_test.go:605
      8     testing.go:1712: race detected during execution of test
```

Every one of the 8 race reports names exactly the two operations in the claim: `runtime.chansend` at line 560 (handler) and `runtime.closechan` at line 605 (test body). The 12 non-racing runs passed; no other failure mode was observed. `go vet ./pkg/server/` on the branch is clean.

Scripted replay: `bash verify/repro/c1_http3_connect_rejected_race.sh 20` (exits 1 and prints the first report when any run races).

### Impact reasoning

- The task's own test guideline requires every new regression test to be run with `-race -count=1`; this test fails that requirement roughly 40% of the time on this machine, so CI with `-race` is flaky for `./pkg/server`.
- The race is real, not a detector artifact: the handler goroutine's `routedMethods <- req.Method` is ordered before `close(routedMethods)` only by network I/O, which the Go memory model does not treat as synchronization. A send on the closed channel would panic the HTTP/3 server goroutine.
- Production code is unaffected (the problem is confined to the test). Minimal fix: call `entryPoint.Shutdown(ctx)` (or wait on a `sync.WaitGroup`/done channel signalled by the handler) before `close(routedMethods)`, or drop the close and read the buffered channel with a `select`/timeout.

Verdict: CONFIRMED.
