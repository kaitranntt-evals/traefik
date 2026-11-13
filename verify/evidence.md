# Evidence — run v-8260ab0c

Observations only. Audited branch: `traefik-gateway-listener-isolation-perfect` @ f489a039ccd73f8748ddee88eae309b6e0ecb26d. Claim branches for C1, C2, C7, C8 come from the sibling repository `kaitranntt-evals/traefik-gateway-listener-isolation` and were checked out in separate detached worktrees. Probes carry `//go:build verify` and only run with `-tags verify`. `<eval_tests>` is the directory where `eval_tests.zip` was extracted; its four files were copied byte-exact. Full raw outputs are under `verify/logs/`.

Baseline on the audited branch (fixtures copied in):

```sh
go test -race -count=1 -timeout=5m -run 'TestEval_' ./pkg/provider/kubernetes/gateway
go vet ./pkg/provider/kubernetes/gateway/...
go test -run='^$' ./pkg/provider/kubernetes/gateway/...
go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway -run 'TestGatewayClassLabelSelector|Test_matchingGatewayListener'
```

```console
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	11.624s
vet exit 0
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.027s [no tests to run]
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.084s
```

## C1

Target: `evalon/traefik-ga-5998d740` @ 0aee7934cf69b3941ba85334395a5f701be5a58e, worktree `~/repos/wt-5998d740`. Instrumentation: `verify/repro/c1_drop_fallback_service.diff` (test file only; `git apply` it in the worktree). It logs each response and, when `VERIFY_DROP=<scope>:<substring>` is set, deletes the matching entries from `conf.HTTP.Services` of the production-generated configuration before the handler is built.

Expectations of the HTTPS `TestListenerIsolation` case (`pkg/provider/kubernetes/gateway/listener_test.go`): each row is `{host, route, expected}`; rows with an expected name assert `200` and a body containing `-default-<expected>-`; rows without assert only `assert.Equal(t, http.StatusNotFound, rec.Code)`. No row or other assertion names a fallback service, checks the body of a 404, or enumerates generated services. The production comment on the service in `listener.go` is "SNI selection still applies when the selected listener has no matching route."

```sh
VERIFY_DROP= go test -count=1 -v -run '^TestListenerIsolation$/HTTPS' ./pkg/provider/kubernetes/gateway
VERIFY_DROP=ISO:wildcard-listener-fallback go test -count=1 -v -run '^TestListenerIsolation$/HTTPS' ./pkg/provider/kubernetes/gateway
VERIFY_DROP=MIS:wildcard-listener-fallback go test -count=1 -v -run '^TestHTTPSListenerMisdirectedRequests$' ./pkg/provider/kubernetes/gateway
```

Key output (HTTPRoute rows; GRPCRoute rows are equivalent):

```console
### VERIFY_DROP= go test -count=1 -v -run '^TestListenerIsolation$/HTTPS' ./pkg/provider/kubernetes/gateway
    listener_test.go:39: VERIFY generated fallback service default-gateway-wildcard-listener-fallback (router rule: HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !((HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") || Host("bar.foo.example.com") || Host("reserved.example.com"))))
    listener_test.go:39: VERIFY generated fallback service default-gateway-wildcard-listener-fallback (router rule: HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !((HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") || Host("bar.foo.example.com") || Host("reserved.example.com"))))
    listener_test.go:67: VERIFY RESP unknown.test/catchall -> 200 "httproute-default-catchall-gw-default-gateway-ep-web-0-021ab09128268b8c3eb9-wrr"
    listener_test.go:67: VERIFY RESP example.com/catchall -> 200 "httproute-default-catchall-gw-default-gateway-ep-web-0-021ab09128268b8c3eb9-wrr"
    listener_test.go:67: VERIFY RESP a.example.com/catchall -> 404 "default-gateway-wildcard-listener-fallback"
    listener_test.go:67: VERIFY RESP bar.foo.example.com/catchall -> 404 "default-gateway-exact-listener-fallback"
    listener_test.go:67: VERIFY RESP reserved.example.com/catchall -> 404 "default-gateway-reserved-listener-fallback"
--- PASS: TestListenerIsolation (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.141s
### VERIFY_DROP=ISO:wildcard-listener-fallback go test -count=1 -v -run '^TestListenerIsolation$/HTTPS' ./pkg/provider/kubernetes/gateway
    listener_test.go:39: VERIFY generated fallback service default-gateway-wildcard-listener-fallback (router rule: HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !((HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") || Host("bar.foo.example.com") || Host("reserved.example.com"))))
    listener_test.go:39: VERIFY removed service default-gateway-wildcard-listener-fallback
    listener_test.go:39: VERIFY generated fallback service default-gateway-wildcard-listener-fallback (router rule: HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !((HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") || Host("bar.foo.example.com") || Host("reserved.example.com"))))
    listener_test.go:39: VERIFY removed service default-gateway-wildcard-listener-fallback
    listener_test.go:67: VERIFY RESP unknown.test/catchall -> 200 "httproute-default-catchall-gw-default-gateway-ep-web-0-021ab09128268b8c3eb9-wrr"
    listener_test.go:67: VERIFY RESP example.com/catchall -> 200 "httproute-default-catchall-gw-default-gateway-ep-web-0-021ab09128268b8c3eb9-wrr"
    listener_test.go:67: VERIFY RESP a.example.com/catchall -> 404 "404 page not found"
    listener_test.go:67: VERIFY RESP bar.foo.example.com/catchall -> 404 "default-gateway-exact-listener-fallback"
    listener_test.go:67: VERIFY RESP reserved.example.com/catchall -> 404 "default-gateway-reserved-listener-fallback"
--- PASS: TestListenerIsolation (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.120s
### VERIFY_DROP=MIS:wildcard-listener-fallback go test -count=1 -v -run '^TestHTTPSListenerMisdirectedRequests$' ./pkg/provider/kubernetes/gateway
    listener_test.go:87: VERIFY generated fallback service default-gateway-wildcard-listener-fallback (router rule: HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !((HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") || Host("bar.foo.example.com") || Host("reserved.example.com"))))
    listener_test.go:87: VERIFY removed service default-gateway-wildcard-listener-fallback
    listener_test.go:87: VERIFY generated fallback service default-gateway-wildcard-listener-fallback (router rule: HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !((HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") || Host("bar.foo.example.com") || Host("reserved.example.com"))))
    listener_test.go:87: VERIFY removed service default-gateway-wildcard-listener-fallback
--- PASS: TestHTTPSListenerMisdirectedRequests (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.130s
```

