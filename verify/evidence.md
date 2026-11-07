# Evidence for audit v-bdf582bd (traefik-gateway-listener-isolation)

Verify branch: `verify/traefik-gateway-listener-isolation-v-bdf582bd` (based on `origin/traefik-gateway-listener-isolation-perfect`, commit `16bd9c70425e`).
Toolchain: `go1.26.3 linux/amd64`. Fixtures: `eval_tests.zip` extracted to `~/eval_tests/tests/` and used byte-exact.

Full fixture suite status per target branch (`go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/` with the three fixtures copied in):

```text
== evalon/traefik-ga-715b83ff
--- FAIL: TestEval_GatewayHTTPListenerIsolation_TrailingDotWildcard (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	2.469s
FAIL
== evalon/traefik-ga-e500a6b0
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	2.233s
== evalon/traefik-ga-963e6173
--- FAIL: TestEval_GatewayHTTPListenerIsolation_TrailingDotWildcard (0.00s)
--- FAIL: TestEval_GatewayHTTPListenerIsolation_TrailingDotWildcardFallthrough (0.00s)
--- FAIL: TestEval_GatewayHTTPSListenerDetectMisdirectedRequests (0.53s)
--- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability (0.55s)
--- FAIL: TestEval_GatewayHTTPSListenerIsolation_RouteKindRestrictedListeners (0.08s)
--- FAIL: TestEval_GatewayHTTPSListenerIsolation_UnsectionedAttachment (0.24s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.958s
FAIL
== evalon/traefik-ga-cbb7ae82
--- FAIL: TestEval_GatewayHTTPListenerIsolation_TrailingDotWildcard (0.00s)
--- FAIL: TestEval_GatewayHTTPListenerIsolation_MultiGatewayElection (0.01s)
--- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability (0.44s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	2.173s
FAIL
```

## C1

**Claim.** On [`evalon/traefik-ga-715b83ff`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-715b83ff), Gateways `team-a/edge` and `team/a-edge` (identical listener names, hostnames in disjoint `alpha`/`beta` families) lose the hostname-specific unmatched-path guards for one Gateway, so crossed SNI/Host requests on `/nope` return 404 instead of 421.

**Verdict: CONFIRMED** (both parts: router preservation and request handling).

Setup (shared by every claim; repeated here so this section is self-contained):

```bash
# Target branch checked out in its own worktree, with the eval fixtures copied byte-exact
# from eval_tests.zip to the paths the assessment provisions them at.
cd ~/wt/715b83ff                      # worktree of evalon/traefik-ga-715b83ff
cp ~/eval_tests/tests/eval_https_test.go ~/eval_tests/tests/eval_isolation_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/gateway/
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/ > /tmp/evalsuite-715b83ff.log 2>&1
```

The probe below is build-tagged `//go:build verify` and reuses the fixture helpers (`gateway.EvalIsoLoadExported`, `evalOneShotProvider`, ...) so the eval fixtures must be present in the package directory when it runs. Production code is untouched; the probe lives in `verify/repro/` on the verify branch and is copied into the package directory only for the run.

Probe: `verify/repro/c1_identical_listener_names_test.go`. Subtest `team/a-edge` is the collision case (Gateway `team-a/edge` + Gateway `team/a-edge`); subtest `team/b-edge` is the control with a distinct name (`team-a/edge` + `team/b-edge`). Each Gateway has an `exact` HTTPS listener (`app.<family>.example.com`) and a `wild` listener (`*.<family>.example.com`) on `websecure`, each with one HTTPRoute (`/exact`, `/wild`). Requests go through the production stack (`evalServeHTTPS` from the fixtures: TCP router → TLS → HTTP router → service).

```bash
cp verify/repro/c1_identical_listener_names_test.go ~/wt/715b83ff/pkg/provider/kubernetes/gateway/verify_c1_test.go
cd ~/wt/715b83ff && go test -tags verify -count=1 -run 'TestVerifyC1' -v ./pkg/provider/kubernetes/gateway/ 2>&1 | grep -v '^{"level"'
```

Output (verbatim, zerolog JSON lines removed):

