## Setup

Audited branch: `traefik-gateway-listener-isolation-perfect` (checked out as `verify/traefik-gateway-listener-isolation-v-3f78e0cd`). Claim branches were fetched from the `kaitranntt-evals/traefik-gateway-listener-isolation` repository (remote `evalrepo`) into separate worktrees; every claim branch has merge-base `35c7530ac0906b5407a3733de173d58d9c427949` with the eval base.

```sh
cd ~/repos/traefik
git remote add evalrepo https://git-manager.itsdev.in/proxy/github.com/kaitranntt-evals/traefik-gateway-listener-isolation
git fetch evalrepo evalon/traefik-ga-0c78c3fa evalon/traefik-ga-6aded2e6 evalon/traefik-ga-5af6118e evalon/traefik-ga-df753f72
for b in 0c78c3fa 6aded2e6 5af6118e df753f72; do git worktree add ~/wt/$b evalrepo/evalon/traefik-ga-$b; done
```

Probe files live in `verify/repro/` with a `//go:build verify` tag; each is copied into `pkg/provider/kubernetes/gateway/` of the target worktree and run with `go test -tags verify`.

## C1

Branches: `evalon/traefik-ga-0c78c3fa` and `evalon/traefik-ga-6aded2e6`. Claim: the authored tests do not prove that an identified broader-listener backend route excludes traffic for a more specific listener hostname.

### Authored tests inspected

`0c78c3fa` (`git diff 35c7530ac..HEAD -- pkg/provider/kubernetes/gateway/*_test.go`): `TestLoadHTTPRoutes_listenerIsolation`, `TestLoadGRPCRoutes_listenerIsolation`, `Test_isolateHostnames`, `Test_isMoreSpecificHostname`, `TestSupportedFeatures_listenerIsolation`, plus new cases in `Test_buildMatchRule` / `Test_buildGRPCMatchRule`. The load-level test discards router identity before asserting:

```go
rules := make(map[string]string)
for _, router := range conf.HTTP.Routers {
    ...
    rules[router.Rule] = tlsOptions
}
assert.Equal(t, test.expectedRules, rules)
```

`6aded2e6`: `TestLoadRoutes_listenerIsolation`, `Test_isMoreSpecificHostname`, `Test_isolateHostnames`, `Test_isolateListeners`, `TestSupportedFeatures_listenerIsolation`, `Test_buildHostExclusionRule`, `Test_joinRules`. The load-level test keeps only `router{EntryPoint, Rule, TLSOptions}`:

```go
routers = append(routers, router{EntryPoint: r.EntryPoints[0], Rule: r.Rule, TLSOptions: tlsOptions})
...
assert.ElementsMatch(t, test.expectedRouters, routers)
```

`Test_isolateListeners` (6aded2e6) asserts identified routers, but only the listener fallback routers (`makeListenerRouterName(...)`, service `listener-not-found`, priority 1), not backend routes. No authored test on either branch builds a muxer or sends a request. Fallback-generated rejection: the fallback routers have priority 1 while every backend route has priority ≥ 2 (see priorities below), so no fallback can outrank a backend route; the exclusion is only effective through the `!Host(...)` text of the backend route rule.

### Baseline: authored tests pass unmodified

```sh
cd ~/wt/0c78c3fa && go test -count=1 -race -timeout=5m -run 'TestLoadHTTPRoutes_listenerIsolation|TestLoadGRPCRoutes_listenerIsolation|Test_isolateHostnames|Test_isMoreSpecificHostname|TestSupportedFeatures_listenerIsolation|Test_buildMatchRule|Test_buildGRPCMatchRule' ./pkg/provider/kubernetes/gateway/
cd ~/wt/6aded2e6 && go test -count=1 -race -timeout=5m -run 'TestLoadRoutes_listenerIsolation|Test_isMoreSpecificHostname|Test_isolateHostnames|Test_isolateListeners|TestSupportedFeatures_listenerIsolation|Test_buildHostExclusionRule|Test_joinRules' ./pkg/provider/kubernetes/gateway/
```

```console
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.567s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.586s
```

### Mutation: swap the rule of the broader route with the narrower route

