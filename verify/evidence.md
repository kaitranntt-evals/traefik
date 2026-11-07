## C1

Audited branch: `traefik-gateway-listener-isolation-perfect` (HEAD `16bd9c70425e946d64be29de626ab91d7f2a1edd`, base `35c7530ac`). `pkg/provider/kubernetes/gateway/client.go` is unchanged relative to the base commit (`git diff 35c7530ac HEAD -- pkg/provider/kubernetes/gateway/client.go` prints nothing), so the behavior below is inherited, but the solution adds two new features (`GatewayHTTPListenerIsolation`, `GatewayHTTPSListenerDetectMisdirectedRequests`) to `SupportedFeatures()` and relies on this function to advertise them.

Relevant code (`client.go:434-463`): `UpdateGatewayClassStatus` returns early when `conditionsEqual(currentGatewayClass.Status.Conditions, status.Conditions)`; `conditionsEqual` (`client.go:847-855`) compares only Type/Reason/Status/Message/ObservedGeneration. `status.SupportedFeatures` is never compared.

Repro: `verify/repro/c1_gatewayclass_supported_features_test.go` (build tag `verify`, package `gateway`). It seeds a fake `GatewayClass` whose stored status already has the Accepted condition and a stale feature list, then (a) calls `UpdateGatewayClassStatus` directly with the same condition fields but a different feature list, (b) a control where the condition Message differs, (c) the realistic path: `Provider.loadConfigurationFromGateways` + `statusReport.Flush` with the stored list equal to the pre-upgrade feature list (everything in `SupportedFeatures()` minus the two new features) and the stored Accepted condition already matching (ObservedGeneration == Generation == 1).

```sh
cd ~/repos/traefik
cp verify/repro/c1_gatewayclass_supported_features_test.go pkg/provider/kubernetes/gateway/
go test -tags verify -race -count=1 -run '^TestVerifyC1_' -v ./pkg/provider/kubernetes/gateway
```

Key output:

```console
=== RUN   TestVerifyC1_UpdateGatewayClassStatus_FeaturesOnlyChange
    c1_gatewayclass_supported_features_test.go:79: stored SupportedFeatures after UpdateGatewayClassStatus: [GatewayPort8080]
        	Error:      	Not equal:
        	            	expected: []string{"GatewayHTTPListenerIsolation", "GatewayHTTPSListenerDetectMisdirectedRequests", "GatewayPort8080"}
        	            	actual  : []string{"GatewayPort8080"}
        	Messages:   	requested feature list should be persisted
--- FAIL: TestVerifyC1_UpdateGatewayClassStatus_FeaturesOnlyChange (0.50s)
=== RUN   TestVerifyC1_UpdateGatewayClassStatus_ConditionChangeWritesFeatures
    c1_gatewayclass_supported_features_test.go:100: stored SupportedFeatures after UpdateGatewayClassStatus: [GatewayHTTPListenerIsolation GatewayPort8080]
--- PASS: TestVerifyC1_UpdateGatewayClassStatus_ConditionChangeWritesFeatures (0.00s)
=== RUN   TestVerifyC1_ProviderUpgrade_NewFeaturesNotAdvertised
    c1_gatewayclass_supported_features_test.go:130: stored SupportedFeatures after upgrade reconcile (30 entries): [BackendTLSPolicy BackendTLSPolicySANValidation GRPCRoute GRPCRouteNamedRouteRule Gateway GatewayPort8080 HTTPRoute ... TLSRouteModeMixed TLSRouteModeTerminate]
        	Error:      	[...] does not contain "GatewayHTTPListenerIsolation"
        	Error:      	[...] does not contain "GatewayHTTPSListenerDetectMisdirectedRequests"
--- FAIL: TestVerifyC1_ProviderUpgrade_NewFeaturesNotAdvertised (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.598s
```

Reading: with identical condition fields (only `LastTransitionTime` differs) the stored `status.supportedFeatures` keeps the previous list (subtest a); the write only happens when a condition field changes (control b). In the upgrade path (c) the provider's own reconcile leaves the 30-entry pre-upgrade list in place and the two features this solution adds are never advertised on an existing GatewayClass. A fresh GatewayClass (no stored Accepted condition) or any later condition change (e.g. a `generation` bump from editing the GatewayClass spec) does write the full list — that is the workaround.

Impact reasoning: every in-place controller upgrade hits this (GatewayClass objects are long-lived and their `generation` does not change when the controller binary changes). `kubectl get gatewayclass -o yaml` keeps showing the old `supportedFeatures`, so conformance tooling / users reading the status believe the new features are unsupported, while the data plane does enforce them. Nothing is logged.

Verdict: CONFIRMED.

## C2

Claim branch: `evalon/traefik-ga-e2b78df4` from repository `kaitranntt-evals/traefik-gateway-listener-isolation` (HEAD `7c331e3c4b73d5378db7c89b6db87c8cb43c8d36`), fetched as remote `evalrepo` into worktree `~/repos/traefik-c2`:

```sh
cd ~/repos/traefik
git remote add evalrepo https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation
git fetch evalrepo evalon/traefik-ga-e2b78df4
git worktree add ../traefik-c2 evalrepo/evalon/traefik-ga-e2b78df4
```

Eval fixtures (byte-exact copies from `eval_tests.zip`) were copied into `pkg/provider/kubernetes/gateway/` of the worktree (`eval_https_test.go`, `eval_isolation_test.go`, `export_eval_test.go`); the repro reuses their exported helpers (`gateway.EvalIsoLoadExported`, `gateway.EvalIsoHTTPSGatewayExported`, `gateway.EvalIsoRouteExported`, `evalServeHTTPS`).

