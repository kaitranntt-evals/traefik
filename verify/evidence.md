# Evidence for verify run v-6f57b961

Environment: `go version go1.26.3 linux/amd64`. Audited branch `origin/traefik-gateway-listener-isolation-perfect` at `8ffb781f37856b386d52e48ae05199e2cf5d4871`. Production code is untouched on this branch; everything added lives under `verify/`.

Suite on the audited branch with the eval fixtures (`eval_https_test.go`, `eval_isolation_test.go`, `eval_tlsconflict_test.go`, `export_eval_test.go`) copied byte-exact into `pkg/provider/kubernetes/gateway/` for the run and removed afterwards:

```bash
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/...
go vet ./pkg/provider/kubernetes/gateway/...
go test -race -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway
```

```text
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	7.592s
vet-exit=0
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	15.117s
```

The eval fixtures pass on the audited branch; none of them places a TLS passthrough listener under an HTTPS wildcard listener.

## C1

Claim: the listener-isolation tests leave omission of the generated `catchall-hosts` service undetected. Target branch: [`evalon/traefik-ga-db4a8424`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-db4a8424) at `7d0a941b9c4fc25edb7ca38cbb70a0dcb772fcdb`, in a separate worktree.

Fixture: `TestGatewayListenerIsolation` in `pkg/provider/kubernetes/gateway/listener_isolation_test.go`. It attaches a route without hostnames to each listener (route `catchall` among them) and adds:

```go
objects = append(objects, isolationRoute(kind, "catchall", []gatev1.Hostname{"foo.example.com", "elsewhere.test"}))
```

`isolationRoute` names a route with hostnames `<section>-hosts`, so this is the route `catchall-hosts`. The assertions that touch the catch-all listener positively are:

```go
{"elsewhere.test", "/catchall/Call", http.StatusOK},
{"example.com", "/catchall/Call", http.StatusOK},
```

Instrumentation (test helper only, `verify/repro/c1_drop_service.patch`): `LoadListenerConfigurationForTest` prints the generated `catchall*` routers and services when `VERIFY_DUMP` is set, and deletes the generated HTTP services whose key matches the regular expression in `VERIFY_DROP`, leaving everything else as generated.

Generated records:

```text
$ VERIFY_DUMP=1 go test -count=1 -v -run "^TestGatewayListenerIsolation$/^HTTP$/^HTTPRoute$" ./pkg/provider/kubernetes/gateway/
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute router httproute-default-catchall-gw-default-isolation-ep-web-0-bcf39167ea4be843379c rule=!Host("**.example.com") && !Host("**.foo.example.com") && !Host("empty.example.com") && !Host("foo.example.com") && (Path("/catchall") || PathPrefix("/catchall/")) service=httproute-default-catchall-gw-default-isolation-ep-web-0-bcf39167ea4be843379c-wrr prio=10901
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute router httproute-default-catchall-hosts-gw-default-isolation-ep-web-0-533f51ac951c1342d0ff rule=Host("elsewhere.test") && !Host("**.example.com") && !Host("**.foo.example.com") && !Host("empty.example.com") && !Host("foo.example.com") && (Path("/catchall") || PathPrefix("/catchall/")) service=httproute-default-catchall-hosts-gw-default-isolation-ep-web-0-533f51ac951c1342d0ff-wrr prio=10915
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute service-key httproute-default-catchall-gw-default-isolation-ep-web-0-bcf39167ea4be843379c-svc-default-whoami-0
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute service-key httproute-default-catchall-gw-default-isolation-ep-web-0-bcf39167ea4be843379c-wrr
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute service-key httproute-default-catchall-hosts-gw-default-isolation-ep-web-0-533f51ac951c1342d0ff-svc-default-whoami-0
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute service-key httproute-default-catchall-hosts-gw-default-isolation-ep-web-0-533f51ac951c1342d0ff-wrr
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.122s
```

The `catchall-hosts` router (`Host("elsewhere.test") && ...`) and the `catchall` router both match `elsewhere.test` on `/catchall/Call`, and both point at the same backend, so the test cannot tell them apart.

Unchanged run, then single-family omission, then control omission of the `catchall` route services, whole package:

```text
$ go test -count=1 ./pkg/provider/kubernetes/gateway/... (baseline)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.536s
$ VERIFY_DROP=catchall-hosts go test -count=1 ./pkg/provider/kubernetes/gateway/...
            	            	actual  : 421
            	Test:       	TestGatewayHTTPSListenerTLSOptionsConflict/GRPCRoute/catchall
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.615s
FAIL
$ VERIFY_DROP=catchall-hosts go test -count=1 ./pkg/provider/kubernetes/gateway/... 2>&1 | grep -c '^VERIFY.*dropped'
12
$ VERIFY_DROP='httproute-default-catchall-gw|grpcroute-default-catchall-gw' go test -count=1 ./pkg/provider/kubernetes/gateway/... (control)
--- FAIL: TestGatewayListenerIsolation (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.612s
FAIL
```

Focused on the fixture, with only the `catchall-hosts` services removed:

