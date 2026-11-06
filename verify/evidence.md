## Setup

All commands run on the audit VM (go1.26.3). The claim branches live in the sibling repository `kaitranntt-evals/traefik-gateway-listener-isolation`; each was checked out in its own worktree and the eval fixtures were copied byte-exact into `pkg/provider/kubernetes/gateway/`.

```sh
cd ~/repos/traefik
git remote add claimrepo https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation
git fetch claimrepo evalon/traefik-ga-ee75c112 evalon/traefik-ga-fe614666
git worktree add ../wt-c1 claimrepo/evalon/traefik-ga-ee75c112   # HEAD fc4eee534 "Support Gateway API listener isolation"
git worktree add ../wt-c2 claimrepo/evalon/traefik-ga-fe614666   # HEAD 4136dc33e "chore: apply eval changes"
for w in wt-c1 wt-c2; do cp eval_tests/tests/*.go ~/repos/$w/pkg/provider/kubernetes/gateway/; done
cd ~/repos/wt-c1 && go vet ./pkg/provider/kubernetes/gateway/   # exit 0
cd ~/repos/wt-c2 && go vet ./pkg/provider/kubernetes/gateway/   # exit 0
```

## C1

Branch: `claimrepo/evalon/traefik-ga-ee75c112` (worktree `~/repos/wt-c1`).

Naming drift: this branch has no `mostSpecificListener`/`buildListenerRouters` and selects no per-listener TLS options. Misdirection detection is a `misdirectedRequest` middleware (`pkg/middlewares/gatewayapi/misdirectedrequest`) whose hostname list is built by `misdirectedRequestMiddleware` in `kubernetes.go`; that list is the "listener selection" input, and `selectHostname` in the middleware picks the listener hostname for the SNI and for the Host header and returns 421 when they differ. `misdirectedRequestMiddleware` iterates every HTTPS listener on the port with **no** `Attached` check (unlike `loadMisdirectedRequestRouters`, which skips `!listener.Attached` for the fallback router).

Repro: `verify/repro/c1_rejected_listener_misdirection_test.go` + `verify/repro/c1_export_test.go` (copied into `pkg/provider/kubernetes/gateway/` as `verify_c1_test.go` / `verify_c1_export_test.go`). It builds a 443 HTTPS Gateway from inline YAML; the rejected listener is `precise` (hostname `abc.example.com`) whose `certificateRefs` names a nonexistent Secret `missing-secret`, so `loadGatewayListeners` leaves `Attached == false`. One HTTPRoute (`/catchall`) attaches to the `catchall` listener (no hostname). Requests go through the fixture harness `evalServeHTTPS` (real `pkg/server` router factory, TLS-options selection by SNI, `requestdecorator`, HTTPS handler chain).

```sh
cd ~/repos/wt-c1 && go test -count=1 -timeout=5m -run '^TestVerifyC1_' -v ./pkg/provider/kubernetes/gateway
```

Key output (verbatim):

```console
=== RUN   TestVerifyC1_RejectedListenerMisdirection/baseline_only_catchall
    verify_c1_test.go:69: LISTENER catchall hostname="" Attached=true
    verify_c1_test.go:72: SNI=abc.example.com Host=xyz.example.com /catchall -> 200 "10.10.0.1"
=== RUN   TestVerifyC1_RejectedListenerMisdirection/rejected_exact_listener_missing_secret
    verify_c1_test.go:77: LISTENER catchall hostname="" Attached=true
    verify_c1_test.go:77: LISTENER precise hostname="abc.example.com" Attached=false
    verify_c1_test.go:77: MIDDLEWARE gw-default-my-gateway-ep-websecure-443-misdirected-request misdirectedRequest.hostnames=["" "abc.example.com"]
    verify_c1_test.go:77: ROUTER gw-default-my-gateway-ep-websecure-443-misdirected-request rule=PathPrefix("/") middlewares=[gw-default-my-gateway-ep-websecure-443-misdirected-request] service=gw-default-my-gateway-ep-websecure-443-misdirected-request-wrr
    verify_c1_test.go:77: ROUTER httproute-default-http-app-catchall-gw-default-my-gateway-ep-websecure-0-fdbf9fe621d4c678fbb1 rule=Path("/catchall") && !Host("abc.example.com") middlewares=[gw-default-my-gateway-ep-websecure-443-misdirected-request] service=...
    verify_c1_test.go:85: SNI=abc.example.com Host=xyz.example.com /catchall -> 421 "Misdirected Request\n"
    verify_c1_test.go:88: SNI=xyz.example.com Host=xyz.example.com /catchall -> 200 "10.10.0.1"
{"level":"debug","message":"Service selected by WRR: misdirected-request-not-found"}
    verify_c1_test.go:91: SNI=abc.example.com Host=abc.example.com /catchall -> 404 ""
    verify_c1_test.go:93: SNI=<empty> Host=xyz.example.com /catchall -> 200 "10.10.0.1"
=== RUN   TestVerifyC1_RejectedListenerMisdirection/control_accepted_sibling_without_rejected_listener
    verify_c1_test.go:97: LISTENER catchall hostname="" Attached=true
    verify_c1_test.go:97: LISTENER other hostname="def.example.com" Attached=true
    verify_c1_test.go:97: MIDDLEWARE gw-default-my-gateway-ep-websecure-443-misdirected-request misdirectedRequest.hostnames=["" "def.example.com"]
    verify_c1_test.go:103: SNI=abc.example.com Host=xyz.example.com /catchall -> 200 "10.10.0.1"
=== RUN   TestVerifyC1_RejectedListenerMisdirection/accepted_sibling_plus_rejected_listener
    verify_c1_test.go:108: LISTENER precise hostname="abc.example.com" Attached=false
    verify_c1_test.go:108: MIDDLEWARE gw-default-my-gateway-ep-websecure-443-misdirected-request misdirectedRequest.hostnames=["" "abc.example.com" "def.example.com"]
    verify_c1_test.go:114: SNI=abc.example.com Host=xyz.example.com /catchall -> 421 "Misdirected Request\n"
--- PASS: TestVerifyC1_RejectedListenerMisdirection (0.32s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.357s
```

