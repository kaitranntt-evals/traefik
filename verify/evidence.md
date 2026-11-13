# Evidence — traefik-gateway-listener-isolation, run v-4e8422d7

Audited branch `traefik-gateway-listener-isolation-perfect` at `8bc82b7eb` (C2, C3, C4). Claim branches from
`kaitranntt-evals/traefik-gateway-listener-isolation`: `evalon/traefik-ga-c0294d6d` @ `670941244`,
`evalon/traefik-ga-a9cce5a8` @ `979b7ddff`, `evalon/traefik-ga-745c63cb` @ `b408e89c8` (C1),
`evalon/traefik-ga-0d524d9c` @ `d9701b00a` (C5). Go 1.26.3, linux/amd64.

Setup used by every section (fixtures are the byte-exact files of `eval_tests.zip`):

```sh
cd ~/repos/traefik
git fetch origin traefik-gateway-listener-isolation-perfect
git checkout -b verify/traefik-gateway-listener-isolation-v-4e8422d7 origin/traefik-gateway-listener-isolation-perfect
git remote add evalrepo https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation
for b in c0294d6d a9cce5a8 745c63cb 0d524d9c; do
  git fetch evalrepo evalon/traefik-ga-$b && git worktree add --detach ~/repos/wt-$b FETCH_HEAD
done
unzip ~/attachments/*/eval_tests.zip -d ~/fx
cp ~/fx/tests/*.go pkg/provider/kubernetes/gateway/
```

Baseline on the audited branch with the fixtures in place:

```console
$ go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	5.413s
$ go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	6.129s
```

## C1

Method: `verify/repro/c1/run_c1.sh` copies `verify_omit.go.txt` into the claim-branch worktree as
`pkg/provider/kubernetes/gateway/verify_omit.go` and inserts one call, `verifyOmit(conf, statusReport)`, right before
`return conf, statusReport, nil` in `loadConfigurationFromGateways`. With `VERIFY_OMIT=<record>` the hook deletes exactly
that one record from the provider output (map delete for a router / service / TLS option; order-preserving removal for a
route-status condition) and nothing else. The package test binary is built once and run once per omitted record over the
**whole package** (`-test.run .`), so "PASS" means no test anywhere in the package noticed the omission. Without records
the script dumps the generated records and runs the unmutated baseline. Worktrees were restored afterwards
(`git checkout -- . && rm verify_omit.go`).

### evalon/traefik-ga-c0294d6d

Test-side lookup (`listener_test.go:245-255`, the fake service manager used by `TestListenerIsolation`,
`TestHTTPSListenerDetectMisdirectedRequests`, `TestHTTPSListenerTLSOptionsFallback`):

```go
func (s isolationTestServices) BuildHTTP(_ context.Context, name string) (http.Handler, error) {
	return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("X-Service", name)
		if service := s.conf.Services[name]; service != nil && service.Weighted != nil {
			if status := service.Weighted.Services[0].Status; status != nil {
				rw.WriteHeader(*status)
				return
			}
		}
		rw.WriteHeader(http.StatusOK)
	}), nil
}
```

The positive expectations are `assert.Equal(t, http.StatusOK, response.Code)` and
`assert.Contains(t, response.Header().Get("X-Service"), "-"+test.route+"-")` (e.g. host `api.foo.example.com`, route
`exact`). `X-Service` is the name the *router* references, so it is set even when the service record does not exist.

```console
$ verify/repro/c1/run_c1.sh ~/repos/wt-c0294d6d '.' | head -1
BASELINE PASS
$ verify/repro/c1/run_c1.sh ~/repos/wt-c0294d6d '.' \
    service:httproute-default-exact-gw-default-gateway-ep-web-0-fe066d5c4d373a14c1b8-wrr \
    service:grpcroute-default-wildcard-gw-default-gateway-ep-web-0-f08df9b0191ab37ebf3a-wrr \
    router:httproute-default-exact-gw-default-gateway-ep-web-0-fe066d5c4d373a14c1b8
PASS  omit=service:httproute-default-exact-gw-default-gateway-ep-web-0-fe066d5c4d373a14c1b8-wrr
PASS  omit=service:grpcroute-default-wildcard-gw-default-gateway-ep-web-0-f08df9b0191ab37ebf3a-wrr
FAIL  omit=router:httproute-default-exact-gw-default-gateway-ep-web-0-fe066d5c4d373a14c1b8  (13 failing (sub)tests;  Error: Not equal: )
```