```text
$ VERIFY_DROP=catchall-hosts go test -count=1 -v -run "^TestGatewayListenerIsolation$" ./pkg/provider/kubernetes/gateway/
VERIFY TestGatewayListenerIsolation/HTTP/GRPCRoute dropped service grpcroute-default-catchall-hosts-gw-default-isolation-ep-web-0-0b869afafbb43a7767be-svc-default-whoami-0
VERIFY TestGatewayListenerIsolation/HTTP/GRPCRoute dropped service grpcroute-default-catchall-hosts-gw-default-isolation-ep-web-0-0b869afafbb43a7767be-wrr
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute dropped service httproute-default-catchall-hosts-gw-default-isolation-ep-web-0-533f51ac951c1342d0ff-svc-default-whoami-0
VERIFY TestGatewayListenerIsolation/HTTP/HTTPRoute dropped service httproute-default-catchall-hosts-gw-default-isolation-ep-web-0-533f51ac951c1342d0ff-wrr
VERIFY TestGatewayListenerIsolation/HTTPS/GRPCRoute dropped service grpcroute-default-catchall-hosts-gw-default-isolation-ep-websecure-0-5dbe3fe49ba9c34d3a56-wrr
VERIFY TestGatewayListenerIsolation/HTTPS/GRPCRoute dropped service grpcroute-default-catchall-hosts-gw-default-isolation-ep-websecure-0-5dbe3fe49ba9c34d3a56-svc-default-whoami-0
VERIFY TestGatewayListenerIsolation/HTTPS/HTTPRoute dropped service httproute-default-catchall-hosts-gw-default-isolation-ep-websecure-0-2ad2f8de996d1892c29c-wrr
VERIFY TestGatewayListenerIsolation/HTTPS/HTTPRoute dropped service httproute-default-catchall-hosts-gw-default-isolation-ep-websecure-0-2ad2f8de996d1892c29c-svc-default-whoami-0
=== RUN   TestGatewayListenerIsolation/HTTP/GRPCRoute/elsewhere.test/catchall/Call
=== RUN   TestGatewayListenerIsolation/HTTPS/GRPCRoute/elsewhere.test/catchall/Call
=== RUN   TestGatewayListenerIsolation/HTTP/HTTPRoute/elsewhere.test/catchall/Call
=== RUN   TestGatewayListenerIsolation/HTTPS/HTTPRoute/elsewhere.test/catchall/Call
--- PASS: TestGatewayListenerIsolation (0.00s)
        --- PASS: TestGatewayListenerIsolation/HTTP/GRPCRoute/elsewhere.test/catchall/Call (0.00s)
        --- PASS: TestGatewayListenerIsolation/HTTPS/GRPCRoute/elsewhere.test/catchall/Call (0.00s)
        --- PASS: TestGatewayListenerIsolation/HTTP/HTTPRoute/elsewhere.test/catchall/Call (0.00s)
        --- PASS: TestGatewayListenerIsolation/HTTPS/HTTPRoute/elsewhere.test/catchall/Call (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.174s
```

Which tests fail in the whole package with only `catchall-hosts` removed (`VERIFY_DROP=catchall-hosts go test -count=1 -v ./pkg/provider/kubernetes/gateway/`):

```text
--- FAIL: TestGatewayHTTPSListenerTLSOptionsConflict (0.00s)
    --- FAIL: TestGatewayHTTPSListenerTLSOptionsConflict/GRPCRoute/catchall (0.04s)
    --- FAIL: TestGatewayHTTPSListenerTLSOptionsConflict/HTTPRoute/catchall (0.06s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.616s
FAIL
```

Observations:

- `TestGatewayListenerIsolation` (HTTP and HTTPS, HTTPRoute and GRPCRoute) passes with all eight `catchall-hosts` services removed: every assertion stays satisfied, because the `catchall` route answers `elsewhere.test` with the same 200.
- Control: removing the `catchall` route services instead makes `TestGatewayListenerIsolation` fail, so the mutation mechanism does bite.
- A different test in the same file, `TestGatewayHTTPSListenerTLSOptionsConflict/{HTTPRoute,GRPCRoute}/catchall`, where `catchall-hosts` is the only route, fails (421) under the omission. It covers HTTPS only; the four `ep-web` (plain HTTP) `catchall-hosts` records are produced only by `TestGatewayListenerIsolation`, so nothing in the package detects their omission.

Impact reasoning: this is a test-strength gap, not a product defect. A regression that stops generating the service of a hostname-restricted route on a catch-all listener would go unnoticed by the isolation fixture and, for plain HTTP, by the whole package; only the HTTPS cross-provider conflict test would catch it.

## C2

Claim: the cycle subtest in `Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts` bypasses the recursive parent traversal. Branch under test: `origin/traefik-gateway-listener-isolation-perfect` at `8ffb781f37856b386d52e48ae05199e2cf5d4871`.

Configuration (`pkg/server/aggregator_test.go`, subtest `cyclic parent refs terminate safely`):

```go
routers := map[string]*dynamic.Router{
	"a@file": {Rule: "Method(`GET`)", ParentRefs: []string{"b@file"}},
	"b@file": {Rule: "Host(`foo.example.com`)", ParentRefs: []string{"a@file"}, Service: "ext@file"},
}
got := resolveHTTPTLSOptions(routers, false)
require.Contains(t, got, "a@file")
require.Contains(t, got, "b@file")
```

Neither router has `TLS` or `EntryPoints`. `resolveHTTPTLSOptions` (`pkg/server/aggregator.go`) only adds a router to `routersByEntryPoint` when `router.TLS != nil`, for each of its entry points, and calls `resolveParentChildDomains` (which defines the recursive `rootsOf`) only inside `for ep, epRouters := range routersByEntryPoint`.

Traversal, command:

```bash
bash verify/repro/c2_cycle_coverage.sh
```

Output:

