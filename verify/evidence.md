## C1

Claim: the solution's CONNECT tests do not complete successfully under Go's race detector in the provided environment.

Target branch: `evalon/traefik-en-c074d1a5` of `kaitranntt-evals/traefik-entrypoint-reject-connect-requests`, commit `8000391cb` ("Reject CONNECT requests on all HTTP versions"), checked out in a separate worktree at `/home/ubuntu/wt-c1`. Go toolchain: `go1.26.3 linux/amd64`.

Solution CONNECT tests on that branch: `Test_denyConnect`, `TestConnectRejected` (pkg/server/server_entrypoint_tcp_test.go) and `TestHTTP3ConnectRejected` (pkg/server/server_entrypoint_tcp_http3_test.go). The eval fixtures were copied byte-exact to the paths assessment provisions them at.

### Setup

```sh
cd ~/repos/traefik
git remote add claimrepo https://git-manager.itsdev.in/proxy/github.com/kaitranntt-evals/traefik-entrypoint-reject-connect-requests
git fetch claimrepo evalon/traefik-en-c074d1a5
git worktree add /home/ubuntu/wt-c1 claimrepo/evalon/traefik-en-c074d1a5
cd /home/ubuntu/wt-c1
cp /home/ubuntu/eval_tests/tests/eval_connect_test.go pkg/server/eval_connect_test.go
cp /home/ubuntu/eval_tests/tests/eval_candidate_test_inventory.go eval_candidate_test_inventory.go
cp /home/ubuntu/eval_tests/tests/eval_run_candidate_race_tests.sh eval_run_candidate_race_tests.sh
go build ./... && go vet ./pkg/server/...   # both exit 0
```

### 1. All CONNECT tests (solution + fixture) under -race, verbose

```sh
cd /home/ubuntu/wt-c1 && go test -race -count=1 -timeout=120s -v ./pkg/server \
  -run '^(Test_denyConnect|TestConnectRejected|TestHTTP3ConnectRejected|TestEvalConnect_.*)$'
```

```console
--- PASS: TestEvalConnect_AuthorityForm_RawTCP (0.00s)
--- PASS: TestEvalConnect_H2_Wire_Ingress (0.23s)
--- PASS: TestEvalConnect_AllowedMethods_Preserved (0.00s)
--- PASS: TestEvalConnect_UnroutedEntrypoint (0.00s)
--- PASS: TestEvalConnect_HTTP3_QUIC (0.18s)
--- PASS: TestEvalConnect_AccessLog_Suppression (0.00s)
--- PASS: TestEvalConnect_PreRoutingValidation_Preserved (0.00s)
--- PASS: TestEvalConnect_DebugLog_Emission (0.00s)
--- PASS: TestEvalConnect_ConcurrentBurst_Race (0.01s)
--- PASS: TestEvalConnect_HeaderPolicy_Precedence (0.00s)
--- PASS: TestHTTP3ConnectRejected (1.02s)
--- PASS: Test_denyConnect (0.00s)
--- PASS: TestConnectRejected (0.03s)
PASS
ok  	github.com/traefik/traefik/v2/pkg/server	2.576s
```

Exit code 0. `grep -c 'DATA RACE' /tmp/connect_race.log` → `0`.

### 2. Repeated -race -count=1 runs of the solution's CONNECT tests (flakiness check)

```sh
cd /home/ubuntu/wt-c1 && for i in 1 2 3 4 5 6 7 8; do \
  go test -race -count=1 -timeout=60s ./pkg/server -run '^(TestHTTP3ConnectRejected|TestConnectRejected|Test_denyConnect)$' 2>&1 | tail -1; done
```

```console
ok  	github.com/traefik/traefik/v2/pkg/server	2.194s
ok  	github.com/traefik/traefik/v2/pkg/server	2.229s
ok  	github.com/traefik/traefik/v2/pkg/server	2.188s
ok  	github.com/traefik/traefik/v2/pkg/server	2.156s
ok  	github.com/traefik/traefik/v2/pkg/server	2.301s
ok  	github.com/traefik/traefik/v2/pkg/server	2.268s
ok  	github.com/traefik/traefik/v2/pkg/server	2.197s
ok  	github.com/traefik/traefik/v2/pkg/server	2.303s
```

8/8 green.