```text
=== RUN   TestVerifyC1
=== RUN   TestVerifyC1/team/a-edge
    verify_c1_test.go:122: ROUTER httproute-team-a-exact-route-gw-team-a-edge-ep-websecure-0-4ec7515970eaff797b41: rule=Host("app.alpha.example.com") && (Path("/exact") || PathPrefix("/exact/"))
    verify_c1_test.go:122: ROUTER httproute-team-a-wild-route-gw-team-a-edge-ep-websecure-0-8a84e510b744b8d898bf: rule=HostRegexp("^[a-z0-9-\\.]+\\.alpha\\.example\\.com$") && (Path("/wild") || PathPrefix("/wild/")) && !Host("app.alpha.example.com")
    verify_c1_test.go:122: ROUTER httproute-team-exact-route-gw-team-a-edge-ep-websecure-0-a4b4b856b683812b58e5: rule=Host("app.beta.example.com") && (Path("/exact") || PathPrefix("/exact/"))
    verify_c1_test.go:122: ROUTER httproute-team-wild-route-gw-team-a-edge-ep-websecure-0-1aae3313984be2f3c39a: rule=HostRegexp("^[a-z0-9-\\.]+\\.beta\\.example\\.com$") && (Path("/wild") || PathPrefix("/wild/")) && !Host("app.beta.example.com")
    verify_c1_test.go:120: FALLBACK ROUTER team-a-edge-exact-fallback: rule=Host("app.alpha.example.com") listenerHostnames="*.alpha.example.com,app.alpha.example.com"
    verify_c1_test.go:120: FALLBACK ROUTER team-a-edge-wild-fallback: rule=Host("**.alpha.example.com") listenerHostnames="*.alpha.example.com,app.alpha.example.com"
    verify_c1_test.go:143: REQ sni=app.alpha.example.com    host=app.alpha.example.com    path=/exact -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=x.alpha.example.com      host=x.alpha.example.com      path=/wild  -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=app.beta.example.com     host=app.beta.example.com     path=/exact -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=x.beta.example.com       host=x.beta.example.com       path=/wild  -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=app.alpha.example.com    host=x.alpha.example.com      path=/nope  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=x.alpha.example.com      host=app.alpha.example.com    path=/nope  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=app.beta.example.com     host=x.beta.example.com       path=/nope  -> 404 404 page not found
    verify_c1_test.go:143: REQ sni=x.beta.example.com       host=app.beta.example.com     path=/nope  -> 404 404 page not found
    verify_c1_test.go:143: REQ sni=app.alpha.example.com    host=x.alpha.example.com      path=/wild  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=app.beta.example.com     host=x.beta.example.com       path=/wild  -> 421 Misdirected Request
=== RUN   TestVerifyC1/team/b-edge
    verify_c1_test.go:122: ROUTER httproute-team-a-exact-route-gw-team-a-edge-ep-websecure-0-4ec7515970eaff797b41: rule=Host("app.alpha.example.com") && (Path("/exact") || PathPrefix("/exact/"))
    verify_c1_test.go:122: ROUTER httproute-team-a-wild-route-gw-team-a-edge-ep-websecure-0-8a84e510b744b8d898bf: rule=HostRegexp("^[a-z0-9-\\.]+\\.alpha\\.example\\.com$") && (Path("/wild") || PathPrefix("/wild/")) && !Host("app.alpha.example.com")
    verify_c1_test.go:122: ROUTER httproute-team-exact-route-gw-team-b-edge-ep-websecure-0-e3741598dbf08bd6430b: rule=Host("app.beta.example.com") && (Path("/exact") || PathPrefix("/exact/"))
    verify_c1_test.go:122: ROUTER httproute-team-wild-route-gw-team-b-edge-ep-websecure-0-5f03439b58f0c36182ad: rule=HostRegexp("^[a-z0-9-\\.]+\\.beta\\.example\\.com$") && (Path("/wild") || PathPrefix("/wild/")) && !Host("app.beta.example.com")
    verify_c1_test.go:120: FALLBACK ROUTER team-a-edge-exact-fallback: rule=Host("app.alpha.example.com") listenerHostnames="*.alpha.example.com,app.alpha.example.com"
    verify_c1_test.go:120: FALLBACK ROUTER team-a-edge-wild-fallback: rule=Host("**.alpha.example.com") listenerHostnames="*.alpha.example.com,app.alpha.example.com"
    verify_c1_test.go:120: FALLBACK ROUTER team-b-edge-exact-fallback: rule=Host("app.beta.example.com") listenerHostnames="*.beta.example.com,app.beta.example.com"
    verify_c1_test.go:120: FALLBACK ROUTER team-b-edge-wild-fallback: rule=Host("**.beta.example.com") listenerHostnames="*.beta.example.com,app.beta.example.com"
    verify_c1_test.go:143: REQ sni=app.alpha.example.com    host=app.alpha.example.com    path=/exact -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=x.alpha.example.com      host=x.alpha.example.com      path=/wild  -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=app.beta.example.com     host=app.beta.example.com     path=/exact -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=x.beta.example.com       host=x.beta.example.com       path=/wild  -> 200 10.10.0.1
    verify_c1_test.go:143: REQ sni=app.alpha.example.com    host=x.alpha.example.com      path=/nope  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=x.alpha.example.com      host=app.alpha.example.com    path=/nope  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=app.beta.example.com     host=x.beta.example.com       path=/nope  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=x.beta.example.com       host=app.beta.example.com     path=/nope  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=app.alpha.example.com    host=x.alpha.example.com      path=/wild  -> 421 Misdirected Request
    verify_c1_test.go:143: REQ sni=app.beta.example.com     host=x.beta.example.com       path=/wild  -> 421 Misdirected Request
--- PASS: TestVerifyC1 (0.58s)
    --- PASS: TestVerifyC1/team/a-edge (0.54s)
    --- PASS: TestVerifyC1/team/b-edge (0.05s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.671s
```

**Reading.** In the collision subtest the provider emits only `team-a-edge-exact-fallback` / `team-a-edge-wild-fallback` (alpha family). The beta family's fallback routers are missing — `FALLBACK ROUTER` lines for `app.beta.example.com` / `**.beta.example.com` are absent — because the fallback router name is built as `provider.Normalize(gateway.Namespace + "-" + gateway.Name + "-" + listener.Name + "-fallback")` (`kubernetes.go:692`), so `team-a`+`edge` and `team`+`a-edge` both normalize to `team-a-edge-<listener>-fallback` and one Gateway's entry overwrites the other's in the router map. Consequently crossed requests on the beta hostnames (`sni=app.beta.example.com host=x.beta.example.com /nope` and the reverse) return `404 404 page not found` instead of `421`. In the control subtest (distinct Gateway name `b-edge`) all four fallback routers exist and the same crossed requests return `421 Misdirected Request`. Only the Gateway identity changed between the two subtests, which isolates the collision as the cause.

**Impact.** Two Gateways in different namespaces whose `namespace-name` strings collide under hyphen joining silently lose the misdirected-request guard for all of one Gateway's HTTPS listeners: an attacker-controlled Host header against a connection negotiated for a different listener yields 404 instead of the required 421, and (as the `/wild` crossed line shows) only the explicit route-level exclusions remain. `GatewayHTTPSListenerDetectMisdirectedRequests` is therefore advertised but not enforced for the affected Gateway.

## C2

**Claim.** On [`evalon/traefik-ga-e500a6b0`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-e500a6b0), when an entry point carries authentication middleware, an unauthenticated HTTPS request whose SNI and Host select different listeners returns 401 instead of 421 because authentication runs before the SNI/Host rejection.

**Verdict: CONFIRMED.**

Setup (shared by every claim; repeated here so this section is self-contained):

```bash
# Target branch checked out in its own worktree, with the eval fixtures copied byte-exact
# from eval_tests.zip to the paths the assessment provisions them at.
cd ~/wt/e500a6b0                      # worktree of evalon/traefik-ga-e500a6b0
cp ~/eval_tests/tests/eval_https_test.go ~/eval_tests/tests/eval_isolation_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/gateway/
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/ > /tmp/evalsuite-e500a6b0.log 2>&1
```

The probe below is build-tagged `//go:build verify` and reuses the fixture helpers (`gateway.EvalIsoLoadExported`, `evalOneShotProvider`, ...) so the eval fixtures must be present in the package directory when it runs. Production code is untouched; the probe lives in `verify/repro/` on the verify branch and is copied into the package directory only for the run.

Probe: `verify/repro/c2_entrypoint_auth_before_421_test.go`. It loads a Gateway with HTTPS listeners `precise=abc.example.com` and `wildcard=*.example.com` (routes `/precise`, `/wildcard`) through the real `server.ConfigurationWatcher` + `router.NewManager` stack with two providers: the Gateway provider output and an `internal` provider contributing an entry-point `Model` for `websecure`. Subtest `entrypoint-auth` attaches a BasicAuth middleware `epauth@internal` to the entry-point model (user `test`, password `test`, bcrypt generated at run time); subtest `no-entrypoint-auth` supplies an empty model. It dumps the runtime routers after aggregation and sends each request without and with an `Authorization: Basic` header.