```text
## 1. cycle subtest, function coverage
=== RUN   Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts
=== RUN   Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts/cyclic_parent_refs_terminate_safely
--- PASS: Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts (0.00s)
    --- PASS: Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts/cyclic_parent_refs_terminate_safely (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.028s	coverage: 3.0% of statements
github.com/traefik/traefik/v3/pkg/server/aggregator.go:193:				resolveHTTPTLSOptions			48.1%
github.com/traefik/traefik/v3/pkg/server/aggregator.go:305:				resolveParentChildDomains		0.0%
github.com/traefik/traefik/v3/pkg/server/aggregator.go:433:				findConflictingRouters			0.0%
## 1b. statement blocks of resolveParentChildDomains/rootsOf executed (last column = hit count)
resolveParentChildDomains spans aggregator.go lines 305-427
blocks in range: 61, blocks executed: 0
## 2. same subtest with the cycle guard (if seen[ref] { continue }) removed from rootsOf
 1 file changed, 3 deletions(-)
=== RUN   Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts
=== RUN   Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts/cyclic_parent_refs_terminate_safely
--- PASS: Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts (0.00s)
    --- PASS: Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts/cyclic_parent_refs_terminate_safely (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.026s
## 3. guard still removed: same cyclic routers plus one TLS router with an entry point
      1 FAIL
      1 FAIL	github.com/traefik/traefik/v3/pkg/server	1.293s
      1 fatal error: stack overflow
      1 github.com/traefik/traefik/v3/pkg/server.resolveParentChildDomains.func3({0x2eafb75a2e40?, 0x1, 0x2eafb70038c0?}, 0x2eafb76f7380)
     45 github.com/traefik/traefik/v3/pkg/server.resolveParentChildDomains.func3({0x2eafb75a2e40?, 0x1, 0x5a553a0?}, 0x2eafb76f7380)
     46 github.com/traefik/traefik/v3/pkg/server.resolveParentChildDomains.func3({0x2eafb75a2e60?, 0x1, 0x5a553a6?}, 0x2eafb76f7380)
      1 runtime: goroutine stack exceeds 1000000000-byte limit
## 4. guard restored: companion test, function coverage
=== RUN   TestVerifyC2_CycleReachesTraversal
--- PASS: TestVerifyC2_CycleReachesTraversal (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.045s	coverage: 10.4% of statements
github.com/traefik/traefik/v3/pkg/server/aggregator.go:305:				resolveParentChildDomains		53.2%
```

Observations:

- Step 1: the subtest passes with `resolveParentChildDomains` at 0.0% and none of its 61 statement blocks (the `rootsOf` closure included) executed.
- Step 2: with the cycle guard `if seen[ref] { continue }` deleted from `rootsOf`, the subtest still passes, so it does not check that cyclic parent refs terminate.
- Step 3: with the guard still deleted, the same two cyclic routers plus one TLS router on an entry point (`verify/repro/verify_c2_cycle_test.go`) overflow the stack inside `resolveParentChildDomains.func3`.
- Step 4: with the guard restored, that companion test passes and covers `resolveParentChildDomains`.

Impact reasoning: test-strength gap. The subtest is named for cycle safety but would stay green if the cycle guard were removed, while a configuration with a parent-ref cycle and any TLS router on an entry point would then crash the process with a stack overflow.

## C3

Claim: a wildcard HTTPS listener intercepts TLS connections intended for an accepted exact-host TLS passthrough route on the same entry point.

Branch under test: `origin/traefik-gateway-listener-isolation-perfect` at `8ffb781f37856b386d52e48ae05199e2cf5d4871` (the base of this verify branch). Control: the base commit `35c7530ac0906b5407a3733de173d58d9c427949` (before the listener-isolation change), in a separate worktree.

Setup shared by the Go repros: copy `verify/repro/verify_export_test.go`, `verify/repro/verify_harness_test.go`, `verify/repro/verify_passthrough_test.go`, `verify/repro/verify_identity_test.go` and `verify/repro/verify_collision_test.go` into `pkg/provider/kubernetes/gateway/` of the checkout under test. They add test files only. The harness runs the provider on fake Kubernetes clients, feeds its configuration through the production `ConfigurationWatcher` (which runs `resolveHTTPTLSOptions`), the TLS manager and `NewRouterFactory(...).CreateRouters`, then either dials a real TLS connection served by the production `Router.ServeTCP`, or selects the TLS options for an SNI with the production matcher and serves the request through the production HTTPS handler (`SNICheck` included). HTTP backends are replaced by a stub answering `backend:<address>`; the entry point HTTPS server is replaced by a stub answering `HTTPS-TERMINATED-BY-TRAEFIK` with the Gateway listener certificate (CN `gateway-listener-cert`); the passthrough backend is a real TLS server with CN `passthrough-backend` answering `PASSTHROUGH-BACKEND`.

Topology: one Gateway on entry point `websecure` with an HTTPS listener `web` for `*.example.com` (HTTPRoute `web` attached) and a TLS listener `db` for `db.example.com` in `Passthrough` mode (TLSRoute `db` for `db.example.com` attached). The first subtest is the same Gateway without the HTTPS listener.

Command, on the branch under test:

```bash
go test -race -count=1 -v -run '^TestVerifyPassthroughBesideHTTPSWildcard$' ./pkg/provider/kubernetes/gateway
```

Output (JSON log lines and assertion diffs removed, long lines cut):

```text
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone
    verify_passthrough_test.go:171: STATUS  TLSRoute db parent=gw/db Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  listener gw/db attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:178: TCP router deny-unknown-host@kubernetesgateway rule=HostSNI(`*`) && !ALPN(`h2`) && !ALPN(`http/1.1`) entryPoints=[websecure] passthrough=false service=deny-unknown-host
    verify_passthrough_test.go:178: TCP router tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e@kubernetesgateway rule=HostSNI("db.example.com") entryPoints=[websecure] passthrough=true service=tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e-wrr
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener
    verify_passthrough_test.go:171: STATUS  HTTPRoute web parent=gw/web Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  TLSRoute db parent=gw/db Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  listener gw/db attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  listener gw/web attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:178: HTTPS router listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") entryPoints=[websecure] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolvedOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"
    verify_passthrough_test.go:178: TCP router deny-unknown-host@kubernetesgateway rule=HostSNI(`*`) && !ALPN(`h2`) && !ALPN(`http/1.1`) entryPoints=[websecure] passthrough=false service=deny-unknown-host
    verify_passthrough_test.go:178: TCP router tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e@kubernetesgateway rule=HostSNI("db.example.com") entryPoints=[websecure] passthrough=true service=tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e-wrr
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="gateway-listener-cert" payload="HTTPS-TERMINATED-BY-TRAEFIK" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="gateway-listener-cert" payload="HTTPS-TERMINATED-BY-TRAEFIK" err=<nil>
--- FAIL: TestVerifyPassthroughBesideHTTPSWildcard (0.77s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone (0.63s)
    --- FAIL: TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener (0.14s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.870s
FAIL
```