Observed: the only row that reaches the wildcard fallback router is `a.example.com/catchall`, whose asserted outcome is status 404. With the service present the 404 comes from the fallback service (body `default-gateway-wildcard-listener-fallback`); with only that service removed the 404 comes from the router layer (body `404 page not found`). The asserted outcome is identical, every assertion passes, and removing the same service also leaves `TestHTTPSListenerMisdirectedRequests` passing. So the omission is undetected, but no expectation or asserted outcome of the case requires the service: the catch-all route is already kept away from `a.example.com` by the catch-all listener router's own exclusion rule, so for this case the service is an auxiliary guard.

## C2

Target: `evalon/traefik-ga-8341c750` @ 0018afefd4b18c84d03084e4fe8f45df2a750e84, worktree `~/repos/wt-8341c750`. Test under inspection: `TestHTTPSListener_conflictWithAnotherProvider` in `pkg/provider/kubernetes/gateway/listener_isolation_server_test.go` (added by the branch). It loads Gateway HTTPS routes for `example.com`, `foo.example.com`, `*.example.com` plus a file-provider router `other@file` = `Host("example.com") && PathPrefix("/other")` on the same entry point `websecure`, runs both through `server.NewConfigurationWatcher` (production `mergeConfiguration` / `resolveHTTPTLSOptions`, strict mode disabled), then asserts for every resulting router that `ConflictingOptions` is false and that the map of router → `TLS.ResolvedOptions` equals an expected map containing the Gateway route `httproute-default-example-com: default`. The configuration is flat (no parent references).

Instrumentation: `verify/repro/c2_mutations.diff` (test file only). `VERIFY_MUT=drop` deletes the conflicted Gateway route from the resolved configuration before the assertions; `VERIFY_MUT=strict` runs the watcher with strict TLS mode enabled.

```sh
VERIFY_MUT= go test -count=1 -v -run '^TestHTTPSListener_conflictWithAnotherProvider$' ./pkg/provider/kubernetes/gateway
VERIFY_MUT=drop go test -count=1 -v -run '^TestHTTPSListener_conflictWithAnotherProvider$' ./pkg/provider/kubernetes/gateway
VERIFY_MUT=strict go test -count=1 -v -run '^TestHTTPSListener_conflictWithAnotherProvider$' ./pkg/provider/kubernetes/gateway
```