```bash
cp verify/repro/c2_entrypoint_auth_before_421_test.go ~/wt/e500a6b0/pkg/provider/kubernetes/gateway/verify_c2_test.go
cd ~/wt/e500a6b0 && go test -tags verify -count=1 -run 'TestVerifyC2' -v ./pkg/provider/kubernetes/gateway/ 2>&1 | grep -v '^{"level"'
```

Output (verbatim, zerolog JSON lines removed):

```text
=== RUN   TestVerifyC2
=== RUN   TestVerifyC2/no-entrypoint-auth
    verify_c2_test.go:159: RUNTIME ROUTER httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-0fa0806dc8bc441269fe@kubernetesgateway: prio=100016 middlewares=[] service=httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-0fa0806dc8bc441269fe-wrr rule=Host("abc.example.com") && Path("/precise")
    verify_c2_test.go:159: RUNTIME ROUTER httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-50919966ee2aba4b8c3b@kubernetesgateway: prio=100014 middlewares=[] service=httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-50919966ee2aba4b8c3b-wrr rule=(Host("**.example.com") && Path("/wildcard")) && !(Host("abc.example.com"))
    verify_c2_test.go:159: RUNTIME ROUTER misdirected-default-precise-gw-default-my-gateway-ep-websecure-0-21242b621f31dd4ae37d@kubernetesgateway: prio=9223372036854774807 middlewares=[] service=misdirected-default-precise-gw-default-my-gateway-ep-websecure-0-21242b621f31dd4ae37d rule=(Host("abc.example.com")) && !(HostSNI("abc.example.com"))
    verify_c2_test.go:159: RUNTIME ROUTER misdirected-default-wildcard-gw-default-my-gateway-ep-websecure-0-fa269b157123508a33c6@kubernetesgateway: prio=9223372036854774807 middlewares=[] service=misdirected-default-wildcard-gw-default-my-gateway-ep-websecure-0-fa269b157123508a33c6 rule=((Host("**.example.com")) && !(Host("abc.example.com"))) && !(HostSNI("**.example.com") && !HostSNI("abc.example.com"))
    verify_c2_test.go:170: UNAUTH REQ sni=abc.example.com  host=abc.example.com  path=/precise  -> 200 10.10.0.1
    verify_c2_test.go:172: AUTHED REQ sni=abc.example.com  host=abc.example.com  path=/precise  -> 200 10.10.0.1
    verify_c2_test.go:170: UNAUTH REQ sni=xyz.example.com  host=xyz.example.com  path=/wildcard -> 200 10.10.0.1
    verify_c2_test.go:172: AUTHED REQ sni=xyz.example.com  host=xyz.example.com  path=/wildcard -> 200 10.10.0.1
    verify_c2_test.go:170: UNAUTH REQ sni=xyz.example.com  host=abc.example.com  path=/precise  -> 421 
    verify_c2_test.go:172: AUTHED REQ sni=xyz.example.com  host=abc.example.com  path=/precise  -> 421 
    verify_c2_test.go:170: UNAUTH REQ sni=abc.example.com  host=xyz.example.com  path=/wildcard -> 421 
    verify_c2_test.go:172: AUTHED REQ sni=abc.example.com  host=xyz.example.com  path=/wildcard -> 421 
    verify_c2_test.go:170: UNAUTH REQ sni=xyz.example.com  host=abc.example.com  path=/nope     -> 421 
    verify_c2_test.go:172: AUTHED REQ sni=xyz.example.com  host=abc.example.com  path=/nope     -> 421 
=== RUN   TestVerifyC2/entrypoint-auth
    verify_c2_test.go:159: RUNTIME ROUTER httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-0fa0806dc8bc441269fe@kubernetesgateway: prio=100016 middlewares=[epauth@internal] service=httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-0fa0806dc8bc441269fe-wrr rule=Host("abc.example.com") && Path("/precise")
    verify_c2_test.go:159: RUNTIME ROUTER httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-50919966ee2aba4b8c3b@kubernetesgateway: prio=100014 middlewares=[epauth@internal] service=httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-50919966ee2aba4b8c3b-wrr rule=(Host("**.example.com") && Path("/wildcard")) && !(Host("abc.example.com"))
    verify_c2_test.go:159: RUNTIME ROUTER misdirected-default-precise-gw-default-my-gateway-ep-websecure-0-21242b621f31dd4ae37d@kubernetesgateway: prio=9223372036854774807 middlewares=[epauth@internal] service=misdirected-default-precise-gw-default-my-gateway-ep-websecure-0-21242b621f31dd4ae37d rule=(Host("abc.example.com")) && !(HostSNI("abc.example.com"))
    verify_c2_test.go:159: RUNTIME ROUTER misdirected-default-wildcard-gw-default-my-gateway-ep-websecure-0-fa269b157123508a33c6@kubernetesgateway: prio=9223372036854774807 middlewares=[epauth@internal] service=misdirected-default-wildcard-gw-default-my-gateway-ep-websecure-0-fa269b157123508a33c6 rule=((Host("**.example.com")) && !(Host("abc.example.com"))) && !(HostSNI("**.example.com") && !HostSNI("abc.example.com"))
    verify_c2_test.go:170: UNAUTH REQ sni=abc.example.com  host=abc.example.com  path=/precise  -> 401 401 Unauthorized
    verify_c2_test.go:172: AUTHED REQ sni=abc.example.com  host=abc.example.com  path=/precise  -> 200 10.10.0.1
    verify_c2_test.go:170: UNAUTH REQ sni=xyz.example.com  host=xyz.example.com  path=/wildcard -> 401 401 Unauthorized
    verify_c2_test.go:172: AUTHED REQ sni=xyz.example.com  host=xyz.example.com  path=/wildcard -> 200 10.10.0.1
    verify_c2_test.go:170: UNAUTH REQ sni=xyz.example.com  host=abc.example.com  path=/precise  -> 401 401 Unauthorized
    verify_c2_test.go:172: AUTHED REQ sni=xyz.example.com  host=abc.example.com  path=/precise  -> 421 
    verify_c2_test.go:170: UNAUTH REQ sni=abc.example.com  host=xyz.example.com  path=/wildcard -> 401 401 Unauthorized
    verify_c2_test.go:172: AUTHED REQ sni=abc.example.com  host=xyz.example.com  path=/wildcard -> 421 
    verify_c2_test.go:170: UNAUTH REQ sni=xyz.example.com  host=abc.example.com  path=/nope     -> 401 401 Unauthorized
    verify_c2_test.go:172: AUTHED REQ sni=xyz.example.com  host=abc.example.com  path=/nope     -> 421 
--- PASS: TestVerifyC2 (0.18s)
    --- PASS: TestVerifyC2/no-entrypoint-auth (0.11s)
    --- PASS: TestVerifyC2/entrypoint-auth (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.212s
```