Matcher generation: the provider reports the TLSRoute `Accepted=True` and both listeners `Programmed=True`, and generates the HTTPS router `listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb` with rule `Host("**.example.com")`, which is not a catch-all and matches `db.example.com`, next to the TCP router `HostSNI("db.example.com")` with `passthrough=true`.

Dispatch: a real TLS connection with SNI `db.example.com` through `Router.ServeTCP` is answered with the Gateway listener certificate and the HTTPS marker, not by the passthrough backend. In the control subtest (no HTTPS listener) the same connection reaches the passthrough backend. `pkg/server/router/tcp/router.go` `ServeTCP` serves a non-catch-all `muxerHTTPS` match before it consults `muxerTCPTLS`.

Same command on the base commit `35c7530ac` (before the change):

```text
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener
    verify_passthrough_test.go:178: HTTPS router httproute-default-web-gw-default-gw-ep-websecure-0-09f5a4bbb1754f4e02b9@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathPrefix("/") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
--- PASS: TestVerifyPassthroughBesideHTTPSWildcard (0.70s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone (0.62s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.811s
```

There the HTTPS route for the wildcard listener uses `HostRegexp`, which registers no SNI in the HTTPS muxer, and `db.example.com` reaches the passthrough backend. The interception is therefore introduced by the audited change.

Impact reasoning: a Gateway that serves a TLS passthrough hostname (for example a database) and an HTTPS wildcard listener covering it on the same port is a valid, accepted Gateway API topology; both listeners and both routes report Accepted/Programmed. After the change, every TLS connection for the passthrough hostname is terminated by Traefik with the wildcard listener certificate and handed to the HTTP stack, so the passthrough backend is unreachable and clients see the wrong certificate, with no status condition signalling it. Workaround observed: none within the same entry point; the control shows it only works when the HTTPS wildcard listener is absent.

## C4

Claim: a TLS connection with SNI `db.example.com` is intercepted by an HTTPS wildcard listener instead of reaching an accepted `db.example.com` TLS passthrough route on the same entry point.

Branch under test: `origin/traefik-gateway-listener-isolation-perfect` at `8ffb781f37856b386d52e48ae05199e2cf5d4871` (the base of this verify branch). Control: the base commit `35c7530ac0906b5407a3733de173d58d9c427949` (before the listener-isolation change), in a separate worktree.

Setup shared by the Go repros: copy `verify/repro/verify_export_test.go`, `verify/repro/verify_harness_test.go`, `verify/repro/verify_passthrough_test.go`, `verify/repro/verify_identity_test.go` and `verify/repro/verify_collision_test.go` into `pkg/provider/kubernetes/gateway/` of the checkout under test. They add test files only. The harness runs the provider on fake Kubernetes clients, feeds its configuration through the production `ConfigurationWatcher` (which runs `resolveHTTPTLSOptions`), the TLS manager and `NewRouterFactory(...).CreateRouters`, then either dials a real TLS connection served by the production `Router.ServeTCP`, or selects the TLS options for an SNI with the production matcher and serves the request through the production HTTPS handler (`SNICheck` included). HTTP backends are replaced by a stub answering `backend:<address>`; the entry point HTTPS server is replaced by a stub answering `HTTPS-TERMINATED-BY-TRAEFIK` with the Gateway listener certificate (CN `gateway-listener-cert`); the passthrough backend is a real TLS server with CN `passthrough-backend` answering `PASSTHROUGH-BACKEND`.

Topology: one Gateway on entry point `websecure` with an HTTPS listener `web` for `*.example.com` (HTTPRoute `web` attached) and a TLS listener `db` for `db.example.com` in `Passthrough` mode (TLSRoute `db` for `db.example.com` attached). The first subtest is the same Gateway without the HTTPS listener.

Command, on the branch under test:

```bash
go test -race -count=1 -v -run '^TestVerifyPassthroughBesideHTTPSWildcard$' ./pkg/provider/kubernetes/gateway
```

Output (JSON log lines and assertion diffs removed, long lines cut):

```text
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone
    verify_passthrough_test.go:171: STATUS  TLSRoute db parent=gw/db Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  listener gw/db attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:178: TCP router deny-unknown-host@kubernetesgateway rule=HostSNI(`*`) && !ALPN(`h2`) && !ALPN(`http/1.1`) entryPoints=[websecure] passthrough=false service=deny-unknown-host
    verify_passthrough_test.go:178: TCP router tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e@kubernetesgateway rule=HostSNI("db.example.com") entryPoints=[websecure] passthrough=true service=tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e-wrr
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener
    verify_passthrough_test.go:171: STATUS  HTTPRoute web parent=gw/web Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  TLSRoute db parent=gw/db Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  listener gw/db attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:171: STATUS  listener gw/web attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_passthrough_test.go:178: HTTPS router listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") entryPoints=[websecure] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolvedOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"
    verify_passthrough_test.go:178: TCP router deny-unknown-host@kubernetesgateway rule=HostSNI(`*`) && !ALPN(`h2`) && !ALPN(`http/1.1`) entryPoints=[websecure] passthrough=false service=deny-unknown-host
    verify_passthrough_test.go:178: TCP router tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e@kubernetesgateway rule=HostSNI("db.example.com") entryPoints=[websecure] passthrough=true service=tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e-wrr
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="gateway-listener-cert" payload="HTTPS-TERMINATED-BY-TRAEFIK" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="gateway-listener-cert" payload="HTTPS-TERMINATED-BY-TRAEFIK" err=<nil>
--- FAIL: TestVerifyPassthroughBesideHTTPSWildcard (0.77s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone (0.63s)
    --- FAIL: TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener (0.14s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.870s
FAIL
```