`verify/repro/c1_mutate.sh` appends, at the end of `loadConfigurationFromGateways`, a block that swaps the `Rule` of the `web` router whose rule starts with `Host("**.example.com") &&` with the one starting with `Host("abc.foo.example.com") &&` (the resulting diffs are `verify/repro/c1_mutation_0c78c3fa.diff` and `verify/repro/c1_mutation_6aded2e6.diff`). The multiset of rules is unchanged; only which HTTPRoute carries which rule changes. `verify/repro/c1_identity_probe_test.go` loads the branch's own `httproute/with_listener_isolation.yml`, builds a muxer with all `web` routers and reports which router (named after the HTTPRoute) serves `GET http://abc.foo.example.com/`.

#### 0c78c3fa

```sh
cd ~/wt/0c78c3fa && cp ~/repos/traefik/verify/repro/c1_identity_probe_test.go pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerifyC1' ./pkg/provider/kubernetes/gateway/      # baseline
sh ~/repos/traefik/verify/repro/c1_mutate.sh
go test -tags verify -count=1 -race -v -run 'TestLoadHTTPRoutes_listenerIsolation|TestLoadGRPCRoutes_listenerIsolation|Test_isolateHostnames|Test_isMoreSpecificHostname|TestSupportedFeatures_listenerIsolation|Test_buildMatchRule|Test_buildGRPCMatchRule' ./pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerifyC1' ./pkg/provider/kubernetes/gateway/      # under mutation
git checkout -- pkg/provider/kubernetes/gateway/kubernetes.go
```

```console
### baseline probe (unmodified branch)
    web router httproute-default-empty-gw-default-my-gateway-ep-web-0-b6f365654203081e5b5d priority=2 rule=!Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && PathPrefix("/")
    web router httproute-default-wildcard-example-com-gw-default-my-gateway-ep-web-0-d425f485e7c95ffe4274 priority=15 rule=Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && PathPrefix("/")
    web router httproute-default-wildcard-foo-example-com-gw-default-my-gateway-ep-web-0-c37f25a429ece14d243e priority=19 rule=Host("**.foo.example.com") && !Host("abc.foo.example.com") && PathPrefix("/")
    web router httproute-default-with-hostnames-gw-default-my-gateway-ep-web-0-04c2c5776c8fa19cc548 priority=17 rule=Host("bar.example.com") && PathPrefix("/")
    web router httproute-default-abc-foo-example-com-gw-default-my-gateway-ep-web-0-6ab13e274f1a405cb63d priority=21 rule=Host("abc.foo.example.com") && PathPrefix("/")
    GET http://abc.foo.example.com/ -> router "httproute-default-abc-foo-example-com-gw-default-my-gateway-ep-web-0-6ab13e274f1a405cb63d"
    GET http://bar.foo.example.com/ -> router "httproute-default-wildcard-foo-example-com-gw-default-my-gateway-ep-web-0-c37f25a429ece14d243e"
--- PASS: TestVerifyC1_requestIdentity (0.06s)
### apply mutation
440:	// VERIFY C1 MUTATION: swap broader/narrower route rules on web.
 pkg/provider/kubernetes/gateway/kubernetes.go | 18 ++++++++++++++++++
### authored tests under mutation
--- PASS: Test_buildGRPCMatchRule (0.00s)
--- PASS: Test_buildMatchRule (0.00s)
--- PASS: TestLoadHTTPRoutes_listenerIsolation (0.00s)
--- PASS: Test_isolateHostnames (0.00s)
--- PASS: Test_isMoreSpecificHostname (0.00s)
--- PASS: TestSupportedFeatures_listenerIsolation (0.00s)
--- PASS: TestLoadGRPCRoutes_listenerIsolation (0.03s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.615s
### probe under mutation
    web router httproute-default-abc-foo-example-com-gw-default-my-gateway-ep-web-0-6ab13e274f1a405cb63d priority=21 rule=Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && PathPrefix("/")
    web router httproute-default-wildcard-example-com-gw-default-my-gateway-ep-web-0-d425f485e7c95ffe4274 priority=15 rule=Host("abc.foo.example.com") && PathPrefix("/")
    GET http://abc.foo.example.com/ -> router "httproute-default-wildcard-example-com-gw-default-my-gateway-ep-web-0-d425f485e7c95ffe4274"
    Error:      	"httproute-default-wildcard-example-com-gw-default-my-gateway-ep-web-0-d425f485e7c95ffe4274" does not contain "abc-foo-example-com"
--- FAIL: TestVerifyC1_requestIdentity (0.06s)
```

#### 6aded2e6