**Reading.** The branch implements misdirected-request detection as ordinary HTTP routers (`misdirected-...` with rule `Host(...) && !HostSNI(...)`, service returning 421). `pkg/server/aggregator.go` `applyModel` prepends entry-point model middlewares to every root router (`cp.Middlewares = append(m.Middlewares, cp.Middlewares...)`), so after aggregation the generated `misdirected-*` routers carry `middlewares=[epauth@internal]` exactly like the real routes (see the `RUNTIME ROUTER` lines in the `entrypoint-auth` subtest). At request time the router chain runs the middleware before the 421 service: with auth configured, every crossed request (`sni=xyz.example.com host=abc.example.com /precise`, `sni=abc.example.com host=xyz.example.com /wildcard`, `.../nope`) returns `401 Unauthorized` when unauthenticated and `421` only once valid credentials are supplied. The `no-entrypoint-auth` control returns `421` for the same crossed requests regardless of credentials, so the 401 is caused by the entry-point middleware ordering and not by the request shape.

**Impact.** The SNI/Host consistency check is a transport-level guard and must reject a misdirected request before any route-level processing; here the authentication middleware (and any other entry-point middleware such as rate limiting, forward-auth, or request logging with body capture) executes against a request that should never have reached the listener. A client that holds credentials for one listener can still get the misdirected response differentiated (401 vs 421) and unauthenticated crossed requests exercise the auth backend. Behavior diverges from the Gateway API conformance expectation of 421 for `GatewayHTTPSListenerDetectMisdirectedRequests`.

## C3

**Claim.** On [`evalon/traefik-ga-e500a6b0`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-e500a6b0), the added multi-listener positive-isolation tests (`TestListenerIsolation`, `TestHTTPSListenerDetectMisdirectedRequests`, `TestListenerIsolationScope` in `pkg/provider/kubernetes/gateway/listener_test.go`) do not verify that accepted routes bind to their configured backends.

**Verdict: CONFIRMED.**

Setup (shared by every claim; repeated here so this section is self-contained):

```bash
# Target branch checked out in its own worktree, with the eval fixtures copied byte-exact
# from eval_tests.zip to the paths the assessment provisions them at.
cd ~/wt/e500a6b0                      # worktree of evalon/traefik-ga-e500a6b0
cp ~/eval_tests/tests/eval_https_test.go ~/eval_tests/tests/eval_isolation_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/gateway/
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/ > /tmp/evalsuite-e500a6b0.log 2>&1
```

The probe below is build-tagged `//go:build verify` and reuses the fixture helpers (`gateway.EvalIsoLoadExported`, `evalOneShotProvider`, ...) so the eval fixtures must be present in the package directory when it runs. Production code is untouched; the probe lives in `verify/repro/` on the verify branch and is copied into the package directory only for the run.

Assertion text. The tests build their handler with `isolationHandler` (`listener_test.go:241`), which registers every router rule on a bare muxer with a synthetic handler that writes a fixed status and ignores the router's service entirely unless the service name is literally `misdirected-request`:

```go
status := http.StatusOK
if service := conf.HTTP.Services[router.Service]; service != nil && service.Weighted != nil && len(service.Weighted.Services) == 1 && service.Weighted.Services[0].Name == "misdirected-request" {
    status = *service.Weighted.Services[0].Status
}
err := mux.AddRoute(router.Rule, router.RuleSyntax, router.Priority, "kubernetesgateway", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
    rw.WriteHeader(status)
}))
```

and the positive assertions are of the form `assert.Equal(t, want, isolationRequest(handler, protocol, test.host, test.host, "/route/"+route))` with `want = http.StatusOK` when `route == test.listener` — a status code only; nothing inspects which service or backend the accepted router points at.

Demonstration run (mutation): make every accepted HTTPRoute/GRPCRoute router point at a non-existent service, then run the added tests and, for contrast, the configuration-comparing tests and the eval fixture that goes through the production service layer.

```bash
cd ~/wt/e500a6b0
cp pkg/provider/kubernetes/gateway/httproute.go /tmp/httproute.go.orig; cp pkg/provider/kubernetes/gateway/grpcroute.go /tmp/grpcroute.go.orig
patch -p0 < verify/repro/c3_bogus_service_mutation.diff   # (equivalently, apply the two one-line insertions shown below)
go test -count=1 -run '^(TestListenerIsolation|TestHTTPSListenerDetectMisdirectedRequests|TestListenerIsolationScope|TestSupportedListenerIsolationFeatures)$' ./pkg/provider/kubernetes/gateway/
go test -count=1 -run '^(TestLoadHTTPRoutes|TestLoadGRPCRoutes|TestEval_GatewayHTTPListenerIsolation_SingleGatewaySpecificity_ProductionHTTP)$' ./pkg/provider/kubernetes/gateway/
cp /tmp/httproute.go.orig pkg/provider/kubernetes/gateway/httproute.go; cp /tmp/grpcroute.go.orig pkg/provider/kubernetes/gateway/grpcroute.go
```

Mutation applied (`verify/repro/c3_bogus_service_mutation.diff`):

```diff
--- /tmp/httproute.go.orig	2026-11-07 00:44:03.608427712 +0000
+++ pkg/provider/kubernetes/gateway/httproute.go	2026-11-07 00:44:03.611427704 +0000
@@ -185,6 +185,7 @@
 
 			p.applyRouterTransform(ctx, &router, route)
 
+			router.Service = "verify-bogus-service" // C3 mutation
 			conf.HTTP.Routers[routerName] = &router
 		}
 	}
--- /tmp/grpcroute.go.orig	2026-11-07 00:44:03.609427710 +0000
+++ pkg/provider/kubernetes/gateway/grpcroute.go	2026-11-07 00:44:03.612427701 +0000
@@ -173,6 +173,7 @@
 				}
 			}
 
+			router.Service = "verify-bogus-service" // C3 mutation
 			conf.HTTP.Routers[routerName] = &router
 		}
 	}
```