A sweep over every generated record of the three listener tests (same script, run regex
`TestListenerIsolation|TestHTTPSListenerDetectMisdirectedRequests|TestHTTPSListenerTLSOptionsFallback`) gave PASS for
every `service:*-wrr` and `service:*-svc-default-whoami-0` record and FAIL for every route router. The router control
shows the hook is effective; the service omissions show that the route service that must answer the asserted 200 can be
absent from the provider output without any test failing.

Observation: the omission of the generated route service is undetected on this branch.

### evalon/traefik-ga-a9cce5a8

Test-side lookup (`kubernetes_test.go`, `newTestEntryPoint`, used by `TestLoadRoutes_listenerIsolation`,
`TestLoadHTTPRoutes_detectMisdirectedRequests`, `...ConflictingTLSOptions`):

```go
status := http.StatusOK
if service, ok := conf.HTTP.Services[router.Service]; ok && service.Weighted != nil && len(service.Weighted.Services) == 1 && service.Weighted.Services[0].Status != nil {
	status = *service.Weighted.Services[0].Status
}
```

Positive expectation: `assert.Equal(t, http.StatusOK, status, ...)` plus a router-name prefix check. A missing service
falls through to the default `http.StatusOK`.

```console
$ verify/repro/c1/run_c1.sh ~/repos/wt-a9cce5a8 '.' | head -1
BASELINE PASS
$ verify/repro/c1/run_c1.sh ~/repos/wt-a9cce5a8 '.' \
    service:httproute-default-attaches-to-abc-foo-example-com-gw-default-http-listener-isolation-ep-web-0-edd72e8329996f2fa54e-wrr \
    service:grpcroute-default-attaches-to-empty-hostname-gw-default-grpc-listener-isolation-ep-web-0-825f58ccf497e9af8f22-wrr \
    service:httproute-default-https-listener-detect-misdirected-requests-test-1-gw-default-same-namespace-with-https-listener-ep-websecure-0-f1a1ed9d9fe13d15a9b3-wrr \
    router:httproute-default-attaches-to-abc-foo-example-com-gw-default-http-listener-isolation-ep-web-0-edd72e8329996f2fa54e
PASS  omit=service:httproute-default-attaches-to-abc-foo-example-com-gw-default-http-listener-isolation-ep-web-0-edd72e8329996f2fa54e-wrr
PASS  omit=service:grpcroute-default-attaches-to-empty-hostname-gw-default-grpc-listener-isolation-ep-web-0-825f58ccf497e9af8f22-wrr
PASS  omit=service:httproute-default-https-listener-detect-misdirected-requests-test-1-gw-default-same-namespace-with-https-listener-ep-websecure-0-f1a1ed9d9fe13d15a9b3-wrr
FAIL  omit=router:httproute-default-attaches-to-abc-foo-example-com-gw-default-http-listener-isolation-ep-web-0-edd72e8329996f2fa54e  (2 failing (sub)tests;  Error: Not equal: )
```

Sweep over all generated records of the three tests: every route `-wrr` / `-svc-` service omission PASSed; route
routers and TLS options FAILed; two of the four `*-fallback-wrr` (421) services FAILed (those carry an explicit
`Status` the handler reads), the other two fallback services and three fallback routers PASSed.

Observation: the omission of a generated route service is undetected on this branch.

### evalon/traefik-ga-745c63cb

Positional assertions (`listener_isolation_test.go:159-166`, `TestListenerIsolationProviderConfiguration`):

```go
for _, routeStatus := range report.httpRoutes {
	require.Len(t, routeStatus.Parents, 1)
	assert.Equal(t, metav1.ConditionTrue, routeStatus.Parents[0].Conditions[0].Status)
}
for _, routeStatus := range report.grpcRoutes {
	require.Len(t, routeStatus.Parents, 1)
	assert.Equal(t, metav1.ConditionTrue, routeStatus.Parents[0].Conditions[0].Status)
}
```

Production builds the slice as `[]metav1.Condition{acceptedCondition}` then appends `ResolvedRefs`
(`httproute.go:95-97`). Generated records for this test (baseline dump):