```sh
cd ~/wt/6aded2e6 && cp ~/repos/traefik/verify/repro/c1_identity_probe_test.go pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerifyC1' ./pkg/provider/kubernetes/gateway/      # baseline
sh ~/repos/traefik/verify/repro/c1_mutate.sh
go test -tags verify -count=1 -race -v -run 'TestLoadRoutes_listenerIsolation|Test_isMoreSpecificHostname|Test_isolateHostnames|Test_isolateListeners|TestSupportedFeatures_listenerIsolation|Test_buildHostExclusionRule|Test_joinRules' ./pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerifyC1' ./pkg/provider/kubernetes/gateway/      # under mutation
git checkout -- pkg/provider/kubernetes/gateway/kubernetes.go
```

```console
### baseline probe (unmodified branch)
    web router httproute-default-http-app-empty-gw-default-my-gateway-ep-web-0-be7dc29e461d783fa13a priority=2 rule=PathPrefix("/") && !Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com")
    web router httproute-default-http-app-wildcard-foo-example-com-gw-default-my-gateway-ep-web-0-dcc9f0534db0b4918bf4 priority=19 rule=Host("**.foo.example.com") && PathPrefix("/") && !Host("abc.foo.example.com")
    web router httproute-default-http-app-abc-foo-example-com-gw-default-my-gateway-ep-web-0-3b12bfd3816eb3ab0d46 priority=21 rule=Host("abc.foo.example.com") && PathPrefix("/")
    web router httproute-default-http-app-wildcard-example-com-gw-default-my-gateway-ep-web-0-1aa2fe98f8d361efcf4e priority=15 rule=Host("**.example.com") && PathPrefix("/") && !Host("**.foo.example.com") && !Host("abc.foo.example.com")
    GET http://abc.foo.example.com/ -> router "httproute-default-http-app-abc-foo-example-com-gw-default-my-gateway-ep-web-0-3b12bfd3816eb3ab0d46"
    GET http://bar.foo.example.com/ -> router "httproute-default-http-app-wildcard-foo-example-com-gw-default-my-gateway-ep-web-0-dcc9f0534db0b4918bf4"
--- PASS: TestVerifyC1_requestIdentity (0.06s)
### apply mutation
441:	// VERIFY C1 MUTATION: swap broader/narrower route rules on web.
 pkg/provider/kubernetes/gateway/kubernetes.go | 18 ++++++++++++++++++
### authored tests under mutation
--- PASS: TestSupportedFeatures_listenerIsolation (0.00s)
--- PASS: Test_buildHostExclusionRule (0.00s)
--- PASS: Test_joinRules (0.00s)
--- PASS: TestLoadRoutes_listenerIsolation (0.00s)
--- PASS: Test_isMoreSpecificHostname (0.00s)
--- PASS: Test_isolateHostnames (0.00s)
--- PASS: Test_isolateListeners (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.585s
### probe under mutation
    web router httproute-default-http-app-wildcard-example-com-gw-default-my-gateway-ep-web-0-1aa2fe98f8d361efcf4e priority=15 rule=Host("abc.foo.example.com") && PathPrefix("/")
    web router httproute-default-http-app-abc-foo-example-com-gw-default-my-gateway-ep-web-0-3b12bfd3816eb3ab0d46 priority=21 rule=Host("**.example.com") && PathPrefix("/") && !Host("**.foo.example.com") && !Host("abc.foo.example.com")
    GET http://abc.foo.example.com/ -> router "httproute-default-http-app-wildcard-example-com-gw-default-my-gateway-ep-web-0-1aa2fe98f8d361efcf4e"
    Error:      	"httproute-default-http-app-wildcard-example-com-gw-default-my-gateway-ep-web-0-1aa2fe98f8d361efcf4e" does not contain "abc-foo-example-com"
--- FAIL: TestVerifyC1_requestIdentity (0.06s)
```

### Reasoning

On both branches every authored test stays green while the broader-listener HTTPRoute (`wildcard-example-com` / `http-app-wildcard-example-com`) serves the more-specific listener's hostname `abc.foo.example.com` with the path `/` that the broader route otherwise matches. No authored assertion connects the `!Host(...)` exclusion to an identified backend route, and no request-level (forbidden fallthrough) assertion exists. Verdict: CONFIRMED on `evalon/traefik-ga-0c78c3fa` and CONFIRMED on `evalon/traefik-ga-6aded2e6`.

## C2

Branch: `evalon/traefik-ga-5af6118e`. Claim: adding an unrelated reserved listener hostname to Gateway A raises its hostname-free gRPC route priority enough to steal `foo.example.com` traffic from Gateway B's hostname-specific route.

