## Setup

Audited branch: `traefik-gateway-listener-isolation-perfect` (commit `16bd9c704`, merge-base with eval base `35c7530ac` is `35c7530ac` itself). Work branch `verify/traefik-gateway-listener-isolation-v-16696101`. Claim-target branches were fetched from `kaitranntt-evals/traefik-gateway-listener-isolation` (remote `claims`) into separate `jj` workspaces `~/repos/traefik-4fd51fba`, `~/repos/traefik-7b8831c5`, `~/repos/traefik-91454014`; each is a single commit on top of `35c7530ac`.

The three eval fixtures from `eval_tests.zip` were copied byte-exact to `pkg/provider/kubernetes/gateway/{eval_https_test.go,eval_isolation_test.go,export_eval_test.go}` (absent on every branch). Baseline on the audited branch:

```console
$ cd ~/repos/traefik && go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/ && go vet ./pkg/provider/kubernetes/gateway/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	3.765s
VET_OK
```

Probes under `verify/repro/` are `//go:build verify` test files in package `gateway_test`; they reuse the fixture helpers (`gateway.EvalIso*Exported`, `evalServeHTTPS`), so the fixtures must be copied into the package directory before running them.

## C1

Target branch: `evalon/traefik-ga-4fd51fba` (workspace `~/repos/traefik-4fd51fba`). Authored isolation test: `TestLoadRoutes_listenerIsolation` in `pkg/provider/kubernetes/gateway/kubernetes_test.go` (the branch has no `listener_test.go`; `eval_*_test.go` are the eval's fixtures, not authored tests). The test routes requests through `newIsolationTestHandler`, a muxer built from the provider's routers whose leaf handler writes `X-Route` = the HTTPRoute name parsed out of the router name (`httproute-default-<route>-gw-...`) — i.e. a backend-route identity independent of listener values — and `-misdirected` fallback routers answer 404.

Assertions relevant to the claim (verbatim, `kubernetes_test.go`):

```go
10345:				{host: "bar.foo.example.com", path: "/hostnames", expectedStatus: http.StatusOK, expectedRoute: "attaches-to-wildcard-foo-example-com"},
10365:				{serverName: "example.org", host: "example.org", expectedStatus: http.StatusOK, expectedRoute: "attaches-to-https"},
10371:				{serverName: "second-example.org", host: "second-example.org", path: "/other", expectedStatus: http.StatusNotFound},
```

Fixture `fixtures/httproute/with_https_listeners_misdirected_requests.yml`: listener `https` (no hostname) has route `attaches-to-https` with `PathPrefix /`; listener `second-example-org` (hostname `second-example.org`) has route `attaches-to-second-example-org` with `PathPrefix /second` only. So `/other` on `second-example.org` matches no route of the more-specific listener and would match the broader route `attaches-to-https`; the test asserts 404 and `X-Route == ""` (line 10426 asserts `expectedRoute` for every request).

Baseline run:

```console
$ cd ~/repos/traefik-4fd51fba && go test -count=1 -race -v -timeout=5m -run '^TestLoadRoutes_listenerIsolation$' ./pkg/provider/kubernetes/gateway
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
    --- PASS: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners (0.48s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners (0.48s)
    --- PASS: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners (0.48s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.535s
```

Challenge: disable the exclusion rule the solution appends to route rules and check whether the authored test notices.

```diff
 func buildIsolationRule(routeHostnames, isolatedHostnames []gatev1.Hostname) string {
+	return "" // VERIFY MUTATION: disable listener isolation
```

```console
$ go test -count=1 -race -v -timeout=5m -run '^TestLoadRoutes_listenerIsolation$' ./pkg/provider/kubernetes/gateway
 expected: "attaches-to-wildcard-example-com" actual : "attaches-to-empty-hostname-with-hostnames" Messages: SNI: "", Host: "bar.example.com", Path: "/foo.Bar/Baz"
 expected: "attaches-to-wildcard-foo-example-com" actual : "attaches-to-empty-hostname-with-hostnames" Messages: SNI: "", Host: "bar.foo.example.com", Path: "/hostnames"
 expected: 421 actual : 200 Messages: SNI: "example.org", Host: "second-example.org", Path: "/other"
 expected: "" actual : "attaches-to-https" Messages: SNI: "example.org", Host: "second-example.org", Path: "/other"
 expected: 404 actual : 421 Messages: SNI: "second-example.org", Host: "second-example.org", Path: "/other"
--- FAIL: TestLoadRoutes_listenerIsolation (0.00s)
    --- FAIL: TestLoadRoutes_listenerIsolation/GRPCRoutes_attached_to_HTTP_listeners (0.48s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTP_listeners (0.48s)
    --- FAIL: TestLoadRoutes_listenerIsolation/HTTPRoutes_attached_to_HTTPS_listeners (0.48s)
FAIL
```

Reading: with exclusion removed, `Host second-example.org /other` is served by the broader backend route `attaches-to-https` (200, `X-Route: attaches-to-https`) — the broader route takes precedence over the `-misdirected` fallback responder — and the authored 404/empty-route assertion at line 10371 fails (it gets 421 from the broader route's `ServerNameCheck` when SNI is `second-example.org`, and 200 from the backend when SNI is `example.org`). The HTTP cases also fail: `bar.foo.example.com /hostnames` and `bar.example.com /foo.Bar/Baz` fall through to the broader `attaches-to-empty-hostname(-with-hostnames)` routes, which the test identifies by name. The mutation was reverted (`jj restore pkg/provider/kubernetes/gateway/kubernetes.go`; `grep -c "VERIFY MUTATION"` → 0).

Verdict: REFUTED — at least one authored, executed test (line 10371) identifies the broader backend route independently (`X-Route` from the HTTPRoute name) and establishes its exclusion by rejecting a request that would otherwise be served by it, and the mutation shows that broader route does take precedence over the fallback responder.

## C2

Audited branch. Probe: `verify/repro/c2_c3_sibling_gateway_crossed_names_test.go` — Gateway `gw-a` with HTTPS listener `wildcard=*.example.com` + route `/a`, sibling Gateway `gw-b` with HTTPS listener `exact=bar.example.com` + route `/b`, same entry point `websecure`; dumps routers/TLS options, then sends requests through the production HTTPS chain (`evalServeHTTPS`: `server.NewRouterFactory` → `router.GetHTTPSHandler()`, TLS options chosen by SNI via `HTTP3TLSConfigMatcherFunc`) with the global zerolog logger captured at debug level.

```console
$ cd ~/repos/traefik && cp verify/repro/c2_c3_sibling_gateway_crossed_names_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -race -v -run '^TestVerify_C2C3' ./pkg/provider/kubernetes/gateway
=== RUN   TestVerify_C2C3_SiblingGatewayCrossedNames/strictTLSOptions=false
    ROUTER httproute-default-route-a-gw-default-gw-a-ep-websecure-0-fd8e335dd1fdef297a4c
            rule=Host("**.example.com") && Path("/a")
            parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38] tls.options="<nil>" service="httproute-default-route-a-gw-default-gw-a-ep-websecure-0-fd8e335dd1fdef297a4c-wrr"
    ROUTER httproute-default-route-b-gw-default-gw-b-ep-websecure-0-992fa570b3bc5eceaa62
            rule=Host("bar.example.com") && Path("/b")
            parents=[listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38] tls.options="<nil>" service="httproute-default-route-b-gw-default-gw-b-ep-websecure-0-992fa570b3bc5eceaa62-wrr"
    ROUTER listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38
            rule=Host("bar.example.com")
            parents=[] tls.options="listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38" service=""
    ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb
            rule=(Host("**.example.com")) && !(Host("bar.example.com"))
            parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" service=""
    TLSOPTION listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb
    TLSOPTION listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38
    REQUEST SNI=foo.example.com Host=foo.example.com path=/a -> 200 body="10.10.0.1"
    REQUEST SNI=bar.example.com Host=bar.example.com path=/b -> 200 body="10.10.0.1"
    REQUEST SNI=bar.example.com Host=bar.example.com path=/a -> 200 body="10.10.0.1"
    REQUEST SNI=bar.example.com Host=foo.example.com path=/a -> 421 body="Misdirected Request\n"
            snicheck-log=[{"level":"debug","routerName":"listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway","req.Host":"foo.example.com","req.TLS.ServerName":"bar.example.com","message":"TLS options difference: SNI:listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway, Header:listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"}]
    CLAIM OBSERVED: crossed request rejected with 421
    REQUEST SNI=foo.example.com Host=bar.example.com path=/a -> 421 body="Misdirected Request\n"
            snicheck-log=[{"level":"debug","routerName":"listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway","req.Host":"bar.example.com","req.TLS.ServerName":"foo.example.com","message":"TLS options difference: SNI:listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway, Header:listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway"}]
=== RUN   TestVerify_C2C3_SiblingGatewayCrossedNames/strictTLSOptions=true
    (identical routers, TLS options and request results; the crossed request is 421 with the same snicheck log line)
--- PASS: TestVerify_C2C3_SiblingGatewayCrossedNames (0.54s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.627s
```

Part "TLS identity partitioning": CONFIRMED — `buildListenerRouters` emits two parent routers for the entry point, `listener-websecure-https-example-com-…` (rule `(Host("**.example.com")) && !(Host("bar.example.com"))`) and `listener-websecure-https-bar-example-com-…` (rule `Host("bar.example.com")`), each with its own `tls.Options` entry named after itself. Gateway A's route is a child of both (A's wildcard listener is elected for the `bar.example.com` partition too), yet `bar.example.com` and `foo.example.com` resolve to different global TLS option names (`…-bar-example-com-…@kubernetesgateway` vs `…-example-com-…@kubernetesgateway`), as the SNICheck log line shows.

Part "Production rejection": CONFIRMED — SNI `bar.example.com` + Host `foo.example.com` to A's `/a` route returns 421 `Misdirected Request` in both strict modes, and the debug line is emitted by `snicheck.SNICheck.ServeHTTP` on the parent router `listener-websecure-https-example-com-…` ("TLS options difference"), i.e. before the child route dispatch (controls: same-name requests to `/a` and `/b` return 200 with the backend body `10.10.0.1`).

Impact reasoning: the Gateway API `HTTPSListenerDetectMisdirectedRequests` rule compares the listeners *of the Gateway* the SNI and Host select; within Gateway A both names select the same `*.example.com` listener, so a client coalescing an HTTP/2 connection opened for `bar.example.com` onto `foo.example.com` (both under A's wildcard certificate) is a legitimate request that A must serve. The eval fixture `lone_wildcard_listener` asserts exactly this (200 for `SNI bar.example.com / Host foo.example.com`) and passes when A is alone; adding an unrelated Gateway B that claims `bar.example.com` on the shared entry point silently turns the same request into a 421.

## C3

Same probe and run as C2 (`verify/repro/c2_c3_sibling_gateway_crossed_names_test.go`, command and output reproduced here for self-containment):

```console
$ cd ~/repos/traefik && go test -tags verify -count=1 -race -v -run '^TestVerify_C2C3' ./pkg/provider/kubernetes/gateway
    ROUTER listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38
            rule=Host("bar.example.com")
            parents=[] tls.options="listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38" service=""
    ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb
            rule=(Host("**.example.com")) && !(Host("bar.example.com"))
            parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" service=""
    ROUTER httproute-default-route-a-gw-default-gw-a-ep-websecure-0-fd8e335dd1fdef297a4c
            rule=Host("**.example.com") && Path("/a")
            parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38] tls.options="<nil>"
    TLSOPTION listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb
    TLSOPTION listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38
    REQUEST SNI=bar.example.com Host=bar.example.com path=/a -> 200 body="10.10.0.1"
    REQUEST SNI=bar.example.com Host=foo.example.com path=/a -> 421 body="Misdirected Request\n"
            snicheck-log=[{"level":"debug","routerName":"listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway","req.Host":"foo.example.com","req.TLS.ServerName":"bar.example.com","message":"TLS options difference: SNI:listener-websecure-https-bar-example-com-1dee2d48e6d5713daf38@kubernetesgateway, Header:listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"}]
--- PASS: TestVerify_C2C3_SiblingGatewayCrossedNames (0.54s)   (both strictTLSOptions=false and =true)
```

Part "TLS option partitioning": CONFIRMED — the exact (`bar.example.com`) and wildcard (`**.example.com` minus `bar.example.com`) partitions are two parent routers with distinct `tls.Options` (`conf.TLS.Options` holds both names), and A's wildcard route `route-a` has both as `ParentRefs`.

Part "Production rejection": CONFIRMED — through the production handler the request SNI `bar.example.com` / Host `foo.example.com` / `/a` returns 421 from the parent `SNICheck` ("TLS options difference", routerName = the wildcard parent router), before the child muxer; the same route answers 200 for `SNI=Host=bar.example.com` and `SNI=Host=foo.example.com`.

## C4

Target branch: `evalon/traefik-ga-7b8831c5` (workspace `~/repos/traefik-7b8831c5`). On this branch `snicheck.SNICheck.ServeHTTP` gains a listener comparison before the TLS-options comparison:

```go
	if hostListener := s.matchListener(requestdecorator.GetCanonicalHost(req.Context())); hostListener >= 0 && hostListener != s.matchListener(req.TLS.ServerName) {
		http.Error(rw, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
		return
	}
```

(`router.go` wires it: `snicheck.New(routerName, router.TLS.ResolvedOptions, next).WithListenerHostnames(router.TLS.ListenerHostnames)`.) The pre-existing TLS-options branch below it still logs `routerName`, `req.Host`, `req.TLS.ServerName`.

Probe: `verify/repro/c4_snicheck_listener_rejection_no_diagnostic_test.go` — one Gateway with `catchall=` and `precise=abc.example.com` HTTPS listeners, requests through the production HTTPS chain (`evalServeHTTPS`) with the global zerolog logger replaced by a buffer at `zerolog.DebugLevel`.

```console
$ cd ~/repos/traefik-7b8831c5 && cp ~/repos/traefik/verify/repro/c4_snicheck_listener_rejection_no_diagnostic_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -race -v -timeout=5m -run '^TestVerify_C4' ./pkg/provider/kubernetes/gateway
=== RUN   TestVerify_C4_ListenerMisdirectionDiagnostic/strictTLSOptions=false
    ROUTER default-my-gateway-443-https-listener-fallback rule=PathPrefix("/") tls={Options: ResolvedOptions: ListenerHostnames:abc.example.com,* ...}
    ROUTER httproute-default-http-app-catchall-gw-default-my-gateway-ep-websecure-0-1e3ce16f9cdf73ac805e rule=Path("/catchall") && !(Host("abc.example.com")) tls={... ListenerHostnames:abc.example.com,* ...}
    ROUTER httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-0fa0806dc8bc441269fe rule=Host("abc.example.com") && Path("/precise") tls={... ListenerHostnames:abc.example.com,* ...}
    REQUEST SNI=abc.example.com Host=abc.example.com path=/precise -> 200 body="10.10.0.1"
            log-lines=2 diagnostic-with-routerName+req.Host+req.TLS.ServerName=false
            log="{\"level\":\"debug\",\"message\":\"Service selected by WRR: httproute-default-http-app-precise-...-svc-default-whoami-0\"}\n{\"level\":\"debug\",\"message\":\"Service selected by WRR: http://10.10.0.1:80\"}"
    REQUEST SNI=abc.example.com Host=xyz.example.com path=/catchall -> 421 body="Misdirected Request\n"
            log-lines=0 diagnostic-with-routerName+req.Host+req.TLS.ServerName=false
            log=""
    CLAIM OBSERVED: 421 without a diagnostic carrying routerName, req.Host and req.TLS.ServerName
    REQUEST SNI=xyz.example.com Host=abc.example.com path=/precise -> 421 body="Misdirected Request\n"
            log-lines=0 diagnostic-with-routerName+req.Host+req.TLS.ServerName=false
            log=""
    REQUEST SNI=xyz.example.com Host=abc.example.com path=/nope -> 421 body="Misdirected Request\n"
            log-lines=0 diagnostic-with-routerName+req.Host+req.TLS.ServerName=false
            log=""
=== RUN   TestVerify_C4_ListenerMisdirectionDiagnostic/strictTLSOptions=true
    (identical: the three misdirected requests are 421 with log="" in both modes)
--- PASS: TestVerify_C4_ListenerMisdirectionDiagnostic (0.64s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.722s
```

Verdict: CONFIRMED — debug logging is on (the 200 control emits two debug lines from the service layer), yet each listener-misdirection 421 produced through the production handler emits zero log lines; no diagnostic with `routerName`, `req.Host`, `req.TLS.ServerName` exists for that rejecting branch (all routers share the default TLS options, so the logging TLS-options branch is never reached).

Impact reasoning: an operator seeing 421s from this branch has nothing in the debug log to tell which router rejected, which Host was requested or which SNI the connection carried — unlike the pre-existing TLS-options 421 which logs all three.

## C5

Target branch: `evalon/traefik-ga-91454014` (workspace `~/repos/traefik-91454014`). Authored HTTPS request test: `TestListenerIsolation` in `pkg/provider/kubernetes/gateway/listener_test.go` (`kubernetes_test.go` is unchanged on this branch). The branch has no `snicheck`/`pkg/server/router` change (`git diff --stat 35c7530ac claims/evalon/traefik-ga-91454014` touches only `features.go`, `grpcroute.go`, `httproute.go`, `kubernetes.go`, `listener_test.go`, docs).

Part "Crossed-listener success expectation" — assertion text (`listener_test.go`):

```console
$ grep -n 'httpmuxer.NewMuxer\|requestdecorator.New(nil).ServeHTTP(recorder, req, muxer.ServeHTTP)\|ServerName: "bar.example.com"\|{host: "foo.example.com", service: "exact", listener: "exact"}\|assert.Equal(t, http.StatusOK, recorder.Code)' pkg/provider/kubernetes/gateway/listener_test.go
166:			muxer := httpmuxer.NewMuxer(parser, nil)
193:				{host: "foo.example.com", service: "exact", listener: "exact"},
213:						req.TLS = &tls.ConnectionState{ServerName: "bar.example.com"}
216:					requestdecorator.New(nil).ServeHTTP(recorder, req, muxer.ServeHTTP)
220:						assert.Equal(t, http.StatusOK, recorder.Code)
```

Every HTTPS-protocol request in the test carries `req.TLS.ServerName = "bar.example.com"` (selects the `wildcard=*.example.com` listener) while `Host: foo.example.com` selects the `exact=foo.example.com` listener; the request path is `/exact/Call` and the expectation is `listener: "exact"` → `http.StatusOK`. Demonstration run:

```console
$ cd ~/repos/traefik-91454014 && go test -count=1 -race -v -timeout=5m -run '^TestListenerIsolation$' ./pkg/provider/kubernetes/gateway
        --- PASS: TestListenerIsolation/HTTPRoute_HTTPS_with_route_hostnames/foo.example.com/exact (0.00s)
        --- PASS: TestListenerIsolation/HTTPRoute_HTTPS/foo.example.com/exact (0.00s)
        --- PASS: TestListenerIsolation/GRPCRoute_HTTPS/foo.example.com/exact (0.00s)
        --- PASS: TestListenerIsolation/GRPCRoute_HTTPS_with_route_hostnames/foo.example.com/exact (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.555s
   (153 "--- PASS", 0 "--- FAIL")
```

CONFIRMED: four executed authored subtests assert 200 for SNI `bar.example.com` + Host `foo.example.com` + `/exact/Call`.

Part "Test dispatch bypass": the handler is `httpmuxer.NewMuxer(parser, nil)` fed with `muxer.AddRoute(router.Rule, …)` per provider router and invoked as `requestdecorator.New(nil).ServeHTTP(recorder, req, muxer.ServeHTTP)` (lines 166–216); the file imports neither `snicheck` nor `pkg/server/router`:

```console
$ grep -c 'snicheck\|pkg/server/router' pkg/provider/kubernetes/gateway/listener_test.go
0
```

CONFIRMED: the test dispatches directly through a muxer, without the `snicheck` middleware `buildHTTPHandler` installs for TLS routers.

Supporting observation (production behaviour on the same branch, eval fixture through the production handler):

```console
$ cd ~/repos/traefik-91454014 && cp <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/ && go test -count=1 -timeout=5m -run '^TestEval_GatewayHTTPSListenerDetectMisdirectedRequests$' ./pkg/provider/kubernetes/gateway
                	Error Trace:	.../eval_https_test.go:379
                	            	expected: 421
                	            	actual  : 200
                	Messages:   	SNI catchall, Host exact
--- FAIL: TestEval_GatewayHTTPSListenerDetectMisdirectedRequests (0.84s)
    --- FAIL: .../strictTLSOptions=false/http_exact_catchall ... grpc_exact_catchall ... wildcard_narrowed_route ... wildcard_literal_hostname_collision ... exact_wildcard_equal_length ... entry_point_basic_auth
    --- FAIL: .../strictTLSOptions=true/... (same subtests)
FAIL
```

Impact reasoning: the authored test enshrines crossed-listener success, and production on that branch indeed answers 200 (no misdirection detection: all routers share the default TLS options so the production `SNICheck` never rejects) while the branch advertises `GatewayHTTPSListenerDetectMisdirectedRequests`.