```console
$ verify/repro/c1/run_c1.sh ~/repos/wt-745c63cb 'TestListenerIsolationProviderConfiguration'
BASELINE PASS
cond:grpc:default/grpc:Accepted=True
cond:grpc:default/grpc:ResolvedRefs=True
cond:http:default/http:Accepted=True
cond:http:default/http:ResolvedRefs=True
router:grpcroute-default-grpc-gw-default-gw-ep-web-0-cce69c4f1b5547d976b1
router:httproute-default-http-gw-default-gw-ep-web-0-48a3559a52c1fceb8a59
router:https-listener-fallback-default-gw-gw-default-gw-ep-web-0-837a18b71fe4a3920c72
service:grpcroute-default-grpc-gw-default-gw-ep-web-0-cce69c4f1b5547d976b1-wrr
service:httproute-default-http-gw-default-gw-ep-web-0-48a3559a52c1fceb8a59-wrr
service:https-listener-fallback-default-gw-gw-default-gw-ep-web-0-837a18b71fe4a3920c72
```

```console
$ verify/repro/c1/run_c1.sh ~/repos/wt-745c63cb '.' | head -1
BASELINE PASS
$ verify/repro/c1/run_c1.sh ~/repos/wt-745c63cb '.' cond:http:default/http:Accepted cond:grpc:default/grpc:Accepted \
    cond:http:default/http:ResolvedRefs router:httproute-default-http-gw-default-gw-ep-web-0-48a3559a52c1fceb8a59
PASS  omit=cond:http:default/http:Accepted
PASS  omit=cond:grpc:default/grpc:Accepted
PASS  omit=cond:http:default/http:ResolvedRefs
FAIL  omit=router:httproute-default-http-gw-default-gw-ep-web-0-48a3559a52c1fceb8a59  (3 failing (sub)tests;  Error: Not equal: )
```

With `Accepted` removed, `ResolvedRefs=True` becomes `Conditions[0]` and satisfies the `ConditionTrue` check; no other
assertion in the package looks at this route's conditions. (In the same test, omitting any of the three generated
services and the `https-listener-fallback` router also PASSed.)

Observation: the omission of the `Accepted` route-status condition is undetected on this branch.

## C2

Probe: `verify/repro/verify_tlsconflict_probe_test.go.txt` (`TestVerify_C2_ExternalWildcards`). It reuses the fixtures'
own runtime (`evalTLSConflictRuntimeFor`, i.e. the production aggregator, TLS manager, router managers, SNI lookup and
SNICheck) with: Gateway `verify-gw`, one HTTPS listener `broad=*.example.com`, a hostname-free HTTPRoute on `Path /gw`,
and a file-provider router `ext` (priority 100, `Path("/other")`, `tls: {}` or `tls.options: custom`) whose Host rule is
the wildcard under test. It prints every router after aggregation and each request's SNI-selected TLS options and status.

```console
$ cp verify/repro/verify_tlsconflict_probe_test.go.txt pkg/provider/kubernetes/gateway/verify_tlsconflict_probe_test.go
$ go test -tags verify -count=1 -v -run 'TestVerify_' ./pkg/provider/kubernetes/gateway
--- FAIL: TestVerify_C2_ExternalWildcards (0.35s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=*.example.com/default-options (0.08s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=**.foo.example.com/default-options (0.03s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=*.example.com/custom-options (0.03s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=**.foo.example.com/custom-options (0.06s)
    --- PASS: TestVerify_C2_ExternalWildcards/ext=**.example.com/default-options (0.08s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=*.foo.example.com/default-options (0.03s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=both-wildcards/default-options (0.03s)
    --- PASS: TestVerify_C2_ExternalWildcards/control/ext=foo.example.com/default-options (0.01s)
```

### Part 1 — wildcard conflict handling

Routers after production aggregation (no `websecure-conflicted-` copy, two different resolved TLS options kept):

```console
VERIFY[ext=*.example.com/default-options] router ext@file rule=Host(`*.example.com`) && Path(`/other`) prio=100 parents=[] tls.options="" resolved="default" conflicting=false status=enabled err=[]
VERIFY[ext=*.example.com/default-options] router listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") prio=20 parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" conflicting=false status=enabled err=[]
VERIFY[ext=**.foo.example.com/default-options] router ext@file rule=Host(`**.foo.example.com`) && Path(`/other`) prio=100 parents=[] tls.options="" resolved="default" conflicting=false status=enabled err=[]
VERIFY[ext=**.foo.example.com/default-options] router listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") prio=20 parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" conflicting=false status=enabled err=[]
```