```console
### VERIFY_MUT= go test -count=1 -v -run '^TestHTTPSListener_conflictWithAnotherProvider$' ./pkg/provider/kubernetes/gateway
    listener_isolation_server_test.go:77: VERIFY router websecure-conflicted-httproute-default-example-com-gw-default-my-gateway-ep-websecure-0-2009d9cf3d9c53f3246a@kubernetesgateway rule=Host("example.com") && (Path("/example-com") || PathPrefix("/example-com/")) resolved=default conflicting=false parentRefs=[]
    listener_isolation_server_test.go:77: VERIFY router httproute-default-foo-example-com-gw-default-my-gateway-ep-websecure-0-f2b7d949ea84aa206d9f@kubernetesgateway rule=Host("foo.example.com") && (Path("/foo-example-com") || PathPrefix("/foo-example-com/")) resolved=websecure-foo-example-com@kubernetesgateway conflicting=false
    listener_isolation_server_test.go:77: VERIFY router httproute-default-wildcard-example-com-gw-default-my-gateway-ep-websecure-0-520f64a0db3a03a23922@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && (Path("/wildcard-example-com") || PathPrefix("/wildcard-example-com/")) && !Host("foo.example.com") resol
    listener_isolation_server_test.go:77: VERIFY router websecure-conflicted-other@file rule=Host("example.com") && PathPrefix("/other") resolved=default conflicting=false parentRefs=[]
--- PASS: TestHTTPSListener_conflictWithAnotherProvider (0.06s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.083s
### VERIFY_MUT=drop go test -count=1 -v -run '^TestHTTPSListener_conflictWithAnotherProvider$' ./pkg/provider/kubernetes/gateway
    listener_isolation_server_test.go:77: VERIFY router httproute-default-wildcard-example-com-gw-default-my-gateway-ep-websecure-0-520f64a0db3a03a23922@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && (Path("/wildcard-example-com") || PathPrefix("/wildcard-example-com/")) && !Host("foo.example.com") resol
    listener_isolation_server_test.go:77: VERIFY router websecure-conflicted-other@file rule=Host("example.com") && PathPrefix("/other") resolved=default conflicting=false parentRefs=[]
    listener_isolation_server_test.go:77: VERIFY router websecure-conflicted-httproute-default-example-com-gw-default-my-gateway-ep-websecure-0-2009d9cf3d9c53f3246a@kubernetesgateway rule=Host("example.com") && (Path("/example-com") || PathPrefix("/example-com/")) resolved=default conflicting=false parentRefs=[]
    listener_isolation_server_test.go:81: VERIFY dropped websecure-conflicted-httproute-default-example-com-gw-default-my-gateway-ep-websecure-0-2009d9cf3d9c53f3246a@kubernetesgateway
    listener_isolation_server_test.go:77: VERIFY router httproute-default-foo-example-com-gw-default-my-gateway-ep-websecure-0-f2b7d949ea84aa206d9f@kubernetesgateway rule=Host("foo.example.com") && (Path("/foo-example-com") || PathPrefix("/foo-example-com/")) resolved=websecure-foo-example-com@kubernetesgateway conflicting=false
        	Error:      	Not equal: 
        	            	expected: map[string]string{"httproute-default-example-com":"default", "httproute-default-foo-example-com":"websecure-foo-example-com@kubernetesgateway", "httproute-default-wildcard-example-com":"default", "other@file":"default"}
        	            	actual  : map[string]string{"httproute-default-foo-example-com":"websecure-foo-example-com@kubernetesgateway", "httproute-default-wildcard-example-com":"default", "other@file":"default"}
--- FAIL: TestHTTPSListener_conflictWithAnotherProvider (0.05s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.078s
FAIL
### VERIFY_MUT=strict go test -count=1 -v -run '^TestHTTPSListener_conflictWithAnotherProvider$' ./pkg/provider/kubernetes/gateway
    listener_isolation_server_test.go:77: VERIFY router httproute-default-wildcard-example-com-gw-default-my-gateway-ep-websecure-0-520f64a0db3a03a23922@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && (Path("/wildcard-example-com") || PathPrefix("/wildcard-example-com/")) && !Host("foo.example.com") resol
    listener_isolation_server_test.go:77: VERIFY router websecure-conflicted-other@file rule=Host("example.com") && PathPrefix("/other") resolved=default conflicting=true parentRefs=[]
    listener_isolation_server_test.go:77: VERIFY router websecure-conflicted-httproute-default-example-com-gw-default-my-gateway-ep-websecure-0-2009d9cf3d9c53f3246a@kubernetesgateway rule=Host("example.com") && (Path("/example-com") || PathPrefix("/example-com/")) resolved=websecure-example-com@kubernetesgateway conflicting=true
    listener_isolation_server_test.go:77: VERIFY router httproute-default-foo-example-com-gw-default-my-gateway-ep-websecure-0-f2b7d949ea84aa206d9f@kubernetesgateway rule=Host("foo.example.com") && (Path("/foo-example-com") || PathPrefix("/foo-example-com/")) resolved=websecure-foo-example-com@kubernetesgateway conflicting=false
        	Error:      	Should be false
        	Error:      	Should be false
        	Error:      	Not equal: 
        	            	expected: map[string]string{"httproute-default-example-com":"default", "httproute-default-foo-example-com":"websecure-foo-example-com@kubernetesgateway", "httproute-default-wildcard-example-com":"default", "other@file":"default"}
        	            	actual  : map[string]string{"httproute-default-example-com":"websecure-example-com@kubernetesgateway", "httproute-default-foo-example-com":"websecure-foo-example-com@kubernetesgateway", "httproute-default-wildcard-example-com":"default", "other@file":"default"}
--- FAIL: TestHTTPSListener_conflictWithAnotherProvider (0.06s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.081s
FAIL
```

Observed: the unmodified test passes with the Gateway `example.com` route present, `resolved=default`, `conflicting=false`. Losing the Gateway route fails the map assertion (line 96); strict-mode handling (route left on dedicated options and flagged conflicting) fails lines 89 and 96. The test therefore detects loss or disabling of the Gateway route after non-strict fallback.

## C3

Setup for the audited branch (`traefik-gateway-listener-isolation-perfect` @ f489a039ccd73f8748ddee88eae309b6e0ecb26d), run from `~/repos/traefik`; `<eval_tests>` is the extracted `eval_tests.zip`:

```sh
cp <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_descendant_tls_probe_test.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_resolve_trace_test.go pkg/server/verify_c3456_resolve_trace_test.go
go test -tags verify -count=1 -v -run TestVerify_ResolveTrace ./pkg/server
go test -tags verify -count=1 -v -run 'TestVerify_DescendantTLS' ./pkg/provider/kubernetes/gateway
```

The trace test calls `resolveParentChildDomains` and `resolveHTTPTLSOptions(routers, false)` directly. The probe loads a real Gateway/HTTPRoute through the Gateway provider plus a file-provider configuration, runs them through the production configuration watcher with strict TLS mode disabled, builds the production routers and TLS manager, and sends HTTPS requests in-process (no real TLS handshake; SNI is supplied to the TLS-config lookup). In every case the external tree is rooted at `ext-root@file` = `PathPrefix(/other)` with TLS on entry point `websecure`, the Gateway route is `Path(/gw)` on an HTTPS listener of the same entry point with its own dedicated TLS options, and requests use host/SNI `foo.example.com`. `gatewayFallback=true` means the Gateway listener router was renamed `websecure-conflicted-…` with `ResolvedOptions=default`. `extopts` is the TLS options name on the external root (`default` = none, `custom` = named). `listener` is the Gateway listener hostname: `exact` = `foo.example.com`, `wildcard` = `*.example.com` (rendered as `Host("**.example.com")`), `catchall` = none, `disjoint_control` = a non-overlapping hostname.