### Part 1 - rule construction (grpcroute.go:481-500)

```go
func buildGRPCMatchRule(hostnames, excludedHostnames []gatev1.Hostname, match gatev1.GRPCRouteMatch) (string, int) {
	var matchRules []string
	if excludedHostRule := buildExcludedHostRule(excludedHostnames); excludedHostRule != "" {
		matchRules = append(matchRules, excludedHostRule)
	}
	...
	matchRulesStr := strings.Join(matchRules, " && ")
	hostRule, priority := buildHostRule(hostnames)
	if hostRule == "" {
		return matchRulesStr, len(matchRulesStr)
	}
	return hostRule + " && " + matchRulesStr, priority + len(matchRulesStr)
}
```

`isolateListeners` (kubernetes.go:677-699) is called per Gateway, so Gateway A's exclusions only contain Gateway A's own listener hostnames.

Observed via `verify/repro/c2_sibling_gateway_grpc_test.go` (direct call of `buildGRPCMatchRule`):

```console
construction: no exclusion  -> rule="PathPrefix(\"/\")" priority=15 len(rule)=15
construction: with exclusion -> rule="!Host(\"reserved-listener-hostname-unrelated-to-foo-example-com.internal.example.org\") && PathPrefix(\"/\")" priority=104 len(rule)=104
```

The exclusion text is counted in the priority (priority == len(rule) in both cases). Part 1 holds.

### Part 2 - request selection across two sibling Gateways

Fixture (inline in the probe): Gateway `gateway-a` with hostname-free HTTP listener `catchall` (second case adds listener `reserved` with hostname `reserved-listener-hostname-unrelated-to-foo-example-com.internal.example.org`), Gateway `gateway-b` with listener `exact` hostname `foo.example.com`, GRPCRoute `grpc-a-anyhost` -> gateway-a/catchall and GRPCRoute `grpc-b-foo` -> gateway-b/exact, both matching `helloworld.Greeter/SayHello`; both on entryPoint `web`. All routers are loaded into one `httpmuxer.Muxer` with their provider-computed priorities and the request is dispatched through `requestdecorator`.

```sh
cd ~/wt/5af6118e && cp ~/repos/traefik/verify/repro/c2_sibling_gateway_grpc_test.go pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerifyC2' ./pkg/provider/kubernetes/gateway/
```

```console
=== RUN   TestVerifyC2_siblingGatewayGRPCPriority/gateway-a-without-reserved-listener
    router grpcroute-default-grpc-a-anyhost-gw-default-gateway-a-ep-web-0-a810b4f7ca18a3a81b24 priority=44 rule=PathRegexp("/helloworld\\.Greeter/SayHello")
    router grpcroute-default-grpc-b-foo-gw-default-gateway-b-ep-web-0-e66008e1bf64c849b3de priority=59 rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayHello")
    request POST http://foo.example.com/helloworld.Greeter/SayHello -> router "grpcroute-default-grpc-b-foo-gw-default-gateway-b-ep-web-0-e66008e1bf64c849b3de"
=== RUN   TestVerifyC2_siblingGatewayGRPCPriority/gateway-a-with-reserved-listener
    router grpcroute-default-grpc-a-anyhost-gw-default-gateway-a-ep-web-0-a81bba86fbbf4bf6f3bf priority=133 rule=!Host("reserved-listener-hostname-unrelated-to-foo-example-com.internal.example.org") && PathRegexp("/helloworld\\.Greeter/SayHello")
    router grpcroute-default-grpc-b-foo-gw-default-gateway-b-ep-web-0-e66008e1bf64c849b3de priority=59 rule=Host("foo.example.com") && PathRegexp("/helloworld\\.Greeter/SayHello")
    request POST http://foo.example.com/helloworld.Greeter/SayHello -> router "grpcroute-default-grpc-a-anyhost-gw-default-gateway-a-ep-web-0-a81bba86fbbf4bf6f3bf"
        	Error:      	"grpcroute-default-grpc-a-anyhost-gw-default-gateway-a-ep-web-0-a81bba86fbbf4bf6f3bf" does not contain "grpc-b-foo"
        	Messages:   	foo.example.com must stay on Gateway B's hostname-specific route
--- FAIL: TestVerifyC2_siblingGatewayGRPCPriority (0.05s)
    --- PASS: TestVerifyC2_siblingGatewayGRPCPriority/gateway-a-without-reserved-listener (0.05s)
    --- FAIL: TestVerifyC2_siblingGatewayGRPCPriority/gateway-a-with-reserved-listener (0.00s)
```