Control with an exact external host — the conflict is detected, both routers are renamed and fall back to default:

```console
VERIFY[control/ext=foo.example.com/default-options] router websecure-conflicted-ext@file rule=Host(`foo.example.com`) && Path(`/other`) prio=100 parents=[] tls.options="" resolved="default" conflicting=false status=enabled err=[]
VERIFY[control/ext=foo.example.com/default-options] router websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") prio=20 parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default" conflicting=false status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
```

Source consistent with this: `findConflictingRouters` groups by literal domain string; `resolveParentChildDomains`
only adds child / root domains for which `!isBroadDomain(...)`, so a wildcard external domain never joins the listener
router's conflict inputs unless the string is identical (`**.example.com`, which PASSes above).

### Part 2 — HTTPS runtime trigger

```console
VERIFY[ext=*.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[ext=*.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="aeval-file-backend"
VERIFY[ext=*.example.com/default-options] request sni=bar.foo.example.com host=bar.foo.example.com path=/gw -> sni-selected-tls-options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" (err=<nil>) HTTP 200 body="10.10.0.1"
VERIFY[ext=**.foo.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" (err=<nil>) HTTP 200 body="10.10.0.1"
VERIFY[ext=**.foo.example.com/default-options] request sni=bar.foo.example.com host=bar.foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[ext=**.foo.example.com/default-options] request sni=baz.bar.foo.example.com host=baz.bar.foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[ext=**.foo.example.com/default-options] request sni=bar.foo.example.com host=bar.foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="aeval-file-backend"
VERIFY[ext=*.example.com/custom-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="custom@file" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[ext=**.foo.example.com/custom-options] request sni=bar.foo.example.com host=bar.foo.example.com path=/gw -> sni-selected-tls-options="custom@file" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[ext=both-wildcards/default-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[control/ext=foo.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="10.10.0.1"
VERIFY[control/ext=foo.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="aeval-file-backend"
VERIFY[ext=**.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="10.10.0.1"
```

Impact reasoning: SNI and Host are equal and the path `/gw` belongs only to the Gateway route, yet the request gets 421.
The SNI lookup picks the external wildcard's TLS options (`default` / `custom@file`) while HTTP routing picks the Gateway
route under the listener router that still carries the listener's TLS options, so SNICheck rejects it. Every host
covered by the more specific external wildcard loses all Gateway routes on that listener (the external route itself
keeps working); with two external wildcards (`both-wildcards`) the external route is hit as well. The trigger is an
ordinary multi-provider setup: one file/CRD/Ingress router with a wildcard Host on the same entry point as a wildcard
Gateway listener. Observed workarounds: use exact hosts, or the identical `**.` wildcard string, on the external router.

## C3

Probe: `verify/repro/verify_tlsconflict_probe_test.go.txt` (`TestVerify_C3_ChildOnlyHostname`), same production runtime.
Gateway `verify-gw`, HTTPS listener `broad=*.example.com`, hostname-free HTTPRoute on `Path /gw`; file provider:
`parent` (priority 100, `PathPrefix("/other")`, `tls: {}` or `tls.options: custom`) and `child`
(`Host("foo.example.com")`, `parentRefs: [parent]`, service `aeval-file-service`).

```console
$ go test -tags verify -count=1 -v -run 'TestVerify_' ./pkg/provider/kubernetes/gateway
--- FAIL: TestVerify_C3_ChildOnlyHostname (0.17s)
    --- FAIL: TestVerify_C3_ChildOnlyHostname/child-only-host/default-options (0.04s)
    --- FAIL: TestVerify_C3_ChildOnlyHostname/child-only-host/custom-options (0.04s)
    --- PASS: TestVerify_C3_ChildOnlyHostname/child-only-host/default-options/route-hostname (0.04s)
    --- PASS: TestVerify_C3_ChildOnlyHostname/control/parent-host/default-options (0.05s)
```

### Part 1 — child-hostname propagation

Routers after aggregation: the Gateway listener router keeps its own TLS options and is not renamed, i.e.
`foo.example.com` never reached its conflict inputs:

```console
VERIFY[child-only-host/default-options] router child@file rule=Host(`foo.example.com`) prio=23 parents=[parent@file] tls=nil status=enabled err=[]
VERIFY[child-only-host/default-options] router httproute-default-http-app-gw-gw-default-verify-gw-ep-websecure-0-30d97f2b85878610062c@kubernetesgateway rule=Host("**.example.com") && Path("/gw") prio=100014 parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] tls=nil status=enabled err=[]
VERIFY[child-only-host/default-options] router listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") prio=20 parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" conflicting=false status=enabled err=[]
VERIFY[child-only-host/default-options] router parent@file rule=PathPrefix(`/other`) prio=100 parents=[] tls.options="" resolved="default" conflicting=false status=enabled err=[]
```

Control, host declared on the parent instead of the child — both roots are renamed and fall back to default:

```console
VERIFY[control/parent-host/default-options] router websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway rule=Host("**.example.com") prio=20 parents=[] tls.options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default" conflicting=false status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
VERIFY[control/parent-host/default-options] router websecure-conflicted-parent@file rule=Host(`foo.example.com`) && PathPrefix(`/other`) prio=100 parents=[] tls.options="" resolved="default" conflicting=false status=enabled err=[]
```

Source consistent with this: in `resolveParentChildDomains` `epRoots` is built from each TLS root's *own* rule domains
before `childDomains` is collected; the file parent has no domain, so `parentClaimsDomain("parent@file",
"foo.example.com")` is false and the child host is added neither to `childDomains["parent@file"]` nor to any root the
Gateway listener's broad child is compared against.

### Part 2 — HTTPS runtime trigger

```console
VERIFY[child-only-host/default-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" (err=<nil>) HTTP 200 body="10.10.0.1"
VERIFY[child-only-host/default-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[child-only-host/custom-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[control/parent-host/default-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="aeval-file-backend"
VERIFY[child-only-host/default-options/route-hostname] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="aeval-file-backend"
```

Impact reasoning: the external route `foo.example.com/other` (parent priority 100, `PathPrefix("/other")`, child
`Host("foo.example.com")`) is unreachable over HTTPS — every request gets 421 instead of the documented default-TLS
fallback — as soon as a Gateway wildcard listener with a hostname-free route covers that host. The Gateway route itself
keeps serving. Scope observed: when the Gateway HTTPRoute itself names `foo.example.com` (`route-hostname` variant) the
fallback works and both paths return 200; moving the host to the external parent rule is a workaround.

## C4

The "provided cross-provider HTTPS tests" are `TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict`
(`eval_tlsconflict_test.go`) and `TestEval_GatewayHTTPSSiblingGatewayReachability` (`eval_https_test.go`). Every external
router in `eval_tlsconflict_test.go` is one of:

```go
Rule: `Host("aeval.example.com") && Path("/other")`                       // evalTLSConflictFileConfiguration, TLS: &dynamic.RouterTLSConfig{}
Rule: `Host("` + targetHost + `") && Path("/other")`, TLS: &dynamic.RouterTLSConfig{}   // fileConfFor, exact hosts only
"aeval-file-parent": {Rule: `Host("` + routeHost + `") && PathPrefix("/other")`, TLS: &dynamic.RouterTLSConfig{}},
"aeval-file-child":  {Rule: `Method("GET")`, ParentRefs: []string{"aeval-file-parent"}, ...},
```

and the only TLS options any file configuration defines is `"default": {MinVersion: "VersionTLS13"}`: no external
wildcard Host, no external router with `TLS.Options` set, no child router carrying the hostname.

### Part 1 — external wildcard coverage

The audited branch itself has the TLS-fallback failure for external wildcards, and the provided tests pass on it:

```console
$ go test -tags verify -count=1 -v -run 'TestVerify_' ./pkg/provider/kubernetes/gateway
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=*.example.com/default-options (0.08s)
    --- FAIL: TestVerify_C2_ExternalWildcards/ext=**.foo.example.com/default-options (0.03s)
VERIFY[ext=*.example.com/default-options] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[ext=**.foo.example.com/default-options] request sni=bar.foo.example.com host=bar.foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 421 body="Misdirected Request"
$ go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	5.413s
```

### Part 2 — named TLS-option coverage

On the unmodified audited branch an external exact-host router with `tls.options: custom` falls back correctly
(`TestVerify_C4_NamedOptions`):

```console
VERIFY[named/wildcard-listener] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="10.10.0.1"
VERIFY[named/wildcard-listener] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="default" (err=<nil>) HTTP 200 body="aeval-file-backend"
--- PASS: TestVerify_C4_NamedOptions (0.18s)
```