Output of the added tests under the mutation (verbatim):

```text
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.098s
```

Output of the configuration-comparing tests and the production-stack eval fixture under the same mutation (verbatim, `--- FAIL`/`ok`/`FAIL` lines):

```text
--- FAIL: TestLoadHTTPRoutes (0.00s)
--- FAIL: TestLoadGRPCRoutes (0.00s)
--- FAIL: TestEval_GatewayHTTPListenerIsolation_SingleGatewaySpecificity_ProductionHTTP (0.03s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.166s
FAIL
```

(The three failures are `assert.Equal` diffs of the whole `dynamic.Configuration` for `TestLoadHTTPRoutes`/`TestLoadGRPCRoutes`, and a non-200 status from the real service layer for the eval fixture.)

**Reading.** With every accepted route pointing at `verify-bogus-service`, the branch's added isolation tests stay green (`ok ... 0.098s`), while tests that compare the generated service wiring or exercise the real router→service path fail immediately. The added tests therefore measure only that *some* router with the expected rule exists and would be selected; they cannot detect a route being accepted but bound to the wrong (or no) backend.

**Impact.** A regression that keeps listener matching intact but mis-wires accepted routes (wrong service name, lost `BackendRefs`, child router without a service) passes the branch's new regression tests. Since the task explicitly asked for regression tests of the isolation behavior, the added coverage does not protect the "accepted route reaches its configured backend" half of the contract.

## C4

**Claim.** On [`evalon/traefik-ga-963e6173`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-963e6173), with sibling HTTPS listeners `a.example.com` and `*.example.com` on the same port, a valid request to the exact listener (SNI `a.example.com`, Host `a.example.com`) returns 421 because `mostSpecificHostname` keeps an equal-length wildcard.

**Verdict: CONFIRMED.**

Setup (shared by every claim; repeated here so this section is self-contained):

```bash
# Target branch checked out in its own worktree, with the eval fixtures copied byte-exact
# from eval_tests.zip to the paths the assessment provisions them at.
cd ~/wt/963e6173                      # worktree of evalon/traefik-ga-963e6173
cp ~/eval_tests/tests/eval_https_test.go ~/eval_tests/tests/eval_isolation_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/gateway/
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/ > /tmp/evalsuite-963e6173.log 2>&1
```

The probe below is build-tagged `//go:build verify` and reuses the fixture helpers (`gateway.EvalIsoLoadExported`, `evalOneShotProvider`, ...) so the eval fixtures must be present in the package directory when it runs. Production code is untouched; the probe lives in `verify/repro/` on the verify branch and is copied into the package directory only for the run.

Probe: `verify/repro/c4_equal_length_wildcard_test.go`. Subtest `equal-length` uses listeners `exact=a.example.com` (13 chars) and `wildcard=*.example.com` (13 chars); subtest `longer` uses `exact=abc.example.com` (15 chars) against the same wildcard. Each listener has one HTTPRoute (`/exact`, `/wildcard`). Requests traverse the production stack via `evalServeHTTPS`.

```bash
cp verify/repro/c4_equal_length_wildcard_test.go ~/wt/963e6173/pkg/provider/kubernetes/gateway/verify_c4_test.go
cd ~/wt/963e6173 && go test -tags verify -count=1 -run 'TestVerifyC4' -v ./pkg/provider/kubernetes/gateway/ 2>&1 | grep -v '^{"level"'
```

Output (verbatim, zerolog JSON lines removed):

```text
=== RUN   TestVerifyC4
=== RUN   TestVerifyC4/equal-length_a.example.com_vs_*.example.com
    verify_c4_test.go:42: ROUTER gateway-default-my-gateway-ep-websecure-listener-isolation: prio=1 middlewares=[gateway-default-my-gateway-ep-websecure-listener-isolation] rule=PathPrefix("/")
    verify_c4_test.go:42: ROUTER httproute-default-http-app-exact-gw-default-my-gateway-ep-websecure-0-15c1368c5627573ff8ed: prio=100014 middlewares=[gateway-default-my-gateway-listener-exact-isolation] rule=Host("a.example.com") && Path("/exact")
    verify_c4_test.go:42: ROUTER httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-4455cb93ee2056704c79: prio=100014 middlewares=[gateway-default-my-gateway-listener-wildcard-isolation] rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !Host("a.example.com") && Path("/wildcard")
    verify_c4_test.go:46: MIDDLEWARE gateway-default-my-gateway-ep-websecure-listener-isolation: hostname="" siblings=["a.example.com" "*.example.com"]
    verify_c4_test.go:46: MIDDLEWARE gateway-default-my-gateway-listener-exact-isolation: hostname="a.example.com" siblings=["*.example.com"]
    verify_c4_test.go:46: MIDDLEWARE gateway-default-my-gateway-listener-wildcard-isolation: hostname="*.example.com" siblings=["a.example.com"]
    verify_c4_test.go:59: REQ sni=a.example.com      host=a.example.com      path=/exact    -> 421 Misdirected Request
    verify_c4_test.go:59: REQ sni=other.example.com  host=other.example.com  path=/wildcard -> 200 10.10.0.1
    verify_c4_test.go:59: REQ sni=other.example.com  host=a.example.com      path=/exact    -> 421 Misdirected Request
    verify_c4_test.go:59: REQ sni=a.example.com      host=other.example.com  path=/wildcard -> 421 Misdirected Request
    verify_c4_test.go:59: REQ sni=a.example.com      host=a.example.com      path=/nope     -> 421 Misdirected Request
=== RUN   TestVerifyC4/longer_abc.example.com_vs_*.example.com
    verify_c4_test.go:42: ROUTER gateway-default-my-gateway-ep-websecure-listener-isolation: prio=1 middlewares=[gateway-default-my-gateway-ep-websecure-listener-isolation] rule=PathPrefix("/")
    verify_c4_test.go:42: ROUTER httproute-default-http-app-exact-gw-default-my-gateway-ep-websecure-0-cce066dd109c02dd2fb6: prio=100016 middlewares=[gateway-default-my-gateway-listener-exact-isolation] rule=Host("abc.example.com") && Path("/exact")
    verify_c4_test.go:42: ROUTER httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-974b200fd0cffcc10089: prio=100014 middlewares=[gateway-default-my-gateway-listener-wildcard-isolation] rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !Host("abc.example.com") && Path("/wildcard")
    verify_c4_test.go:46: MIDDLEWARE gateway-default-my-gateway-listener-wildcard-isolation: hostname="*.example.com" siblings=["abc.example.com"]
    verify_c4_test.go:46: MIDDLEWARE gateway-default-my-gateway-ep-websecure-listener-isolation: hostname="" siblings=["abc.example.com" "*.example.com"]
    verify_c4_test.go:46: MIDDLEWARE gateway-default-my-gateway-listener-exact-isolation: hostname="abc.example.com" siblings=["*.example.com"]
    verify_c4_test.go:59: REQ sni=abc.example.com    host=abc.example.com    path=/exact    -> 200 10.10.0.1
    verify_c4_test.go:59: REQ sni=other.example.com  host=other.example.com  path=/wildcard -> 200 10.10.0.1
    verify_c4_test.go:59: REQ sni=other.example.com  host=abc.example.com    path=/exact    -> 421 Misdirected Request
    verify_c4_test.go:59: REQ sni=abc.example.com    host=other.example.com  path=/wildcard -> 421 Misdirected Request
    verify_c4_test.go:59: REQ sni=abc.example.com    host=abc.example.com    path=/nope     -> 421 Misdirected Request
--- PASS: TestVerifyC4 (0.11s)
    --- PASS: TestVerifyC4/equal-length_a.example.com_vs_*.example.com (0.09s)
    --- PASS: TestVerifyC4/longer_abc.example.com_vs_*.example.com (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.136s
```