Matcher generation: the provider reports the TLSRoute `Accepted=True` and both listeners `Programmed=True`, and generates the HTTPS router `listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb` with rule `Host("**.example.com")`, which is not a catch-all and matches `db.example.com`, next to the TCP router `HostSNI("db.example.com")` with `passthrough=true`.

Dispatch: a real TLS connection with SNI `db.example.com` through `Router.ServeTCP` is answered with the Gateway listener certificate and the HTTPS marker, not by the passthrough backend. In the control subtest (no HTTPS listener) the same connection reaches the passthrough backend. `pkg/server/router/tcp/router.go` `ServeTCP` serves a non-catch-all `muxerHTTPS` match before it consults `muxerTCPTLS`.

Same command on the base commit `35c7530ac` (before the change):

```text
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener
    verify_passthrough_test.go:178: HTTPS router httproute-default-web-gw-default-gw-ep-websecure-0-09f5a4bbb1754f4e02b9@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathPrefix("/") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
--- PASS: TestVerifyPassthroughBesideHTTPSWildcard (0.70s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone (0.62s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.811s
```

There the HTTPS route for the wildcard listener uses `HostRegexp`, which registers no SNI in the HTTPS muxer, and `db.example.com` reaches the passthrough backend. The interception is therefore introduced by the audited change.

Impact reasoning: a Gateway that serves a TLS passthrough hostname (for example a database) and an HTTPS wildcard listener covering it on the same port is a valid, accepted Gateway API topology; both listeners and both routes report Accepted/Programmed. After the change, every TLS connection for the passthrough hostname is terminated by Traefik with the wildcard listener certificate and handed to the HTTP stack, so the passthrough backend is unreachable and clients see the wrong certificate, with no status condition signalling it. Workaround observed: none within the same entry point; the control shows it only works when the HTTPS wildcard listener is absent.

## C5

Claim: wildcard and catch-all HTTPS listeners lose crossed-request isolation when an accepted overlapping TLS passthrough listener is present. Target branch: [`evalon/traefik-ga-31f858e0`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-31f858e0) at `6ea12c0da3f58eb47e7fce5314ddc5ea87c7233c`, in a separate worktree.

Setup shared by the Go repros: copy `verify/repro/verify_export_test.go`, `verify/repro/verify_harness_test.go`, `verify/repro/verify_passthrough_test.go`, `verify/repro/verify_identity_test.go` and `verify/repro/verify_collision_test.go` into `pkg/provider/kubernetes/gateway/` of the checkout under test. They add test files only. The harness runs the provider on fake Kubernetes clients, feeds its configuration through the production `ConfigurationWatcher` (which runs `resolveHTTPTLSOptions`), the TLS manager and `NewRouterFactory(...).CreateRouters`, then either dials a real TLS connection served by the production `Router.ServeTCP`, or selects the TLS options for an SNI with the production matcher and serves the request through the production HTTPS handler (`SNICheck` included). HTTP backends are replaced by a stub answering `backend:<address>`; the entry point HTTPS server is replaced by a stub answering `HTTPS-TERMINATED-BY-TRAEFIK` with the Gateway listener certificate (CN `gateway-listener-cert`); the passthrough backend is a real TLS server with CN `passthrough-backend` answering `PASSTHROUGH-BACKEND`.

Topology per subtest: one Gateway on `websecure` with two HTTPS listeners named `broad` and `exact` (the names are only labels), each with its own HTTPRoute and backend (`10.10.0.1` for `broad`, `10.10.0.2` for `exact`), with or without a TLS `Passthrough` listener and TLSRoute. The claim's topology is the subtest `catch-all vs wildcard`: `broad` has no hostname, `exact` is `*.example.com`, the TLS listener is `db.example.com`.

Command:

```bash
go test -race -count=1 -v -run '^TestVerifyHTTPSIdentityBesidePassthrough$' ./pkg/provider/kubernetes/gateway
```

Output of the claim's topology, control (no TLS listener) then with the TLS passthrough listener:

```text
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_wildcard,_control_without_TLS_listener
    verify_identity_test.go:49: STATUS  HTTPRoute broad parent=gw/broad Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  HTTPRoute exact parent=gw/exact Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  listener gw/broad attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  listener gw/exact attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:56: HTTPS router gateway-default-gw-listener-broad-794f0218cf6fb17e75d4-unmatched@kubernetesgateway rule=PathPrefix("/") && !Host("**.example.com") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_identity_test.go:56: HTTPS router gateway-default-gw-listener-exact-48f206592a5e3e2c664a-unmatched@kubernetesgateway rule=Host("**.example.com") && PathPrefix("/") entryPoints=[websecure] tls.options="gateway-default-gw-listener-exact-48f206592a5e3e2c664a" resolvedOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway"
    verify_identity_test.go:56: HTTPS router httproute-default-broad-gw-default-gw-ep-websecure-0-a3e31c1f311026b3ebe0@kubernetesgateway rule=PathPrefix("/") && !Host("**.example.com") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_identity_test.go:56: HTTPS router httproute-default-exact-gw-default-gw-ep-websecure-0-ccb7523265a2a4dc48e2@kubernetesgateway rule=Host("**.example.com") && PathPrefix("/") entryPoints=[websecure] tls.options="gateway-default-gw-listener-exact-48f206592a5e3e2c664a" resolvedOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway"
    verify_identity_test.go:68: REQUEST sni=outside.test host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=a.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=outside.test host=a.example.com -> 421 body="Misdirected Request\n" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=outside.test -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_wildcard,_with_overlapping_TLS_passthrough_listener
    verify_identity_test.go:49: STATUS  HTTPRoute broad parent=gw/broad Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  HTTPRoute exact parent=gw/exact Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  TLSRoute db parent=gw/db Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  listener gw/broad attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  listener gw/db attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:49: STATUS  listener gw/exact attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_identity_test.go:56: HTTPS router gateway-default-gw-listener-broad-794f0218cf6fb17e75d4-unmatched@kubernetesgateway rule=PathPrefix("/") && !Host("**.example.com") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_identity_test.go:56: HTTPS router gateway-default-gw-listener-exact-48f206592a5e3e2c664a-unmatched@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathPrefix("/") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_identity_test.go:56: HTTPS router httproute-default-broad-gw-default-gw-ep-websecure-0-a3e31c1f311026b3ebe0@kubernetesgateway rule=PathPrefix("/") && !Host("**.example.com") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_identity_test.go:56: HTTPS router httproute-default-exact-gw-default-gw-ep-websecure-0-9f2e85667f74a3ada958@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathPrefix("/") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_identity_test.go:56: TCP router deny-unknown-host@kubernetesgateway rule=HostSNI(`*`) && !ALPN(`h2`) && !ALPN(`http/1.1`) entryPoints=[websecure] passthrough=false service=deny-unknown-host
    verify_identity_test.go:56: TCP router tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e@kubernetesgateway rule=HostSNI("db.example.com") entryPoints=[websecure] passthrough=true service=tlsroute-default-db-gw-default-gw-ep-websecure-0-9ad1c08d59d4f8c6369e-wrr
    verify_identity_test.go:68: REQUEST sni=outside.test host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=a.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=outside.test host=a.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 421)
```