Tree under test (`grandchild_exact_host`): `ext-root@file` (TLS, `PathPrefix(/other)`) → `ext-mid@file` (`Method(GET)`, no host, parentRef root) → `ext-grandchild@file` (`Host(foo.example.com)`, parentRef mid, service). The tree is valid: with a disjoint Gateway listener (`disjoint_control`) `GET /other` returns 200 from the file backend.

Observed:
- Propagation: `childDomains[ext-root@file] = []` for the grandchild case, versus `[foo.example.com …]` for the direct-child control. `resolveParentChildDomains` only records a child's domains when `rootOwn[parentRef]` exists, i.e. when the immediate parent is itself a TLS root; `ext-mid@file` is not.
- Production fallback: the Gateway listener router keeps `TLS.ResolvedOptions=listener@kubernetesgateway` (trace) / `gatewayFallback=false sniOptions="listener-websecure-https-foo-example-com-…@kubernetesgateway"` (probe). The direct-child control with the same hostname falls back (`gatewayFallback=true sniOptions="default"`).
- User-visible effect: `GET https://foo.example.com/other` → `421 Misdirected Request` with exact and wildcard Gateway listeners (the external route is unreachable); with a catch-all Gateway listener the Gateway route itself returns 421 while the file route serves. The control returns 200 for both routes.

Impact reasoning: the task requires HTTPS listener routes to keep serving when another provider declares a router for the same host on the same entry point, falling back to default TLS options. Here both providers serve `foo.example.com` on `websecure`, no fallback is applied, and one of the two providers' routes answers 421 depending on listener shape. The trigger is a three-level external router tree whose hostname sits on the leaf; a two-level tree works. Workaround: put the Host matcher on the TLS root or on its direct child.

Domain trace (`TestVerify_ResolveTrace`):

```console
CASE control: child Host(foo.example.com) / gateway exact
  childDomains[ext-root@file] = [foo.example.com foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C3/C5: grandchild Host(foo.example.com) behind intermediate / gateway exact
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE C4: child Host(*.example.com) / gateway exact foo.example.com
  childDomains[ext-root@file] = [foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C4/C6: child Host(*.example.com) / gateway **.example.com
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = []
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE control: flat external Host(*.example.com) / gateway **.example.com
  childDomains[ext@file] = []
  childDomains[listener@kubernetesgateway] = [*.example.com]
  resolved websecure-conflicted-ext@file                    TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
--- PASS: TestVerify_ResolveTrace (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.028s
```

Production probe (`TestVerify_DescendantTLS`), direct-child control rows then `grandchild_exact_host` rows:

```console
RESULT control_child_exact_host/extopts=default/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=wildcard gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=catchall gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=default/listener=exact gatewayFallback=false sniOptions="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=default/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=default/listener=catchall gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=421(Misdirected Request) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=custom/listener=exact gatewayFallback=false sniOptions="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=custom/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=custom/listener=catchall gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=421(Misdirected Request) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=custom/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
--- PASS: TestVerify_DescendantTLS (1.22s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.245s
```

## C4

Setup for the audited branch (`traefik-gateway-listener-isolation-perfect` @ f489a039ccd73f8748ddee88eae309b6e0ecb26d), run from `~/repos/traefik`; `<eval_tests>` is the extracted `eval_tests.zip`:

```sh
cp <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_descendant_tls_probe_test.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_resolve_trace_test.go pkg/server/verify_c3456_resolve_trace_test.go
go test -tags verify -count=1 -v -run TestVerify_ResolveTrace ./pkg/server
go test -tags verify -count=1 -v -run 'TestVerify_DescendantTLS' ./pkg/provider/kubernetes/gateway
```

The trace test calls `resolveParentChildDomains` and `resolveHTTPTLSOptions(routers, false)` directly. The probe loads a real Gateway/HTTPRoute through the Gateway provider plus a file-provider configuration, runs them through the production configuration watcher with strict TLS mode disabled, builds the production routers and TLS manager, and sends HTTPS requests in-process (no real TLS handshake; SNI is supplied to the TLS-config lookup). In every case the external tree is rooted at `ext-root@file` = `PathPrefix(/other)` with TLS on entry point `websecure`, the Gateway route is `Path(/gw)` on an HTTPS listener of the same entry point with its own dedicated TLS options, and requests use host/SNI `foo.example.com`. `gatewayFallback=true` means the Gateway listener router was renamed `websecure-conflicted-…` with `ResolvedOptions=default`. `extopts` is the TLS options name on the external root (`default` = none, `custom` = named). `listener` is the Gateway listener hostname: `exact` = `foo.example.com`, `wildcard` = `*.example.com` (rendered as `Host("**.example.com")`), `catchall` = none, `disjoint_control` = a non-overlapping hostname.

Tree under test (`child_wildcard_host`): `ext-root@file` (TLS, `PathPrefix(/other)`) → `ext-child@file` (`Host(*.example.com)`, service).

Observed:
- Propagation: against a Gateway `**.example.com` listener `childDomains[ext-root@file] = []` — the wildcard child domain is dropped by `!isBroad(c)` (`isBroad` is true for `*`, `*.…`, `**.…`). Against an exact Gateway listener `foo.example.com` the literal `*.example.com` is still not collected, but a later step adds the concrete `foo.example.com` to the root.
- Production fallback, Gateway listener hostname `*.example.com` (router rule `Host("**.example.com")`): `gatewayFallback=false`, Gateway keeps `listener-websecure-https-example-com-…@kubernetesgateway`, `GET /other` → 421. This is the claimed failure.
- Production fallback, Gateway listener hostname exactly `foo.example.com`: `gatewayFallback=true sniOptions="default"`, both routes 200. For this overlap the claimed failure does not occur.
- Control: a flat external router `Host(*.example.com)` against Gateway `**.example.com` does fall back (last trace case), so the miss is specific to the wildcard living on a child.