Before the reserved listener is added, Gateway B's route (priority 59) wins; after, Gateway A's hostname-free route (priority 133 > 59) answers the request for `foo.example.com`. Part 2 holds.

Context: the eval fixture's single-Gateway case stays green on this branch, so it does not cover the two-Gateway interaction:

```sh
cd ~/wt/5af6118e && cp ~/attachments/.../eval_tests/tests/*.go pkg/provider/kubernetes/gateway/
go test -count=1 -race -timeout=5m -v -run '^TestEval_GatewayGRPCRouteListenerIsolation$' ./pkg/provider/kubernetes/gateway
```

```console
--- PASS: TestEval_GatewayGRPCRouteListenerIsolation (0.49s)
    --- PASS: TestEval_GatewayGRPCRouteListenerIsolation/method_isolation (0.48s)
    --- PASS: TestEval_GatewayGRPCRouteListenerIsolation/precedence_stable_under_exclusions (0.01s)
```

### Impact reasoning

Any two Gateways sharing an entryPoint where one has a hostname-free listener: adding a listener with a long hostname to the hostname-free Gateway (an unrelated, routine change) silently re-routes another Gateway's hostname-specific gRPC traffic to the hostname-free route. The configuration looks correct (both routers exist with sensible rules) but the priority ordering is inverted. Verdict: CONFIRMED (both parts).

## C3

Branch: `evalon/traefik-ga-df753f72`. Claim: the hostname-intersection case of `TestLoadHTTPRoutes_listenerIsolation` passes when its expected shadowed status record is absent because the Accepted=True assertion is conditional on the record's presence.

### Assertion text (kubernetes_test.go:10535-10541)

```go
// A shadowed route is accepted even though no request can reach it.
if routeStatus, ok := statusReport.httpRoutes[ktypes.NamespacedName{Namespace: "default", Name: "shadowed"}]; ok {
	require.Len(t, routeStatus.Parents, 1)
	accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gatev1.RouteConditionAccepted))
	require.NotNil(t, accepted)
	assert.Equal(t, metav1.ConditionTrue, accepted.Status)
}
```

The fixture `httproute/with_listener_isolation_hostname_intersection.yml` declares HTTPRoute `shadowed` (parent `my-gateway`/`empty-hostname`, hostname `*.example.com`, path `/shadowed`); it produces no router (`require.Len(t, conf.HTTP.Routers, 4)` passes) so the status record is the only place it is observable. The other fixture of the test (`with_listener_isolation.yml`) has no `shadowed` route (`grep -c shadowed` = 0), so the block is already dead for that case.

### Normal run

```sh
cd ~/wt/df753f72 && go test -count=1 -race -timeout=5m -v -run 'TestLoadHTTPRoutes_listenerIsolation' ./pkg/provider/kubernetes/gateway/
```

```console
=== RUN   TestLoadHTTPRoutes_listenerIsolation/Intersecting_hostnames_on_the_listeners_and_the_routes
--- PASS: TestLoadHTTPRoutes_listenerIsolation (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.544s
```

### Replay with the shadowed status record removed

`verify/repro/c3_shadowed_status_conditional_test.go` loads the same fixture, copies the authored assertions verbatim, and in the second subtest deletes `statusReport.httpRoutes[default/shadowed]` before the assertions run.

```sh
cd ~/wt/df753f72 && cp ~/repos/traefik/verify/repro/c3_shadowed_status_conditional_test.go pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerifyC3' ./pkg/provider/kubernetes/gateway/
```

```console
=== RUN   TestVerifyC3_shadowedStatusRecordRemoved/normal_output
    shadowed status record present before mutation: true (httpRoutes keys: 5)
    Accepted=True assertion block executed: true
=== RUN   TestVerifyC3_shadowedStatusRecordRemoved/shadowed_status_record_removed
    shadowed status record present before mutation: true (httpRoutes keys: 5)
    deleted shadowed status record; httpRoutes keys now: 4
    Accepted=True assertion block executed: false
--- PASS: TestVerifyC3_shadowedStatusRecordRemoved (0.07s)
    --- PASS: TestVerifyC3_shadowedStatusRecordRemoved/normal_output (0.07s)
    --- PASS: TestVerifyC3_shadowedStatusRecordRemoved/shadowed_status_record_removed (0.00s)
```

With the record removed, every authored assertion still passes and the Accepted=True block is not executed. Verdict: CONFIRMED.