**Reading.** In the equal-length subtest the fully valid request `sni=a.example.com host=a.example.com /exact` returns `421 Misdirected Request`, while the wildcard request `sni=other.example.com host=other.example.com /wildcard` returns 200. In the `longer` subtest the identical request shape for `abc.example.com` returns `200 10.10.0.1`. The only variable is the hostname length relative to the wildcard. The listener-isolation middleware (`pkg/middlewares/gatewayapi/listenerisolation/listener_isolation.go`) selects the owning listener with:

```go
func mostSpecificHostname(sni string, hostnames []string) string {
    var selected string
    for _, hostname := range hostnames {
        if len(hostname) > len(selected) && matchHostname(hostname, sni) {
            selected = hostname
        }
    }
    return selected
}
```

A strict `>` on length makes the result depend on iteration order when the exact hostname and a matching wildcard have the same length; the middleware then concludes that the SNI belongs to the wildcard listener while the Host belongs to the exact listener and emits 421.

Related fixture signal on this branch (same suite run, different hostnames, so not relied on for the verdict):

```text
        --- FAIL: TestEval_GatewayHTTPSListenerDetectMisdirectedRequests/strictTLSOptions=false/wildcard_literal_hostname_collision (0.09s)
            eval_https_test.go:405: 
                	Error Trace:	/home/ubuntu/wt/963e6173/pkg/provider/kubernetes/gateway/eval_https_test.go:405
                	Error:      	Not equal: 
                	            	expected: 404
                	            	actual  : 421
                	Test:       	TestEval_GatewayHTTPSListenerDetectMisdirectedRequests/strictTLSOptions=false/wildcard_literal_hostname_collision
                	Messages:   	unmatched path on literal listener returns 404
```

**Impact.** Any Gateway whose exact listener hostname has the same length as a sibling wildcard (`a.example.com` vs `*.example.com`, `ab.example.org` vs `*.example.org`, ...) rejects every legitimate HTTPS request to the exact listener with 421. That is a hard outage for the exact-hostname service, not a hardening gap.

## C5

**Claim.** On [`evalon/traefik-ga-cbb7ae82`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-cbb7ae82), two overlapping multi-listener Gateways (`gw-a`, `gw-b`, each with `exact=foo.example.com` and `wildcard=*.example.com` listeners) shadow each other's distinct GRPCRoute methods so valid requests do not reach the configured backends.

**Verdict: CONFIRMED.**

Setup (shared by every claim; repeated here so this section is self-contained):

```bash
# Target branch checked out in its own worktree, with the eval fixtures copied byte-exact
# from eval_tests.zip to the paths the assessment provisions them at.
cd ~/wt/cbb7ae82                      # worktree of evalon/traefik-ga-cbb7ae82
cp ~/eval_tests/tests/eval_https_test.go ~/eval_tests/tests/eval_isolation_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/gateway/
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/ > /tmp/evalsuite-cbb7ae82.log 2>&1
```

The probe below is build-tagged `//go:build verify` and reuses the fixture helpers (`gateway.EvalIsoLoadExported`, `evalOneShotProvider`, ...) so the eval fixtures must be present in the package directory when it runs. Production code is untouched; the probe lives in `verify/repro/` on the verify branch and is copied into the package directory only for the run.

Probes: `verify/repro/c5_sibling_gateway_grpc_shadow_test.go`. `TestVerifyC5` is the HTTPS case (`websecure`, strict TLS options false and true); `TestVerifyC5_HTTP` is the same topology on plain HTTP (`web`) to isolate the parent-router mechanism from TLS-option conflicts. `gw-a` carries GRPCRoutes for methods `SayAExact` (exact listener) and `SayAWild` (wildcard listener); `gw-b` carries `SayBExact` and `SayBWild`. Every method is a distinct `PathRegexp` so the four routes never overlap.

```bash
cp verify/repro/c5_sibling_gateway_grpc_shadow_test.go ~/wt/cbb7ae82/pkg/provider/kubernetes/gateway/verify_c5_test.go
cd ~/wt/cbb7ae82 && go test -tags verify -count=1 -run 'TestVerifyC5$' -v ./pkg/provider/kubernetes/gateway/ 2>&1 | grep -v '^{"level":"(info|debug)'
cd ~/wt/cbb7ae82 && go test -tags verify -count=1 -run 'TestVerifyC5_HTTP' -v ./pkg/provider/kubernetes/gateway/ 2>&1 | grep -v '^{"level"'
```

HTTPS output (verbatim; zerolog `error` lines kept, `info`/`debug` lines removed):