Impact reasoning: wildcard-vs-wildcard overlap between a Gateway wildcard listener and an external parent whose only hostname is a wildcard child is not detected; the external route answers 421 for hosts under `*.example.com` (with a catch-all Gateway listener and default external options, the Gateway route answers 421 instead). Wildcard-vs-exact overlap is handled. Workaround: put the wildcard Host on the external TLS root.

Domain trace (`TestVerify_ResolveTrace`):

```console
CASE control: child Host(foo.example.com) / gateway exact
  childDomains[ext-root@file] = [foo.example.com foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C3/C5: grandchild Host(foo.example.com) behind intermediate / gateway exact
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE C4: child Host(*.example.com) / gateway exact foo.example.com
  childDomains[ext-root@file] = [foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C4/C6: child Host(*.example.com) / gateway **.example.com
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = []
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE control: flat external Host(*.example.com) / gateway **.example.com
  childDomains[ext@file] = []
  childDomains[listener@kubernetesgateway] = [*.example.com]
  resolved websecure-conflicted-ext@file                    TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
--- PASS: TestVerify_ResolveTrace (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.028s
```

Production probe (`TestVerify_DescendantTLS`), direct-child control rows then `child_wildcard_host` rows:

```console
RESULT control_child_exact_host/extopts=default/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=wildcard gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=catchall gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=default/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=default/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT child_wildcard_host/extopts=default/listener=catchall gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=421(Misdirected Request) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=custom/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=custom/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT child_wildcard_host/extopts=custom/listener=catchall gatewayFallback=false sniOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT child_wildcard_host/extopts=custom/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
--- PASS: TestVerify_DescendantTLS (1.22s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.245s
```

## C5

Setup for the audited branch (`traefik-gateway-listener-isolation-perfect` @ f489a039ccd73f8748ddee88eae309b6e0ecb26d), run from `~/repos/traefik`; `<eval_tests>` is the extracted `eval_tests.zip`:

```sh
cp <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_descendant_tls_probe_test.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_resolve_trace_test.go pkg/server/verify_c3456_resolve_trace_test.go
go test -tags verify -count=1 -v -run TestVerify_ResolveTrace ./pkg/server
go test -tags verify -count=1 -v -run 'TestVerify_DescendantTLS' ./pkg/provider/kubernetes/gateway
```

The trace test calls `resolveParentChildDomains` and `resolveHTTPTLSOptions(routers, false)` directly. The probe loads a real Gateway/HTTPRoute through the Gateway provider plus a file-provider configuration, runs them through the production configuration watcher with strict TLS mode disabled, builds the production routers and TLS manager, and sends HTTPS requests in-process (no real TLS handshake; SNI is supplied to the TLS-config lookup). In every case the external tree is rooted at `ext-root@file` = `PathPrefix(/other)` with TLS on entry point `websecure`, the Gateway route is `Path(/gw)` on an HTTPS listener of the same entry point with its own dedicated TLS options, and requests use host/SNI `foo.example.com`. `gatewayFallback=true` means the Gateway listener router was renamed `websecure-conflicted-…` with `ResolvedOptions=default`. `extopts` is the TLS options name on the external root (`default` = none, `custom` = named). `listener` is the Gateway listener hostname: `exact` = `foo.example.com`, `wildcard` = `*.example.com` (rendered as `Host("**.example.com")`), `catchall` = none, `disjoint_control` = a non-overlapping hostname.

Tree under test (`grandchild_exact_host`): `ext-root@file` (TLS, `PathPrefix(/other)`) → `ext-mid@file` (`Method(GET)`, no host, parentRef root) → `ext-grandchild@file` (`Host(foo.example.com)`, parentRef mid, service). The tree is valid: with a disjoint Gateway listener (`disjoint_control`) `GET /other` returns 200 from the file backend.

Observed:
- Propagation: `childDomains[ext-root@file] = []` for the grandchild case, versus `[foo.example.com …]` for the direct-child control. `resolveParentChildDomains` only records a child's domains when `rootOwn[parentRef]` exists, i.e. when the immediate parent is itself a TLS root; `ext-mid@file` is not.
- Production fallback: the Gateway listener router keeps `TLS.ResolvedOptions=listener@kubernetesgateway` (trace) / `gatewayFallback=false sniOptions="listener-websecure-https-foo-example-com-…@kubernetesgateway"` (probe). The direct-child control with the same hostname falls back (`gatewayFallback=true sniOptions="default"`).
- User-visible effect: `GET https://foo.example.com/other` → `421 Misdirected Request` with exact and wildcard Gateway listeners (the external route is unreachable); with a catch-all Gateway listener the Gateway route itself returns 421 while the file route serves. The control returns 200 for both routes.

Impact reasoning: the task requires HTTPS listener routes to keep serving when another provider declares a router for the same host on the same entry point, falling back to default TLS options. Here both providers serve `foo.example.com` on `websecure`, no fallback is applied, and one of the two providers' routes answers 421 depending on listener shape. The trigger is a three-level external router tree whose hostname sits on the leaf; a two-level tree works. Workaround: put the Host matcher on the TLS root or on its direct child.

Domain trace (`TestVerify_ResolveTrace`):