`go test -race -count=1 -timeout=5m -run '^TestVerifyC1_' ./pkg/provider/kubernetes/gateway` → `ok ... 2.413s`.

### Part 1 — rejected-listener selection: HOLDS

The listener `precise` is reported `Attached=false` (missing Secret → `ResolvedRefs=False`, `Programmed=False`), yet the generated misdirected-request middleware hostname list is `["" "abc.example.com"]` (and `["" "abc.example.com" "def.example.com"]` with an accepted sibling). The rejected hostname therefore takes part in the SNI/Host listener selection the middleware performs. In the baseline with the catch-all alone there is no middleware at all, so the rejected listener alone is what creates it.

### Part 2 — request-path effect: HOLDS

Request SNI=`abc.example.com` (matches the rejected listener), Host=`xyz.example.com`, path `/catchall` (attached catch-all route) → **421 "Misdirected Request"** with the rejected listener present, and **200** in both controls (catch-all alone; catch-all + accepted `def.example.com` sibling, where the middleware exists but does not list `abc.example.com`). The only variable between the 2-listener control and the 3-listener case is the rejected listener, and the only 421 producer in the configuration is the middleware whose hostname list changed, so the 421 is attributable to the rejected listener's participation.

### Verdict and impact

Both parts hold → **CONFIRMED**. A listener that is not programmed (e.g. its Secret is missing, the common case while a certificate is being issued) still reserves its hostname at request time: a connection whose SNI names that hostname can no longer carry requests to the healthy catch-all listener (421), and — side observation — the catch-all route is also generated with `!Host("abc.example.com")`, so a same-name request SNI=Host=`abc.example.com` gets 404 from the fallback router instead of being served by the catch-all. The eval fixture `TestEval_GatewayHTTPListenerIsolation_SingleGatewaySpecificity/rejected_listener_not_suppressing` fails on this branch for the HTTP analogue of that side observation:

```console
cd ~/repos/wt-c1 && go test -count=1 -timeout=4m -run '^TestEval_GatewayHTTPListenerIsolation_SingleGatewaySpecificity$' -v ./pkg/provider/kubernetes/gateway
        	Error Trace:	.../eval_isolation_test.go:591
        	            	expected: 200
        	            	actual  : 404
        	Messages:   	rejected listener must not suppress catch-all route
    --- FAIL: TestEval_GatewayHTTPListenerIsolation_SingleGatewaySpecificity/rejected_listener_not_suppressing (0.00s)
```

Full fixture run on this branch (`go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway`): FAIL for `SingleGatewaySpecificity`, `TrailingDotWildcard`, `TrailingDotWildcardFallthrough`, `ProtocolSeparation`, `HTTPSSiblingGatewayReachability`; no fixture covers the HTTPS rejected-listener 421 path. The 421 itself needs a crossed SNI/Host (HTTP/2 connection coalescing); a client retrying on a fresh connection recovers, so the user-facing impact is intermittent 421s plus the 404 for the rejected hostname.

## C2

Branch: `claimrepo/evalon/traefik-ga-fe614666` (worktree `~/repos/wt-c2`).

Naming drift: `isolateHostnames(hostnames, isolated)` and `isolatedHostnames(listener, listeners)` exist in `kubernetes.go`; the rule is built by `buildHostRule(hostnames, excludedHostnames)` in `httproute.go` (no `buildListenerRule`/`mostSpecificListener`). `isolateHostnames` drops route hostnames shadowed by a more specific sibling, but when **all** are shadowed it restores them (`if len(kept) == 0 { kept = hostnames }`) and only excludes isolated hostnames that the kept ones *encompass* (`findMatchingHostname(kept, isolated)`), which an exact hostname never does for a wildcard. The branch's own `Test_isolateHostnames` case "All hostnames shadowed keeps them with their exclusions" asserts exactly this (`hostnames: abc.foo.example.com, *.foo.example.com; isolated: *.example.com → expectedKept: same, expectedExcluded: nil`).