```text
=== RUN   TestVerifyC5
=== RUN   TestVerifyC5/strict=false
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-a-exact-gw-default-gw-a-ep-websecure-0-9f547b17a226a9aa759d-exact: prio=60 parents=[listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-a-exact-gw-default-gw-a-ep-websecure-0-9f547b17a226a9aa759d-exact-wrr rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayAExact")
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-a-wild-gw-default-gw-a-ep-websecure-0-fd4d3ad29840206a7003-wildcard: prio=57 parents=[listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-a-wild-gw-default-gw-a-ep-websecure-0-fd4d3ad29840206a7003-wildcard-wrr rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathRegexp("/helloworld\\.Greeter/SayAWild")
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-b-exact-gw-default-gw-b-ep-websecure-0-df5a203d1492b9dd289a-exact: prio=60 parents=[listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-b-exact-gw-default-gw-b-ep-websecure-0-df5a203d1492b9dd289a-exact-wrr rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayBExact")
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-b-wild-gw-default-gw-b-ep-websecure-0-24033c498a2c313cc8b0-wildcard: prio=57 parents=[listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-b-wild-gw-default-gw-b-ep-websecure-0-24033c498a2c313cc8b0-wildcard-wrr rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathRegexp("/helloworld\\.Greeter/SayBWild")
    verify_c5_test.go:47: ROUTER listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e: prio=23 parents=[] eps=[websecure] tlsOptions=listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e svc= rule=Host("foo.example.com")
    verify_c5_test.go:47: ROUTER listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c: prio=23 parents=[] eps=[websecure] tlsOptions=listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c svc= rule=Host("foo.example.com")
    verify_c5_test.go:47: ROUTER listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de: prio=20 parents=[] eps=[websecure] tlsOptions=listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de svc= rule=Host("**.example.com")
    verify_c5_test.go:47: ROUTER listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5: prio=20 parents=[] eps=[websecure] tlsOptions=listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5 svc= rule=Host("**.example.com")
{"level":"error","time":"2026-11-07T00:41:37Z","message":"On EntryPoint \"websecure\", Host \"**.example.com\" is served by multiple routers with different TLS options, default TLSOptions will be applied for the following routers: [listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5@kubernetesgateway listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de@kubernetesgateway]"}
{"level":"error","time":"2026-11-07T00:41:37Z","message":"On EntryPoint \"websecure\", Host \"foo.example.com\" is served by multiple routers with different TLS options, default TLSOptions will be applied for the following routers: [listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c@kubernetesgateway listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e@kubernetesgateway]"}
    verify_c5_test.go:60: REQ sni=foo.example.com  host=foo.example.com  method=SayAExact  -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=foo.example.com  host=foo.example.com  method=SayBExact  -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=bar.example.com  host=bar.example.com  method=SayAWild   -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=bar.example.com  host=bar.example.com  method=SayBWild   -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=foo.example.com  host=foo.example.com  method=SayAWild   -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=bar.example.com  host=foo.example.com  method=SayAExact  -> 404 404 page not found
=== RUN   TestVerifyC5/strict=true
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-a-exact-gw-default-gw-a-ep-websecure-0-9f547b17a226a9aa759d-exact: prio=60 parents=[listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-a-exact-gw-default-gw-a-ep-websecure-0-9f547b17a226a9aa759d-exact-wrr rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayAExact")
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-a-wild-gw-default-gw-a-ep-websecure-0-fd4d3ad29840206a7003-wildcard: prio=57 parents=[listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-a-wild-gw-default-gw-a-ep-websecure-0-fd4d3ad29840206a7003-wildcard-wrr rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathRegexp("/helloworld\\.Greeter/SayAWild")
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-b-exact-gw-default-gw-b-ep-websecure-0-df5a203d1492b9dd289a-exact: prio=60 parents=[listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-b-exact-gw-default-gw-b-ep-websecure-0-df5a203d1492b9dd289a-exact-wrr rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayBExact")
    verify_c5_test.go:47: ROUTER grpcroute-default-grpc-b-wild-gw-default-gw-b-ep-websecure-0-24033c498a2c313cc8b0-wildcard: prio=57 parents=[listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5] eps=[] tlsOptions=<nil> svc=grpcroute-default-grpc-b-wild-gw-default-gw-b-ep-websecure-0-24033c498a2c313cc8b0-wildcard-wrr rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathRegexp("/helloworld\\.Greeter/SayBWild")
    verify_c5_test.go:47: ROUTER listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e: prio=23 parents=[] eps=[websecure] tlsOptions=listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e svc= rule=Host("foo.example.com")
    verify_c5_test.go:47: ROUTER listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c: prio=23 parents=[] eps=[websecure] tlsOptions=listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c svc= rule=Host("foo.example.com")
    verify_c5_test.go:47: ROUTER listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de: prio=20 parents=[] eps=[websecure] tlsOptions=listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de svc= rule=Host("**.example.com")
    verify_c5_test.go:47: ROUTER listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5: prio=20 parents=[] eps=[websecure] tlsOptions=listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5 svc= rule=Host("**.example.com")
{"level":"error","time":"2026-11-07T00:41:37Z","message":"On EntryPoint \"websecure\", Host \"**.example.com\" is served by multiple routers with different TLS options, the fallback to the default TLS options being disabled, the following routers are disabled: [listener-default-wildcard-gw-default-gw-a-ep-websecure-0-fafe8df456e880c101de@kubernetesgateway listener-default-wildcard-gw-default-gw-b-ep-websecure-0-157db1e619577086d5e5@kubernetesgateway]"}
{"level":"error","time":"2026-11-07T00:41:37Z","message":"On EntryPoint \"websecure\", Host \"foo.example.com\" is served by multiple routers with different TLS options, the fallback to the default TLS options being disabled, the following routers are disabled: [listener-default-exact-gw-default-gw-a-ep-websecure-0-5b065c1b439d936b5a0e@kubernetesgateway listener-default-exact-gw-default-gw-b-ep-websecure-0-e69fa44937ce519d906c@kubernetesgateway]"}
    verify_c5_test.go:60: REQ sni=foo.example.com  host=foo.example.com  method=SayAExact  -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=foo.example.com  host=foo.example.com  method=SayBExact  -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=bar.example.com  host=bar.example.com  method=SayAWild   -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=bar.example.com  host=bar.example.com  method=SayBWild   -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=foo.example.com  host=foo.example.com  method=SayAWild   -> 404 404 page not found
    verify_c5_test.go:60: REQ sni=bar.example.com  host=foo.example.com  method=SayAExact  -> 404 404 page not found
--- PASS: TestVerifyC5 (0.14s)
    --- PASS: TestVerifyC5/strict=false (0.12s)
    --- PASS: TestVerifyC5/strict=true (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.177s
```

Plain-HTTP output (verbatim, zerolog JSON lines removed):