```console
CASE control: child Host(foo.example.com) / gateway exact
  childDomains[ext-root@file] = [foo.example.com foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C3/C5: grandchild Host(foo.example.com) behind intermediate / gateway exact
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE C4: child Host(*.example.com) / gateway exact foo.example.com
  childDomains[ext-root@file] = [foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C4/C6: child Host(*.example.com) / gateway **.example.com
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = []
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE control: flat external Host(*.example.com) / gateway **.example.com
  childDomains[ext@file] = []
  childDomains[listener@kubernetesgateway] = [*.example.com]
  resolved websecure-conflicted-ext@file                    TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
--- PASS: TestVerify_ResolveTrace (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.028s
```

Production probe (`TestVerify_DescendantTLS`), direct-child control rows then `grandchild_exact_host` rows:

```console
RESULT control_child_exact_host/extopts=default/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=wildcard gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=catchall gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=default/listener=exact gatewayFallback=false sniOptions="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=default/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=default/listener=catchall gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=421(Misdirected Request) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=custom/listener=exact gatewayFallback=false sniOptions="listener-websecure-https-foo-example-com-e031da8f0a3144ae900c@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=custom/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT grandchild_exact_host/extopts=custom/listener=catchall gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=421(Misdirected Request) GET/other=200(aeval-file-backend)
RESULT grandchild_exact_host/extopts=custom/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
--- PASS: TestVerify_DescendantTLS (1.22s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.245s
```

## C6

Setup for the audited branch (`traefik-gateway-listener-isolation-perfect` @ f489a039ccd73f8748ddee88eae309b6e0ecb26d), run from `~/repos/traefik`; `<eval_tests>` is the extracted `eval_tests.zip`:

```sh
cp <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_descendant_tls_probe_test.go pkg/provider/kubernetes/gateway/
cp verify/repro/c3456_resolve_trace_test.go pkg/server/verify_c3456_resolve_trace_test.go
go test -tags verify -count=1 -v -run TestVerify_ResolveTrace ./pkg/server
go test -tags verify -count=1 -v -run 'TestVerify_DescendantTLS' ./pkg/provider/kubernetes/gateway
```

The trace test calls `resolveParentChildDomains` and `resolveHTTPTLSOptions(routers, false)` directly. The probe loads a real Gateway/HTTPRoute through the Gateway provider plus a file-provider configuration, runs them through the production configuration watcher with strict TLS mode disabled, builds the production routers and TLS manager, and sends HTTPS requests in-process (no real TLS handshake; SNI is supplied to the TLS-config lookup). In every case the external tree is rooted at `ext-root@file` = `PathPrefix(/other)` with TLS on entry point `websecure`, the Gateway route is `Path(/gw)` on an HTTPS listener of the same entry point with its own dedicated TLS options, and requests use host/SNI `foo.example.com`. `gatewayFallback=true` means the Gateway listener router was renamed `websecure-conflicted-…` with `ResolvedOptions=default`. `extopts` is the TLS options name on the external root (`default` = none, `custom` = named). `listener` is the Gateway listener hostname: `exact` = `foo.example.com`, `wildcard` = `*.example.com` (rendered as `Host("**.example.com")`), `catchall` = none, `disjoint_control` = a non-overlapping hostname.

Tree under test (`child_wildcard_host`, `listener=wildcard`): `ext-root@file` (TLS, `PathPrefix(/other)`) → `ext-child@file` (`Host(*.example.com)`, service); Gateway listener hostname `*.example.com`, which the provider renders as `Host("**.example.com")`.

Observed:
- Collection: trace case "child Host(*.example.com) / gateway **.example.com" prints `childDomains[ext-root@file] = []` — `!isBroad(c)` removes the wildcard child hostname.
- Production resolution: `resolved listener@kubernetesgateway TLS.ResolvedOptions=listener@kubernetesgateway` (trace); probe row `child_wildcard_host/…/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-…@kubernetesgateway" … GET/other=421(Misdirected Request)` for both `extopts=default` and `extopts=custom`.
- Control: flat external `Host(*.example.com)` vs Gateway `**.example.com` falls back to `default` for both routers.

Impact reasoning: with a Gateway wildcard listener and an external router tree whose wildcard host is on a child, the two providers overlap on every `*.example.com` name, default fallback is not applied, and the external route is answered with 421 instead of being served. Workaround: declare the wildcard Host on the external TLS root.

Domain trace (`TestVerify_ResolveTrace`):

```console
CASE control: child Host(foo.example.com) / gateway exact
  childDomains[ext-root@file] = [foo.example.com foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C3/C5: grandchild Host(foo.example.com) behind intermediate / gateway exact
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE C4: child Host(*.example.com) / gateway exact foo.example.com
  childDomains[ext-root@file] = [foo.example.com foo.example.com]
  childDomains[listener@kubernetesgateway] = [foo.example.com]
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
CASE C4/C6: child Host(*.example.com) / gateway **.example.com
  childDomains[ext-root@file] = []
  childDomains[listener@kubernetesgateway] = []
  resolved listener@kubernetesgateway                       TLS.ResolvedOptions=listener@kubernetesgateway
  resolved websecure-conflicted-ext-root@file               TLS.ResolvedOptions=default
CASE control: flat external Host(*.example.com) / gateway **.example.com
  childDomains[ext@file] = []
  childDomains[listener@kubernetesgateway] = [*.example.com]
  resolved websecure-conflicted-ext@file                    TLS.ResolvedOptions=default
  resolved websecure-conflicted-listener@kubernetesgateway  TLS.ResolvedOptions=default
--- PASS: TestVerify_ResolveTrace (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server	0.028s
```

Production probe (`TestVerify_DescendantTLS`), direct-child control rows then `child_wildcard_host` rows:

```console
RESULT control_child_exact_host/extopts=default/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=wildcard gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=catchall gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT control_child_exact_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=default/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=default/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT child_wildcard_host/extopts=default/listener=catchall gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=421(Misdirected Request) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=default/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=custom/listener=exact gatewayFallback=true sniOptions="default" minVersion=0x304 err=<nil> GET/gw=200(10.10.1.1) GET/other=200(aeval-file-backend)
RESULT child_wildcard_host/extopts=custom/listener=wildcard gatewayFallback=false sniOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT child_wildcard_host/extopts=custom/listener=catchall gatewayFallback=false sniOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0 err=<nil> GET/gw=200(10.10.1.1) GET/other=421(Misdirected Request)
RESULT child_wildcard_host/extopts=custom/listener=disjoint_control gatewayFallback=false sniOptions="default" minVersion=0x304 err=<nil> GET/gw=404(404 page not found) GET/other=200(aeval-file-backend)
--- PASS: TestVerify_DescendantTLS (1.22s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.245s
```

## C7

Target: `evalon/traefik-ga-42029a8c` @ 3d6636e9a99259eb08fca72df8e5ba2c45d21a04, worktree `~/repos/wt-42029a8c`.

```sh
cp verify/repro/c7_multihost_fallback_probe_test.go <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerify_MultiHostFallback' ./pkg/provider/kubernetes/gateway
```

Configuration: one Gateway HTTPS listener `*.example.com` on `websecure`; HTTPRoute `mixed` with hostnames `foo.example.com` and `bar.example.com` (`/mixed`); HTTPRoute `baronly` with hostname `bar.example.com` (`/baronly`); both initially use the same dedicated listener TLS options. A file-provider router `ext@file` = `Host(foo.example.com) && Path(/other)` with TLS on `websecure` forces fallback on `foo.example.com`. Everything goes through the production configuration watcher (strict mode disabled), router factory and TLS manager; requests are dispatched in-process with the stated SNI/host.

```console
ROUTER ext@file configured="" resolved="default" status=enabled err=[] rule=Host(`unrelated.test`) && Path(`/other`)
ROUTER httproute-default-http-app-baronly-gw-default-verify-gw-ep-websecure-0-3dc7dae862a9bf2fbc5f@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" status=enabled err=[] rule=Host("bar.example.com") && Path("/baronly")
ROUTER httproute-default-http-app-mixed-gw-default-verify-gw-ep-websecure-0-15dd97ffca440d89148e@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" status=enabled err=[] rule=(Host("foo.example.com") || Host("bar.example.com")) && Path("/mixed")
SNI foo.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
SNI bar.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
SNI baz.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
RESULT fileRouter=false/fileOptions=default GET https://foo.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=false/fileOptions=default GET https://foo.example.com/other -> 404 ()
RESULT fileRouter=false/fileOptions=default GET https://bar.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=false/fileOptions=default GET https://bar.example.com/baronly -> 200 (10.10.2.1)
ROUTER ext@file configured="" resolved="default" status=enabled err=[] rule=Host(`foo.example.com`) && Path(`/other`)
ROUTER httproute-default-http-app-baronly-gw-default-verify-gw-ep-websecure-0-3dc7dae862a9bf2fbc5f@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" status=enabled err=[] rule=Host("bar.example.com") && Path("/baronly")
ROUTER websecure-conflicted-httproute-default-http-app-mixed-gw-default-verify-gw-ep-websecure-0-15dd97ffca440d89148e@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="default" status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead] rule=(Host("foo.example.com") || Host("bar.example.com")) && Path("/mixed")
SNI foo.example.com -> options="default" minVersion=0x304 err=<nil>
SNI bar.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
SNI baz.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
RESULT fileRouter=true/fileOptions=default GET https://foo.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=true/fileOptions=default GET https://foo.example.com/other -> 200 (aeval-file-backend)
RESULT fileRouter=true/fileOptions=default GET https://bar.example.com/mixed -> 421 (Misdirected Request)
RESULT fileRouter=true/fileOptions=default GET https://bar.example.com/baronly -> 200 (10.10.2.1)
ROUTER ext@file configured="custom" resolved="custom@file" status=enabled err=[] rule=Host(`unrelated.test`) && Path(`/other`)
ROUTER httproute-default-http-app-baronly-gw-default-verify-gw-ep-websecure-0-3dc7dae862a9bf2fbc5f@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" status=enabled err=[] rule=Host("bar.example.com") && Path("/baronly")
ROUTER httproute-default-http-app-mixed-gw-default-verify-gw-ep-websecure-0-15dd97ffca440d89148e@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" status=enabled err=[] rule=(Host("foo.example.com") || Host("bar.example.com")) && Path("/mixed")
SNI foo.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
SNI bar.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
SNI baz.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
RESULT fileRouter=false/fileOptions=custom GET https://foo.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=false/fileOptions=custom GET https://foo.example.com/other -> 404 ()
RESULT fileRouter=false/fileOptions=custom GET https://bar.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=false/fileOptions=custom GET https://bar.example.com/baronly -> 200 (10.10.2.1)
ROUTER httproute-default-http-app-baronly-gw-default-verify-gw-ep-websecure-0-3dc7dae862a9bf2fbc5f@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" status=enabled err=[] rule=Host("bar.example.com") && Path("/baronly")
ROUTER websecure-conflicted-ext@file configured="custom" resolved="default" status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead] rule=Host(`foo.example.com`) && Path(`/other`)
ROUTER websecure-conflicted-httproute-default-http-app-mixed-gw-default-verify-gw-ep-websecure-0-15dd97ffca440d89148e@kubernetesgateway configured="listener-example-com-47287a8f16ec75e6073f" resolved="default" status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead] rule=(Host("foo.example.com") || Host("bar.example.com")) && Path("/mixed")
SNI foo.example.com -> options="default" minVersion=0x304 err=<nil>
SNI bar.example.com -> options="default" minVersion=0x304 err=<nil>
SNI baz.example.com -> options="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" minVersion=0x304 err=<nil>
RESULT fileRouter=true/fileOptions=custom GET https://foo.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=true/fileOptions=custom GET https://foo.example.com/other -> 200 (aeval-file-backend)
RESULT fileRouter=true/fileOptions=custom GET https://bar.example.com/mixed -> 200 (10.10.1.1)
RESULT fileRouter=true/fileOptions=custom GET https://bar.example.com/baronly -> 421 (Misdirected Request)
--- PASS: TestVerify_MultiHostFallback (0.21s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.236s
```