Repro: `verify/repro/c2_catchall_exact_hosts_under_wildcard_test.go` (copied into `pkg/provider/kubernetes/gateway/verify_c2_test.go`). HTTP Gateway `my-gateway` with listeners `catchall` (no hostname) and `wild` (`*.example.com`), both port 80 / entryPoint `web`. HTTPRoute `http-app-catchall` → sectionName `catchall`, hostnames `foo.example.com`, `bar.example.com`, path `/catchall`, backend `whoami-catchall` (10.10.2.1). HTTPRoute `http-app-wild` → sectionName `wild`, path `/wild`, backend `whoami-precise` (10.10.1.1). Served through the fixture harness `evalServeHTTP` (real `pkg/server` router).

```sh
cd ~/repos/wt-c2 && go test -count=1 -timeout=5m -run '^TestVerifyC2_|^Test_isolateHostnames$' -v ./pkg/provider/kubernetes/gateway
```

Key output (verbatim):

```console
--- PASS: Test_isolateHostnames (0.00s)
    --- PASS: Test_isolateHostnames/All_hostnames_shadowed_keeps_them_with_their_exclusions (0.00s)
=== RUN   TestVerifyC2_CatchAllExactHostnamesUnderWildcardListener
    verify_c2_test.go:26: ROUTER httproute-default-http-app-catchall-gw-default-my-gateway-ep-web-0-a71b313c9ab5d7f9fc25 rule=(Host("foo.example.com") || Host("bar.example.com")) && Path("/catchall") priority=100016 service=httproute-default-http-app-catchall-gw-default-my-gateway-ep-web-0-a71b313c9ab5d7f9fc25-wrr
    verify_c2_test.go:26: ROUTER httproute-default-http-app-wild-gw-default-my-gateway-ep-web-0-c635fce5496b60e089c0 rule=Host("**.example.com") && Path("/wild") priority=100014 service=httproute-default-http-app-wild-gw-default-my-gateway-ep-web-0-c635fce5496b60e089c0-wrr
{"level":"debug","message":"Service selected by WRR: http://10.10.2.1:80"}
    verify_c2_test.go:40: Host=foo.example.com /catchall -> 200 "10.10.2.1"
{"level":"debug","message":"Service selected by WRR: http://10.10.2.1:80"}
    verify_c2_test.go:40: Host=bar.example.com /catchall -> 200 "10.10.2.1"
{"level":"debug","message":"Service selected by WRR: http://10.10.1.1:80"}
    verify_c2_test.go:45: Host=foo.example.com /wild -> 200 "10.10.1.1"
    verify_c2_test.go:49: Host=other.org /catchall -> 404 "404 page not found\n"
--- PASS: TestVerifyC2_CatchAllExactHostnamesUnderWildcardListener (0.21s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.255s
```

`go test -race -count=1 -timeout=5m -run '^TestVerifyC2_' ./pkg/provider/kubernetes/gateway` → `ok ... 1.904s`.

### Part 1 — hostname isolation: HOLDS

The catch-all route's generated rule is `(Host("foo.example.com") || Host("bar.example.com")) && Path("/catchall")`: both exact hostnames are retained and there is no `!Host("**.example.com")` exclusion, even though the `wild` listener (`*.example.com`) on the same port is more specific for both of them.

### Part 2 — request-path effect: HOLDS

`Host: foo.example.com /catchall` → 200 from backend 10.10.2.1 (`whoami-catchall`), `Host: bar.example.com /catchall` → 200 from 10.10.2.1. The requests reach the route attached to the catch-all listener; the `wild` listener's own route is distinguishable (200 from 10.10.1.1 for `/wild`). Under listener isolation, requests for `*.example.com` hostnames must be handled by the `wild` listener only, so `/catchall` should have been 404.

### Verdict and impact

Both parts hold → **CONFIRMED**. A route attached to the catch-all listener that names hostnames falling under a sibling wildcard listener keeps serving them, i.e. the Gateway does not isolate the `*.example.com` listener from the catch-all for those hostnames. The branch's fixture run (`go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway`) fails `SingleGatewaySpecificity`, `HTTPSListenerDetectMisdirectedRequests`, `HTTPSSiblingGatewayReachability`, `HTTPSListenerIsolation_RouteKindRestrictedListeners`, `HTTPSListenerIsolation_UnsectionedAttachment`, but the shadowed-route fixture `TestEval_GatewayRouteAcceptanceIndependentOfIsolation` passes because it shadows with an exact sibling hostname (`abc.example.com` vs `abc.example.com`), for which `isolateHostnames` does emit the exclusion; the wildcard-covers-exact case has no fixture coverage and is the gap shown here.