All requests and results:

```text
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_exact,_control_without_TLS_listener
    verify_identity_test.go:68: REQUEST sni=app.example.com host=app.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="gateway-default-gw-listener-broad-794f0218cf6fb17e75d4@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=foo.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=app.example.com host=foo.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-broad-794f0218cf6fb17e75d4@kubernetesgateway" (want 421)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=app.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_exact,_with_overlapping_TLS_passthrough_listener
    verify_identity_test.go:68: REQUEST sni=app.example.com host=app.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=foo.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=app.example.com host=foo.example.com -> 421 body="Misdirected Request\n" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=app.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_exact,_control_without_TLS_listener
    verify_identity_test.go:68: REQUEST sni=outside.test host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=foo.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=outside.test host=foo.example.com -> 421 body="Misdirected Request\n" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=outside.test -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_exact,_with_overlapping_TLS_passthrough_listener
    verify_identity_test.go:68: REQUEST sni=outside.test host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=foo.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=outside.test host=foo.example.com -> 421 body="Misdirected Request\n" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=foo.example.com host=outside.test -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_wildcard,_control_without_TLS_listener
    verify_identity_test.go:68: REQUEST sni=outside.test host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=a.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=outside.test host=a.example.com -> 421 body="Misdirected Request\n" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=outside.test -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_wildcard,_with_overlapping_TLS_passthrough_listener
    verify_identity_test.go:68: REQUEST sni=outside.test host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=a.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=outside.test host=a.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=a.example.com host=outside.test -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_nested_wildcard,_control_without_TLS_listener
    verify_identity_test.go:68: REQUEST sni=app.example.com host=app.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="gateway-default-gw-listener-broad-794f0218cf6fb17e75d4@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=app.sub.example.com host=app.sub.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 200)
    verify_identity_test.go:68: REQUEST sni=app.example.com host=app.sub.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-broad-794f0218cf6fb17e75d4@kubernetesgateway" (want 421)
    verify_identity_test.go:68: REQUEST sni=app.sub.example.com host=app.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-default-gw-listener-exact-48f206592a5e3e2c664a@kubernetesgateway" (want 421)
=== RUN   TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_nested_wildcard,_with_overlapping_TLS_passthrough_listener
    verify_identity_test.go:68: REQUEST sni=app.example.com host=app.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=app.sub.example.com host=app.sub.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="default" (want 200)
    verify_identity_test.go:68: REQUEST sni=app.example.com host=app.sub.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="default" (want 421)
    verify_identity_test.go:68: REQUEST sni=app.sub.example.com host=app.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="default" (want 421)
--- FAIL: TestVerifyHTTPSIdentityBesidePassthrough (1.33s)
    --- PASS: TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_exact,_control_without_TLS_listener (0.66s)
    --- PASS: TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_exact,_with_overlapping_TLS_passthrough_listener (0.14s)
    --- PASS: TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_exact,_control_without_TLS_listener (0.03s)
    --- PASS: TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_exact,_with_overlapping_TLS_passthrough_listener (0.13s)
    --- PASS: TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_wildcard,_control_without_TLS_listener (0.11s)
    --- FAIL: TestVerifyHTTPSIdentityBesidePassthrough/catch-all_vs_wildcard,_with_overlapping_TLS_passthrough_listener (0.13s)
    --- PASS: TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_nested_wildcard,_control_without_TLS_listener (0.06s)
    --- FAIL: TestVerifyHTTPSIdentityBesidePassthrough/wildcard_vs_nested_wildcard,_with_overlapping_TLS_passthrough_listener (0.07s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.423s
FAIL
```

Identity separation: without the TLS listener the wildcard listener routers carry their own TLS options (`gateway-default-gw-listener-exact-...`) and the catch-all uses `default`. With the accepted TLS passthrough listener for `db.example.com` (TLSRoute `Accepted=True`, listener `Programmed=True`) the wildcard routers are generated with `tls.options=""`, resolved to `default`, the same identity as the catch-all. `isolation.go` `canHaveTLSOptions` returns false for a wildcard hostname that includes the hostname of an attached TLS listener on the same entry point.

Request handling: ordinary requests reach the intended backends (`outside.test` -> `10.10.0.1`, `a.example.com` -> `10.10.0.2`). Crossed requests SNI `outside.test` + Host `a.example.com` and SNI `a.example.com` + Host `outside.test` return 200 from a backend with the TLS listener present, and 421 in the control. The same happens between two wildcard listeners (`*.example.com` and `*.sub.example.com`) when the TLS listener is `db.sub.example.com`. A wildcard or catch-all against an exact-host HTTPS listener still returns 421, because the exact listener keeps its own identity.