Targeted regression `verify/repro/c4_named_options_regression.patch` (external named TLS options no longer take part in
`findConflictingRouters`):

```diff
+		// VERIFY C4 regression: external named TLS options no longer take part in conflict detection.
+		if o := router.TLS.ResolvedOptions; o != traefiktls.DefaultTLSConfigName && !strings.HasSuffix(o, "@kubernetesgateway") {
+			continue
+		}
```

```console
$ git apply verify/repro/c4_named_options_regression.patch
$ go test -tags verify -count=1 -v -run 'TestVerify_C4' ./pkg/provider/kubernetes/gateway
VERIFY[named/wildcard-listener] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="custom@file" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[named/catchall-listener] request sni=foo.example.com host=foo.example.com path=/gw -> sni-selected-tls-options="custom@file" (err=<nil>) HTTP 421 body="Misdirected Request"
VERIFY[named/exact-listener] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" (err=<nil>) HTTP 421 body="Misdirected Request"
--- FAIL: TestVerify_C4_NamedOptions (0.21s)
$ go test -count=1 -timeout=5m -v -run '^(TestEval_GatewayHTTPSSiblingGatewayReachability|TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict)$' ./pkg/provider/kubernetes/gateway
--- PASS: TestEval_GatewayHTTPSSiblingGatewayReachability (1.71s)
--- PASS: TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict (0.41s)
    --- PASS: TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict/singleton (0.02s)
    --- PASS: TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict/partial (0.08s)
    --- PASS: TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict/wildcard (0.07s)
    --- PASS: TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict/catchall (0.05s)
    --- PASS: TestEval_GatewayHTTPSListenerIsolation_CrossProviderTLSConflict/external_parent (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	2.151s
$ go test -count=1 ./pkg/provider/kubernetes/gateway/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	6.129s
$ git checkout pkg/server/aggregator.go
```

All four fixture files (every `TestEval_*`) still pass under the regression. For completeness: the branch's own unit
tests in another package do notice it (`go test -count=1 ./pkg/server/` →
`--- FAIL: Test_resolveHTTPTLSOptions`, `Test_resolveHTTPTLSOptions_parentRefs`,
`Test_resolveHTTPTLSOptions_childHostConflict`), but those are not the provided cross-provider HTTPS tests.

### Part 3 — child-only hostname coverage

The audited branch itself has the failure, and the provided tests pass on it (their only parent/child case,
`external_parent`, puts the host on the parent and `Method("GET")` on the child):

```console
$ go test -tags verify -count=1 -v -run 'TestVerify_' ./pkg/provider/kubernetes/gateway
    --- FAIL: TestVerify_C3_ChildOnlyHostname/child-only-host/default-options (0.04s)
VERIFY[child-only-host/default-options] request sni=foo.example.com host=foo.example.com path=/other -> sni-selected-tls-options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" (err=<nil>) HTTP 421 body="Misdirected Request"
$ go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	5.413s
```

Impact reasoning: the fixtures accept an implementation in which (1) and (3) are broken in shipped behaviour and in which
(2) can regress silently; a green fixture run therefore does not establish the cross-provider TLS fallback for these
three shapes.

## C5

Branch `evalon/traefik-ga-0d524d9c` @ `d9701b00a`, worktree `~/repos/wt-0d524d9c`.

### Part 1 — backend generation

`listener_isolation_test.go` `listenerRoute` gives both route kinds the same backend:

```go
backend := gatev1.BackendRef{BackendObjectReference: gatev1.BackendObjectReference{
	Name: "backend", Group: new(gatev1.Group("test.traefik.io")), Kind: new(gatev1.Kind("TestBackend")),
}}
```

`LoadListenerConfiguration` (`listener_test.go:46`) registers `test.traefik.io/TestBackend` through
`p.RegisterBackendFuncs`, which only the HTTPRoute path consults; `loadGRPCBackendRef` (`grpcroute.go:246`) returns
`InvalidKind` for `group != groupCore || kind != kindService`, and `loadGRPCService` then emits a WRR child with
`GRPCStatus{Code: codes.Unavailable, Msg: "Service Unavailable"}`.