```text
=== RUN   TestVerifyC5_HTTP
    verify_c5_test.go:89: ROUTER grpcroute-default-grpc-a-exact-gw-default-gw-a-ep-web-0-7e6672077f3cf9d888c0-exact: prio=60 parents=[listener-default-exact-gw-default-gw-a-ep-web-0-e8130de120e1e5b327b8] eps=[] svc=grpcroute-default-grpc-a-exact-gw-default-gw-a-ep-web-0-7e6672077f3cf9d888c0-exact-wrr rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayAExact")
    verify_c5_test.go:89: ROUTER grpcroute-default-grpc-a-wild-gw-default-gw-a-ep-web-0-52952156d545ae653608-wildcard: prio=57 parents=[listener-default-wildcard-gw-default-gw-a-ep-web-0-eb67e3a78d1832a986a9] eps=[] svc=grpcroute-default-grpc-a-wild-gw-default-gw-a-ep-web-0-52952156d545ae653608-wildcard-wrr rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathRegexp("/helloworld\\.Greeter/SayAWild")
    verify_c5_test.go:89: ROUTER grpcroute-default-grpc-b-exact-gw-default-gw-b-ep-web-0-2cb6421145becbb75f13-exact: prio=60 parents=[listener-default-exact-gw-default-gw-b-ep-web-0-ce594fb1fef261bd304a] eps=[] svc=grpcroute-default-grpc-b-exact-gw-default-gw-b-ep-web-0-2cb6421145becbb75f13-exact-wrr rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayBExact")
    verify_c5_test.go:89: ROUTER grpcroute-default-grpc-b-wild-gw-default-gw-b-ep-web-0-aac3f9fa8a3f851283d8-wildcard: prio=57 parents=[listener-default-wildcard-gw-default-gw-b-ep-web-0-5475f81e5f0a4326f545] eps=[] svc=grpcroute-default-grpc-b-wild-gw-default-gw-b-ep-web-0-aac3f9fa8a3f851283d8-wildcard-wrr rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && PathRegexp("/helloworld\\.Greeter/SayBWild")
    verify_c5_test.go:89: ROUTER listener-default-exact-gw-default-gw-a-ep-web-0-e8130de120e1e5b327b8: prio=23 parents=[] eps=[web] svc= rule=Host("foo.example.com")
    verify_c5_test.go:89: ROUTER listener-default-exact-gw-default-gw-b-ep-web-0-ce594fb1fef261bd304a: prio=23 parents=[] eps=[web] svc= rule=Host("foo.example.com")
    verify_c5_test.go:89: ROUTER listener-default-wildcard-gw-default-gw-a-ep-web-0-eb67e3a78d1832a986a9: prio=20 parents=[] eps=[web] svc= rule=Host("**.example.com")
    verify_c5_test.go:89: ROUTER listener-default-wildcard-gw-default-gw-b-ep-web-0-5475f81e5f0a4326f545: prio=20 parents=[] eps=[web] svc= rule=Host("**.example.com")
    verify_c5_test.go:101: REQ host=foo.example.com  method=SayAExact  -> 200 10.10.0.1
    verify_c5_test.go:101: REQ host=foo.example.com  method=SayBExact  -> 404 404 page not found
    verify_c5_test.go:101: REQ host=bar.example.com  method=SayAWild   -> 404 404 page not found
    verify_c5_test.go:101: REQ host=bar.example.com  method=SayBWild   -> 200 10.10.0.1
    verify_c5_test.go:101: REQ host=foo.example.com  method=SayAWild   -> 404 404 page not found
--- PASS: TestVerifyC5_HTTP (0.08s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.107s
```

**Reading.** The branch models each listener as a root router (`listener-...`, rule `Host("foo.example.com")` or `Host("**.example.com")`, no service) and makes every route router a child via `ParentRefs`. The generated configuration is correct per Gateway (each `grpcroute-*` child has its own Gateway's listener as parent), but the two Gateways produce two root routers with the *same* rule and priority (`prio=23` for `foo.example.com`, `prio=20` for the wildcard). The muxer selects one root for a given Host, and only that root's children are consulted. On plain HTTP this produces the shadowing directly: `host=foo.example.com SayAExact -> 200` but `SayBExact -> 404`, `host=bar.example.com SayBWild -> 200` but `SayAWild -> 404`. On HTTPS the same duplicated roots additionally carry different per-listener TLS options, so the server logs `Host "foo.example.com" is served by multiple routers with different TLS options` and either downgrades (strict=false) or disables (strict=true) both roots; all six requests return 404, including fully valid `SNI=Host` requests for both Gateways.

The fixture that exercises the same scenario (`TestEval_GatewayHTTPSSiblingGatewayReachability`) fails on this branch with the same symptom (expected 200, got 404 for "sibling route reachable with SNI=Host"):

```text
--- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability (0.44s)
    --- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability/strictTLSOptions=false (0.24s)
        --- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability/strictTLSOptions=false/punctuated_and_multi_listener_siblings (0.05s)
            eval_https_test.go:534: 
                	Error Trace:	/home/ubuntu/wt/cbb7ae82/pkg/provider/kubernetes/gateway/eval_https_test.go:534
                	Error:      	Not equal: 
                	            	expected: 200
                	            	actual  : 404
                	Test:       	TestEval_GatewayHTTPSSiblingGatewayReachability/strictTLSOptions=false/punctuated_and_multi_listener_siblings
                	Messages:   	sibling route reachable with SNI=Host
            eval_https_test.go:546: 
                	Error Trace:	/home/ubuntu/wt/cbb7ae82/pkg/provider/kubernetes/gateway/eval_https_test.go:546
                	Error:      	Not equal: 
                	            	expected: 200
                	            	actual  : 404
                	Test:       	TestEval_GatewayHTTPSSiblingGatewayReachability/strictTLSOptions=false/punctuated_and_multi_listener_siblings
                	Messages:   	unreserved host reaches sibling catchall route
            eval_https_test.go:548: 
                	Error Trace:	/home/ubuntu/wt/cbb7ae82/pkg/provider/kubernetes/gateway/eval_https_test.go:548
                	Error:      	Not equal: 
                	            	expected: 200
                	            	actual  : 404
                	Test:       	TestEval_GatewayHTTPSSiblingGatewayReachability/strictTLSOptions=false/punctuated_and_multi_listener_siblings
                	Messages:   	sibling exact host reaches other sibling catchall route
        --- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability/strictTLSOptions=false/namespaced_identity_collisions (0.05s)
```

**Impact.** Two Gateways (e.g. two teams, or two namespaces) that legitimately share a hostname family on the same entry point cannot both serve traffic: one Gateway's routes (HTTP and plain-HTTP gRPC) are unreachable, and over HTTPS both Gateways' routes become unreachable. The isolation implementation turns a supported Gateway API topology (distinct routes under overlapping listeners across Gateways) into an outage rather than isolating only the cross-listener traffic.