Passthrough on this branch (`go test -race -count=1 -v -run '^TestVerifyPassthroughBesideHTTPSWildcard$' ./pkg/provider/kubernetes/gateway`):

```text
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
=== RUN   TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener
    verify_passthrough_test.go:178: HTTPS router httproute-default-web-gw-default-gw-ep-websecure-0-09f5a4bbb1754f4e02b9@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathPrefix("/") entryPoints=[websecure] tls.options="" resolvedOptions="default"
    verify_passthrough_test.go:182: DISPATCH sni=db.example.com -> peerCertCN="passthrough-backend" payload="PASSTHROUGH-BACKEND" err=<nil>
    verify_passthrough_test.go:182: DISPATCH sni=app.example.com -> peerCertCN="" payload="" err=EOF
--- PASS: TestVerifyPassthroughBesideHTTPSWildcard (0.70s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/control:_TLS_passthrough_listener_alone (0.60s)
    --- PASS: TestVerifyPassthroughBesideHTTPSWildcard/HTTPS_wildcard_listener_beside_the_TLS_passthrough_listener (0.09s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.792s
```

The branch keeps the passthrough route reachable, and pays for it with the lost identity.

Impact reasoning: the trigger is one accepted TLS passthrough listener whose hostname falls under an HTTPS wildcard listener, on a Gateway that also has a catch-all (or a second wildcard) HTTPS listener. From then on a client that opened a connection for one listener can send requests for the other on it and gets its backend instead of 421, which is what `GatewayHTTPSListenerDetectMisdirectedRequests`, advertised by the branch, says cannot happen. No status condition reports the downgrade. Workaround observed: none short of removing the overlapping TLS listener.

## C6

Claim: the distinct HTTPS hostnames `foo-bar.example.com` and `foo.bar.example.com` collide in listener identity, allowing crossed requests to reach a backend instead of returning HTTP 421. Target branch: [`evalon/traefik-ga-528cc418`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-528cc418) at `8c04550daa05a19db5e59cb5f466faa4169bd56b`, in a separate worktree.

Name drift: the claim points at `makeListenerRouterName`, `buildListenerRouters` and `applyListenerIsolation`; the behaviour was adjudicated through the generated configuration and the served requests rather than through those symbols.

Setup shared by the Go repros: copy `verify/repro/verify_export_test.go`, `verify/repro/verify_harness_test.go`, `verify/repro/verify_passthrough_test.go`, `verify/repro/verify_identity_test.go` and `verify/repro/verify_collision_test.go` into `pkg/provider/kubernetes/gateway/` of the checkout under test. They add test files only. The harness runs the provider on fake Kubernetes clients, feeds its configuration through the production `ConfigurationWatcher` (which runs `resolveHTTPTLSOptions`), the TLS manager and `NewRouterFactory(...).CreateRouters`, then either dials a real TLS connection served by the production `Router.ServeTCP`, or selects the TLS options for an SNI with the production matcher and serves the request through the production HTTPS handler (`SNICheck` included). HTTP backends are replaced by a stub answering `backend:<address>`; the entry point HTTPS server is replaced by a stub answering `HTTPS-TERMINATED-BY-TRAEFIK` with the Gateway listener certificate (CN `gateway-listener-cert`); the passthrough backend is a real TLS server with CN `passthrough-backend` answering `PASSTHROUGH-BACKEND`.

Topology: one Gateway on `websecure` with HTTPS listener `a` (HTTPRoute `a`, backend `10.10.0.1`) and HTTPS listener `b` (HTTPRoute `b`, backend `10.10.0.2`). Control subtest: `foo.example.com` and `bar.example.com`. Claim subtest: `foo-bar.example.com` and `foo.bar.example.com`.

Command:

```bash
go test -race -count=1 -v -run '^TestVerifyHyphenDotHostnameIdentity$' ./pkg/provider/kubernetes/gateway
```

Output:

```text
=== RUN   TestVerifyHyphenDotHostnameIdentity
=== RUN   TestVerifyHyphenDotHostnameIdentity/control:_hostnames_that_normalize_differently
    verify_collision_test.go:34: STATUS  HTTPRoute a parent=gw/a Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:34: STATUS  HTTPRoute b parent=gw/b Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:34: STATUS  listener gw/a attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:34: STATUS  listener gw/b attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:41: HTTPS router httproute-default-a-gw-default-gw-ep-websecure-0-54183f30141d1f21604f@kubernetesgateway rule=Host("foo.example.com") && PathPrefix("/") entryPoints=[websecure] tls.options="gateway-listener-foo-example-com" resolvedOptions="gateway-listener-foo-example-com@kubernetesgateway"
    verify_collision_test.go:41: HTTPS router httproute-default-b-gw-default-gw-ep-websecure-0-3eafee555bc8bd8bd793@kubernetesgateway rule=Host("bar.example.com") && PathPrefix("/") entryPoints=[websecure] tls.options="gateway-listener-bar-example-com" resolvedOptions="gateway-listener-bar-example-com@kubernetesgateway"
    verify_collision_test.go:43: TLS option  gateway-listener-bar-example-com@kubernetesgateway
    verify_collision_test.go:43: TLS option  gateway-listener-foo-example-com@kubernetesgateway
    verify_collision_test.go:43: TLS option  default
    verify_collision_test.go:56: REQUEST sni=foo.example.com host=foo.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="gateway-listener-foo-example-com@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=bar.example.com host=bar.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-listener-bar-example-com@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=foo.example.com host=bar.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-listener-foo-example-com@kubernetesgateway" (want 421)
    verify_collision_test.go:56: REQUEST sni=bar.example.com host=foo.example.com -> 421 body="Misdirected Request\n" tlsOptions="gateway-listener-bar-example-com@kubernetesgateway" (want 421)
=== RUN   TestVerifyHyphenDotHostnameIdentity/hostnames_differing_only_by_hyphen_and_dot
    verify_collision_test.go:34: STATUS  HTTPRoute a parent=gw/a Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:34: STATUS  HTTPRoute b parent=gw/b Accepted=True(Accepted) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:34: STATUS  listener gw/a attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:34: STATUS  listener gw/b attachedRoutes=1 Accepted=True(Accepted) Programmed=True(Programmed) ResolvedRefs=True(ResolvedRefs)
    verify_collision_test.go:41: HTTPS router httproute-default-a-gw-default-gw-ep-websecure-0-813347d4f0f94182e73f@kubernetesgateway rule=Host("foo-bar.example.com") && PathPrefix("/") entryPoints=[websecure] tls.options="gateway-listener-foo-bar-example-com" resolvedOptions="gateway-listener-foo-bar-example-com@kubernetesgateway"
    verify_collision_test.go:41: HTTPS router httproute-default-b-gw-default-gw-ep-websecure-0-6e47af6e25f7d3512f2b@kubernetesgateway rule=Host("foo.bar.example.com") && PathPrefix("/") entryPoints=[websecure] tls.options="gateway-listener-foo-bar-example-com" resolvedOptions="gateway-listener-foo-bar-example-com@kubernetesgateway"
    verify_collision_test.go:43: TLS option  gateway-listener-foo-bar-example-com@kubernetesgateway
    verify_collision_test.go:43: TLS option  default
    verify_collision_test.go:56: REQUEST sni=foo-bar.example.com host=foo-bar.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="gateway-listener-foo-bar-example-com@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=foo.bar.example.com host=foo.bar.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-listener-foo-bar-example-com@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=foo-bar.example.com host=foo.bar.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="gateway-listener-foo-bar-example-com@kubernetesgateway" (want 421)
    verify_collision_test.go:56: REQUEST sni=foo.bar.example.com host=foo-bar.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="gateway-listener-foo-bar-example-com@kubernetesgateway" (want 421)
--- FAIL: TestVerifyHyphenDotHostnameIdentity (0.62s)
    --- PASS: TestVerifyHyphenDotHostnameIdentity/control:_hostnames_that_normalize_differently (0.56s)
    --- FAIL: TestVerifyHyphenDotHostnameIdentity/hostnames_differing_only_by_hyphen_and_dot (0.06s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.714s
FAIL
```

Identity generation: in the control each listener gets its own TLS options (`gateway-listener-foo-example-com`, `gateway-listener-bar-example-com`). For the claim's hostnames both listeners' routers carry `tls.options="gateway-listener-foo-bar-example-com"` and the merged configuration holds a single such TLS option: the name is the hostname with dots normalized to hyphens, without a disambiguating suffix.

Request handling: ordinary requests reach the intended backends. Crossed requests SNI `foo-bar.example.com` + Host `foo.bar.example.com`, and the reverse, return 200 from the other listener's backend; the control pair returns 421 both ways.

Same test on `origin/traefik-gateway-listener-isolation-perfect` (`8ffb781f3`), for contrast, where the name carries a hash of the hostname:

```text
=== RUN   TestVerifyHyphenDotHostnameIdentity
=== RUN   TestVerifyHyphenDotHostnameIdentity/control:_hostnames_that_normalize_differently
    verify_collision_test.go:43: TLS option  listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway
    verify_collision_test.go:43: TLS option  listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway
    verify_collision_test.go:43: TLS option  default
    verify_collision_test.go:56: REQUEST sni=foo.example.com host=foo.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=bar.example.com host=bar.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=foo.example.com host=bar.example.com -> 421 body="Misdirected Request\n" tlsOptions="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" (want 421)
    verify_collision_test.go:56: REQUEST sni=bar.example.com host=foo.example.com -> 421 body="Misdirected Request\n" tlsOptions="listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway" (want 421)
=== RUN   TestVerifyHyphenDotHostnameIdentity/hostnames_differing_only_by_hyphen_and_dot
    verify_collision_test.go:43: TLS option  listener-websecure-https-foo-bar-example-com-e1367e1461794b2a8fdc@kubernetesgateway
    verify_collision_test.go:43: TLS option  default
    verify_collision_test.go:43: TLS option  listener-websecure-https-foo-bar-example-com-685363e81d0274f1342b@kubernetesgateway
    verify_collision_test.go:56: REQUEST sni=foo-bar.example.com host=foo-bar.example.com -> 200 body="backend:10.10.0.1:80" tlsOptions="listener-websecure-https-foo-bar-example-com-e1367e1461794b2a8fdc@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=foo.bar.example.com host=foo.bar.example.com -> 200 body="backend:10.10.0.2:80" tlsOptions="listener-websecure-https-foo-bar-example-com-685363e81d0274f1342b@kubernetesgateway" (want 200)
    verify_collision_test.go:56: REQUEST sni=foo-bar.example.com host=foo.bar.example.com -> 421 body="Misdirected Request\n" tlsOptions="listener-websecure-https-foo-bar-example-com-e1367e1461794b2a8fdc@kubernetesgateway" (want 421)
    verify_collision_test.go:56: REQUEST sni=foo.bar.example.com host=foo-bar.example.com -> 421 body="Misdirected Request\n" tlsOptions="listener-websecure-https-foo-bar-example-com-685363e81d0274f1342b@kubernetesgateway" (want 421)
--- PASS: TestVerifyHyphenDotHostnameIdentity (0.66s)
    --- PASS: TestVerifyHyphenDotHostnameIdentity/control:_hostnames_that_normalize_differently (0.56s)
    --- PASS: TestVerifyHyphenDotHostnameIdentity/hostnames_differing_only_by_hyphen_and_dot (0.10s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.751s
```

Impact reasoning: two HTTPS listeners whose hostnames differ only by `-` versus `.` at the same position are legal, distinct DNS names, though an uncommon pairing. On this branch they are treated as one listener for misdirected-request detection: a connection established for one hostname is served the other listener's routes instead of 421, silently. Workaround observed: none; the control shows only differently-normalizing hostnames are separated.