Probe `verify/repro/c5/verify_c5_probe_test.go.txt` (loads the same configuration as the positive controls, dumps the
generated services, sends the controls' positive request and prints the whole response):

```console
$ cp verify/repro/c5/verify_c5_probe_test.go.txt ~/repos/wt-0d524d9c/pkg/provider/kubernetes/gateway/verify_c5_probe_test.go
$ cd ~/repos/wt-0d524d9c && go test -count=1 -v -run 'TestVerifyC5' ./pkg/provider/kubernetes/gateway
    verify_c5_probe_test.go:36: VERIFY[HTTPRoute]   wrr child test-backend: GRPCStatus=<nil> Status=false generatedAsService=true
    verify_c5_probe_test.go:36: VERIFY[HTTPRoute]   wrr child backend: GRPCStatus=<nil> Status=true generatedAsService=false
    verify_c5_probe_test.go:41: VERIFY[HTTPRoute] positive request -> HTTP 200 headers=map[] body=""
    verify_c5_probe_test.go:30: VERIFY[GRPCRoute] service grpcroute-default-route-gw-default-gateway-ep-web-0-82959b5c35d9427c292e-wrr = {"weighted":{"services":[{"name":"grpcroute-default-route-gw-default-gateway-ep-web-0-82959b5c35d9427c292e-svc-default-backend-0","weight":1}]}}
    verify_c5_probe_test.go:36: VERIFY[GRPCRoute]   wrr child grpcroute-default-route-gw-default-gateway-ep-web-0-82959b5c35d9427c292e-svc-default-backend-0: GRPCStatus=&{Code:Unavailable Msg:Service Unavailable} Status=false generatedAsService=false
    verify_c5_probe_test.go:41: VERIFY[GRPCRoute] positive request -> HTTP 200 headers=map[Content-Type:[application/json]] body="{\"code\":14,\"message\":\"Service Unavailable\"}"
    verify_c5_probe_test.go:43: VERIFY[GRPCRoute] HTTP 200 carries gRPC status 14 Unavailable: no successful backend was reached
--- FAIL: TestVerifyC5 (0.18s)
```

### Part 2 — positive-control assertions

Every positive control only compares the HTTP status code, e.g.
`assert.Equal(t, status, listenerRequest(handler, host, host, string(listener.Name)).Code)` and
`assert.Equal(t, http.StatusOK, listenerRequest(handler, "foo.example.org", "foo.example.org", "Call").Code)`; none of
the tests using `listenerRoute` compares the generated configuration. The delivered tests, run unmodified on the branch
(where the GRPCRoute service is the error service shown above):

```console
$ cd ~/repos/wt-0d524d9c && go test -count=1 -v -run '^(TestGatewayListenerIsolation|TestGatewayListenerRouteHostnameIsolation|TestGatewayHTTPSListenerMisdirectedRequests|TestGatewayHTTPSListenerTLSOptionsConflict|TestGatewayHTTPSListenerSharedRoute|TestLoadGRPCRoutes)$' ./pkg/provider/kubernetes/gateway
--- PASS: TestLoadGRPCRoutes (0.00s)
--- PASS: TestGatewayListenerIsolation (0.00s)
--- PASS: TestGatewayListenerRouteHostnameIsolation (0.00s)
--- PASS: TestGatewayHTTPSListenerMisdirectedRequests (0.00s)
--- PASS: TestGatewayHTTPSListenerTLSOptionsConflict (0.00s)
--- PASS: TestGatewayHTTPSListenerSharedRoute (0.00s)
        --- PASS: TestGatewayListenerIsolation/GRPCRoute/HTTP/intersect=false/other.org/catch-all (0.00s)
        --- PASS: TestGatewayHTTPSListenerMisdirectedRequests/GRPCRoute/second.org/second.org (0.00s)
    --- PASS: TestGatewayHTTPSListenerTLSOptionsConflict/GRPCRoute/exact (0.02s)
    --- PASS: TestGatewayHTTPSListenerSharedRoute/GRPCRoute (0.03s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.488s
```

245 GRPCRoute subtests pass, 0 fail. `TestLoadGRPCRoutes` does compare full configurations but does not use
`listenerRoute` / `TestBackend`, so it does not cover these controls.

Impact reasoning: on this branch every GRPCRoute "200" in the listener-isolation tests is the Unavailable error
service's HTTP 200 JSON body, not a backend. The tests still distinguish "router matched" (200) from "no router" (404)
and 421, so listener selection is exercised, but they would equally pass if GRPCRoute backends were never wired, and
they assert nothing about real gRPC backend routing.