### 3. The eval race runner script

Note: the script does `cd "$HOME/repos/traefik"` when that directory exists, so run from the worktree it must be given a different `HOME`, otherwise it silently measures the main checkout (first attempt did exactly that and selected only `TestHTTP2RequestTLS|TestRequestTargetForms|Test_denyConnect` from the `perfect` branch).

```sh
cd /home/ubuntu/wt-c1 && mkdir -p /tmp/fakehome && HOME=/tmp/fakehome bash eval_run_candidate_race_tests.sh; echo RC=$?
```

```console
Running candidate tests under -race in ./pkg/server: ^(TestConnectRejected|TestEvalConnect_AccessLog_Suppression|TestEvalConnect_AllowedMethods_Preserved|TestEvalConnect_AuthorityForm_RawTCP|TestEvalConnect_ConcurrentBurst_Race|TestEvalConnect_DebugLog_Emission|TestEvalConnect_H2_Wire_Ingress|TestEvalConnect_HTTP3_QUIC|TestEvalConnect_HeaderPolicy_Precedence|TestEvalConnect_PreRoutingValidation_Preserved|TestEvalConnect_UnroutedEntrypoint|TestHTTP2RequestTLS|TestHTTP3ConnectRejected|TestRequestTargetForms|Test_denyConnect)$
[FAIL] observed subtest 'github.com/traefik/traefik/v2/pkg/server:TestConnectRejected/no_matching_router/ordinary_request' terminated with non-pass action: skip
RC=1
```

The `go test -race` invocation inside the script exited 0 (the script did not print `[FAIL] go test exited with error code`); the only accounting error is the GET control subtest at `pkg/server/server_entrypoint_tcp_test.go:1818`:

```go
t.Run("ordinary request", func(t *testing.T) {
	if rt.handler == nil {
		t.Skip("no router to forward to")
	}
```

Every CONNECT root test (`TestConnectRejected`, `TestHTTP3ConnectRejected`, `Test_denyConnect`) and every CONNECT subtest recorded exactly one pass; no fail events, no data-race reports. Per the claim's own criterion, a skipped GET control alone is not a CONNECT test failure.

### 4. Full package suite (environment command)

```sh
cd /home/ubuntu/wt-c1 && go test -count=1 -timeout=60s ./pkg/server/...
```

```console
ok  	github.com/traefik/traefik/v2/pkg/server	17.169s
ok  	github.com/traefik/traefik/v2/pkg/server/cookie	0.005s
ok  	github.com/traefik/traefik/v2/pkg/server/middleware	0.012s
...
ok  	github.com/traefik/traefik/v2/pkg/server/service/udp	0.005s
```

Exit code 0.

### 5. Side observation (outside the claim's scope): -count>1 port collision

```sh
cd /home/ubuntu/wt-c1 && go test -race -count=5 -timeout=300s ./pkg/server \
  -run '^(Test_denyConnect|TestConnectRejected|TestHTTP3ConnectRejected|TestEvalConnect_.*)$'
```

```console
        	            	error preparing server: error opening listener: listen tcp 127.0.0.1:8093: bind: address already in use
        	Test:       	TestHTTP3ConnectRejected
FAIL	github.com/traefik/traefik/v2/pkg/server	3.856s
```

`TestHTTP3ConnectRejected` binds the fixed address `127.0.0.1:8093`, so back-to-back iterations in one process collide. This is the pre-existing convention in that file — the baseline test fails the same way:

```sh
go test -race -count=3 -timeout=120s ./pkg/server -run '^TestHTTP3ReadTimeout$'
```

```console
--- FAIL: TestHTTP3ReadTimeout (0.00s)
        	            	error preparing http3 server: starting listener: listen udp 127.0.0.1:8091: bind: address already in use
```

The environment's commands all use `-count=1`, under which the test is reliably green (sections 1–3), so this does not bear on C1.

### Verdict reasoning

Under the race detector with the environment's own invocations (`-race -count=1`, the eval runner script, the per-test commands), all three solution CONNECT tests and all ten fixture CONNECT tests ran to completion and passed with zero data-race reports across 9 independent runs. The runner script's non-zero exit stems solely from the skipped `ordinary request` GET control subtest in the `no matching router` case, which the claim explicitly excludes. C1 is REFUTED.