Relevant code on the claim branch: `kubernetes.go:1251` `loadHTTPSListenerFallbackRouters` adds, for every HTTPS listener with a hostname, a router `Host("<hostname>") && PathPrefix("/")` with priority from `buildMatchRule` (= 1 + len(hostname) = 16 for `abc.example.com`) serving a 404 WRR. `httproute.go:817-841` `buildPathRule` gives `PathPrefix("/")` priority 1, and `httproute.go:141` adds `len(rules)-ri` (=1), so a hostname-less `PathPrefix("/")` route on a sibling Gateway has priority 2.

Repro: `verify/repro/c2_sibling_https_root_prefix_test.go` (build tag `verify`, package `gateway_test`). Two HTTPS Gateways on entry point `websecure`: `gw-exact` with listener `precise` hostname `abc.example.com` and an Exact `/precise` HTTPRoute; `gw-catchall` with a hostname-less listener and a `PathPrefix: /` HTTPRoute to `whoami`. Requests are served through `server.NewRouterFactory`/`CreateRouters` with SNI = Host.

```sh
cd ~/repos/traefik-c2
cp <extracted eval_tests.zip>/tests/*.go pkg/provider/kubernetes/gateway/   # eval_https_test.go eval_isolation_test.go export_eval_test.go
cp ~/repos/traefik/verify/repro/c2_sibling_https_root_prefix_test.go pkg/provider/kubernetes/gateway/
go test -tags verify -race -count=1 -run '^TestVerifyC2_' -v ./pkg/provider/kubernetes/gateway
```

Key output (claim branch):

```console
=== RUN   TestVerifyC2_SiblingHTTPS_RootPrefixSiblingVsExactHostFallback/strict=false
    router gateway-default-gw-exact-listener-precise-ep-websecure: rule=Host("abc.example.com") && PathPrefix("/") priority=16 service=gateway-default-gw-exact-listener-precise-ep-websecure-wrr
    router httproute-default-http-app-precise-gw-default-gw-exact-ep-websecure-0-e0c51859e4ceccb199b1: rule=Host("abc.example.com") && Path("/precise") priority=100016 ...
    router httproute-default-http-app-root-gw-default-gw-catchall-ep-websecure-0-be6b27776417e7216646: rule=PathPrefix("/") priority=2 ...
    abc.example.com /precise -> 200
    xyz.example.com /outside -> 200
    abc.example.com /outside -> 404 body=""
        	Error:      	Not equal:
        	            	expected: 200
        	            	actual  : 404
        	Messages:   	claim C2: expected to reach sibling catch-all PathPrefix(/) backend; 404 means the exact-host fallback won
--- FAIL: TestVerifyC2_SiblingHTTPS_RootPrefixSiblingVsExactHostFallback (0.55s)
    --- FAIL: TestVerifyC2_SiblingHTTPS_RootPrefixSiblingVsExactHostFallback/strict=false (0.51s)
    --- FAIL: TestVerifyC2_SiblingHTTPS_RootPrefixSiblingVsExactHostFallback/strict=true (0.04s)
=== RUN   TestVerifyC2_SiblingHTTPS_ExactSiblingReachable
    abc.example.com /other (Exact sibling route) -> 200
--- PASS: TestVerifyC2_SiblingHTTPS_ExactSiblingReachable (0.07s)
```

Contrast runs:

- Same probe on the audited branch `traefik-gateway-listener-isolation-perfect` (`cd ~/repos/traefik && go test -tags verify -race -count=1 -run '^TestVerifyC2_' -v ./pkg/provider/kubernetes/gateway`): `abc.example.com /outside -> 200 body="10.10.0.1"` (both strict modes), `--- PASS`. That implementation uses per-listener parent routers (`listener-websecure-https-abc-example-com-...: rule=Host("abc.example.com") priority=0`, `listener-websecure-https-...: rule=(Host("*")) && !(Host("abc.example.com"))`) instead of a priority-16 fallback 404 router, so the sibling catch-all route is reached.
- Eval fixture on the claim branch: `go test -race -count=1 -run '^TestEval_GatewayHTTPSSiblingGatewayReachability$' ./pkg/provider/kubernetes/gateway` — subtest `punctuated_and_multi_listener_siblings` (sibling routes are all Exact-path, priority 100000+) PASSES, including `def.example.com /catchall -> 200 "sibling exact host reaches other sibling catchall route"`; only the unrelated `identity_collision_unmatched_path` subtests fail (`expected: 421 actual: 404`). The fixture therefore establishes the intended semantics (a sibling Gateway's catch-all route must stay reachable for another Gateway's exact host) but only exercises Exact-path sibling routes, so it does not catch the `PathPrefix("/")` case.

Reading: on the claim branch, the exact-host listener's fallback 404 router (priority 16) outranks the sibling Gateway's hostname-less `PathPrefix("/")` route (priority 2), so `https://abc.example.com/outside` gets 404 even though a valid route on the sibling Gateway matches it; the identical topology with an Exact-path sibling route (priority 100000) reaches the backend (200). Reachability of a sibling Gateway's route thus depends on the path match type/priority rather than on listener ownership.

Impact reasoning: the trigger is two Gateways of the same class sharing one HTTPS entry point (Traefik merges them), one with a hostname-specific listener and the other with a hostname-less catch-all listener whose route is a `PathPrefix /` default route — the standard "default backend" pattern. Every request to the exact host that does not match that Gateway's own routes is answered 404 by Traefik instead of by the default backend. Workaround: give the catch-all Gateway's route a more specific path match, or move the route onto the exact-host Gateway.

Verdict: CONFIRMED on `evalon/traefik-ga-e2b78df4` (the only branch the claim targets).