Observed (the four blocks are: no file router; file router with default options; non-overlapping file router with custom options; overlapping file router with custom options):
- Without the overlapping file router all Gateway requests return 200 and both Gateway routers resolve to the dedicated listener options.
- With it, the mixed-host router becomes `websecure-conflicted-…mixed…` with `resolved="default"`, while the `bar.example.com`-only router keeps `resolved="listener-example-com-…@kubernetesgateway"`. Fallback does not propagate.
- The two routers serving `bar.example.com` now disagree on TLS options. File router with default options: SNI `bar.example.com` selects the dedicated options and `GET https://bar.example.com/mixed` → `421`. File router with custom options: SNI `bar.example.com` selects `default` and `GET https://bar.example.com/baronly` → `421`.

Impact reasoning: a previously working Gateway route on `bar.example.com` starts returning 421 Misdirected Request as soon as another provider declares a router for `foo.example.com`, a hostname that route does not serve. Which of the two Gateway routes breaks depends on the other provider's TLS options. The trigger needs an HTTPRoute listing two hostnames plus a second route on one of them, which is an ordinary Gateway setup. Workaround: split the multi-host route per hostname or remove the external router.

## C8

Target: `evalon/traefik-ga-ef258f77` @ a296bb162627fb3af9a2f37ac58a92d966a8886e, worktree `~/repos/wt-ef258f77`.

```sh
cp verify/repro/c8_parent_rename_probe_test.go <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/
go test -tags verify -count=1 -v -run 'TestVerify_ParentRename' ./pkg/provider/kubernetes/gateway
```

Configuration: file provider declares `ext-parent@file` (TLS with named options `custom`, entry point `websecure`, `Host(...)`) and `ext-child@file` (`Path(/other)`, `ParentRefs: [ext-parent]`, service). A Gateway HTTPS route serves `foo.example.com` `/gw` on the same entry point. Production configuration watcher with strict mode disabled, then production router-tree parsing and router construction; requests dispatched in-process. The control block gives the parent `Host(unrelated.test)`; the second block gives it `Host(foo.example.com)`.

```console
ROUTER ext-child@file resolvedTLS=- parentRefs=[ext-parent@file] childRefs=[] status=enabled err=[] rule=Path(`/other`)
ROUTER ext-parent@file resolvedTLS=custom@file parentRefs=[] childRefs=[ext-child@file] status=enabled err=[] rule=Host(`unrelated.test`)
ROUTER httproute-default-http-app-gw-gw-default-verify-gw-ep-websecure-0-30daf25dd5b5f7b5d99f@kubernetesgateway resolvedTLS=default parentRefs=[] childRefs=[] status=enabled err=[] rule=Host("foo.example.com") && Path("/gw")
RESULT control_no_overlap sniOptions(unrelated.test)="custom@file" err=<nil> GET https://unrelated.test/other=200(aeval-file-backend) GET https://foo.example.com/gw=200(10.10.1.1)
ROUTER ext-child@file resolvedTLS=- parentRefs=[ext-parent@file] childRefs=[] status=disabled err=[parent router "ext-parent@file" does not exist router is not reachable] rule=Path(`/other`)
ROUTER websecure-conflicted-ext-parent@file resolvedTLS=default parentRefs=[] childRefs=[] status=disabled err=[router has no service and no child routers router must have either a service or child routers router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead] rule=Host(`foo.example.com`)
ROUTER websecure-conflicted-httproute-default-http-app-gw-gw-default-verify-gw-ep-websecure-0-30daf25dd5b5f7b5d99f@kubernetesgateway resolvedTLS=default parentRefs=[] childRefs=[] status=enabled err=[] rule=Host("foo.example.com") && Path("/gw")
RESULT overlap_triggers_fallback sniOptions(foo.example.com)="default" err=<nil> GET https://foo.example.com/other=404(404 page not found) GET https://foo.example.com/gw=200(10.10.1.1)
--- PASS: TestVerify_ParentRename (0.13s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.160s
```

Observed:
- Control (no overlap): parent keeps its name, `childRefs=[ext-child@file]`, `GET https://unrelated.test/other` → 200 from the file backend.
- Overlap: the parent is renamed to `websecure-conflicted-ext-parent@file` (`resolvedTLS=default`), the child still has `parentRefs=[ext-parent@file]`.
- Tree parsing: child `status=disabled err=[parent router "ext-parent@file" does not exist router is not reachable]`; renamed parent `status=disabled err=[router has no service and no child routers …]`.
- `GET https://foo.example.com/other` → `404 page not found`, while the Gateway route `GET https://foo.example.com/gw` → 200.

Impact reasoning: an external parent/child router tree whose TLS parent has named options and shares a host and entry point with a Gateway HTTPS route loses its child route entirely (404) once non-strict fallback applies — the situation the task says must keep serving. Workaround: flatten the external tree or remove the overlap.
