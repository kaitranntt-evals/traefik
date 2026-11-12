# Evidence — run `v-d9c7e7f8`

Observations only; the verdicts are in the report delivered with the session.

## Setup

Audited branch: [`traefik-gateway-listener-isolation-perfect`](https://github.com/kaitranntt-evals/traefik/tree/traefik-gateway-listener-isolation-perfect) at `16bd9c704`. Claim branch for C3: [`evalon/traefik-ga-a4b7c292`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-a4b7c292) at `68e381aaf`. Task base: `35c7530ac0906b5407a3733de173d58d9c427949`. `go version go1.26.3 linux/amd64`.

```sh
cd ~/repos/traefik
git fetch origin traefik-gateway-listener-isolation-perfect
git checkout -b verify/traefik-gateway-listener-isolation-v-d9c7e7f8 origin/traefik-gateway-listener-isolation-perfect
# C3 targets a branch of a sibling repository.
git remote add claims https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation
git fetch claims evalon/traefik-ga-a4b7c292
git worktree add --detach ~/repos/traefik-c3 FETCH_HEAD
# Control: the commit the task started from.
git worktree add --detach ~/repos/traefik-base 35c7530ac0906b5407a3733de173d58d9c427949
# Eval fixtures, byte-exact.
unzip -o eval_tests.zip -d /tmp/ev && sha256sum /tmp/ev/tests/*.go
```

```console
25484ce7712e94124092649ec70355507d1b883b244456eb8c8b9b742e245ad8  /tmp/ev/tests/eval_https_test.go
fb780a907d6c398a60563091f5d3ecc87d29698f91585f37f758bde8e2f88143  /tmp/ev/tests/eval_isolation_test.go
89aa34da72fff7f7e1dcf737585d2bb0ba2fb3cb49632dd9a7bafe29f9f331ba  /tmp/ev/tests/export_eval_test.go
```

How the probes work (`verify/repro/verify_harness_test.go`, build tag `verify`, production code untouched):

- The Kubernetes objects are loaded by the Gateway provider through the helpers of the archived fixtures (`EvalIsoLoadExported`, `EvalIsoHTTPSGatewayExported`, ...), copied byte-exact to `pkg/provider/kubernetes/gateway/`.
- One `dynamic.Message` per provider goes through the production `server.NewConfigurationWatcher` (`mergeConfiguration`, `applyModel`, `resolveHTTPTLSOptions`), with `strictTLSOptions=false`. Lines tagged `AGG` print what the watcher hands to its listeners.
- The result goes through the production `server.NewRouterFactory(...).CreateRouters` (`ParseRouterTree`, handler build). Lines tagged `RUNTIME` print each router status and errors.
- Every request is sent twice: in-process (TLS options picked by SNI, then the production HTTPS handler chain, as the fixtures do) and over a real TLS connection to a Traefik TCP entry point bound on loopback (`server.NewTCPEntryPoints`), with the given SNI in the handshake and the given `Host` header. The stub backend answers `200` with its own IP, so the body names the backend that served the request.
- `verify/repro/run.sh c1c2|c3` copies the fixtures and the probes next to the package and runs `go test -tags verify -count=1 -v -run '^TestVerifyC1C2_'` (or `'^TestVerifyC3_'`) `./pkg/provider/kubernetes/gateway`. Full outputs are in `verify/logs/`.

The probe tests assert the behavior a user expects (route reachable), so a failing probe test is the observation of the problem.

## C1

Claim: non-strict TLS-option conflicts between Gateway listeners and another provider leave Gateway child routers referencing removed listener parent names (*reference-integrity*) and make their attached routes unavailable (*route-availability*). Adjudicated on [`traefik-gateway-listener-isolation-perfect`](https://github.com/kaitranntt-evals/traefik/tree/traefik-gateway-listener-isolation-perfect).

Configuration: one Gateway, HTTPS listeners `precise` (`abc.example.com`) and `sibling` (`xyz.example.com`) on entry point `websecure`; HTTPRoute `http-app-precise` (`/gw`) and GRPCRoute `grpc-app-precise` (`SayHello`) attached to `precise`; HTTPRoute `http-app-sibling` attached to `sibling`. A second provider, `file`, declares one HTTPS router `other-app` on `websecure` with rule `Host("abc.example.com") && PathPrefix("/file")`, with the `default` TLS options (first case) or its own TLS options `modern` (second case).

```sh
cd ~/repos/traefik   # traefik-gateway-listener-isolation-perfect, 16bd9c704
EVAL_TESTS=/tmp/ev/tests verify/repro/run.sh c1c2 > verify/logs/c1c2_perfect.log 2>&1; echo exit=$?
```

```console
exit=1
--- PASS: TestVerifyC1C2_Control_GatewayAlone (0.09s)
--- PASS: TestVerifyC1C2_Control_OtherProviderDifferentHost (0.06s)
--- FAIL: TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions (0.03s)
--- FAIL: TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions (0.06s)
--- FAIL: TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard (0.06s)
--- PASS: TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions (0.04s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.371s
FAIL
```

### Reference-integrity

What the Gateway provider emits (`TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions`): the children carry no entry point and no TLS, only `parentRefs` naming the listener router, which holds the dedicated TLS options.

```console
PROVIDER(kubernetesgateway) router "grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016"
              rule=Host("abc.example.com") && PathRegexp("/helloworld\\.Greeter/SayHello") eps=[] service="grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016-wrr" parentRefs=[listener-websecure-https-abc-example-com-84b9126a78b887b479e2] tls=nil
PROVIDER(kubernetesgateway) router "httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08"
              rule=Host("abc.example.com") && Path("/gw") eps=[] service="httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08-wrr" parentRefs=[listener-websecure-https-abc-example-com-84b9126a78b887b479e2] tls=nil
PROVIDER(kubernetesgateway) router "httproute-default-http-app-sibling-gw-default-my-gateway-ep-websecure-0-786db502b3b7f5ee97a0"
              rule=Host("xyz.example.com") && Path("/gw") eps=[] service="httproute-default-http-app-sibling-gw-default-my-gateway-ep-websecure-0-786db502b3b7f5ee97a0-wrr" parentRefs=[listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6] tls=nil
PROVIDER(kubernetesgateway) router "listener-websecure-https-abc-example-com-84b9126a78b887b479e2"
              rule=Host("abc.example.com") eps=[websecure] service="" parentRefs=[] tls.options="listener-websecure-https-abc-example-com-84b9126a78b887b479e2"
PROVIDER(kubernetesgateway) router "listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6"
              rule=Host("xyz.example.com") eps=[websecure] service="" parentRefs=[] tls.options="listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6"
```

The conflict Traefik reports while aggregating both providers (log line of the same test):

```console
error: On EntryPoint "websecure", Host "abc.example.com" is served by multiple routers with different TLS options, default TLSOptions will be applied for the following routers: [other-app@file listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway]
```

What the configuration watcher hands over after `resolveHTTPTLSOptions`: the listener router now only exists as `websecure-conflicted-listener-...`, and both children still name the removed router.

```console
AGG router "grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016@kubernetesgateway"
              rule=Host("abc.example.com") && PathRegexp("/helloworld\\.Greeter/SayHello") priority=59 eps=[] service="grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016-wrr" tls=nil
      parentRef "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" existsAfterAggregation=false
AGG router "httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08@kubernetesgateway"
              rule=Host("abc.example.com") && Path("/gw") priority=100016 eps=[] service="httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08-wrr" tls=nil
      parentRef "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" existsAfterAggregation=false
AGG router "httproute-default-http-app-sibling-gw-default-my-gateway-ep-websecure-0-786db502b3b7f5ee97a0@kubernetesgateway"
              rule=Host("xyz.example.com") && Path("/gw") priority=100016 eps=[] service="httproute-default-http-app-sibling-gw-default-my-gateway-ep-websecure-0-786db502b3b7f5ee97a0-wrr" tls=nil
      parentRef "listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6@kubernetesgateway" existsAfterAggregation=true
AGG router "listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6@kubernetesgateway"
              rule=Host("xyz.example.com") priority=23 eps=[websecure] service="" tls.options="listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6" resolved="listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6@kubernetesgateway" conflicting=false
AGG router "websecure-conflicted-listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway"
              rule=Host("abc.example.com") priority=23 eps=[websecure] service="" tls.options="listener-websecure-https-abc-example-com-84b9126a78b887b479e2" resolved="default" conflicting=false
AGG router "websecure-conflicted-other-app@file"
              rule=Host("abc.example.com") && PathPrefix("/file") priority=46 eps=[websecure] service="other-svc" tls.options="" resolved="default" conflicting=false
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=1 dangling=2
```

Same outcome when the other provider uses its own TLS options (`TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions`):

```console
      parentRef "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" existsAfterAggregation=false
      parentRef "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" existsAfterAggregation=false
AGG router "websecure-conflicted-listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway"
              rule=Host("abc.example.com") priority=23 eps=[websecure] service="" tls.options="listener-websecure-https-abc-example-com-84b9126a78b887b479e2" resolved="default" conflicting=false
AGG router "websecure-conflicted-other-app@file"
              rule=Host("abc.example.com") && PathPrefix("/file") priority=46 eps=[websecure] service="other-svc" tls.options="modern" resolved="default" conflicting=false
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=1 dangling=2
```

### Route-availability

`ParseRouterTree` (through `RouterFactory.CreateRouters`) disables both children, and the renamed parent, now childless, is disabled as well (`TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions`):

```console
RUNTIME router "grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016@kubernetesgateway" status=disabled childRefs=[] err=[parent router "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" does not exist router is not reachable]
RUNTIME router "httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08@kubernetesgateway" status=disabled childRefs=[] err=[parent router "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" does not exist router is not reachable]
RUNTIME router "httproute-default-http-app-sibling-gw-default-my-gateway-ep-websecure-0-786db502b3b7f5ee97a0@kubernetesgateway" status=enabled childRefs=[] err=[]
RUNTIME router "listener-websecure-https-xyz-example-com-114b6dff035ed5d61ae6@kubernetesgateway" status=enabled childRefs=[httproute-default-http-app-sibling-gw-default-my-gateway-ep-websecure-0-786db502b3b7f5ee97a0@kubernetesgateway] err=[]
RUNTIME router "websecure-conflicted-listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" status=disabled childRefs=[] err=[router has no service and no child routers router must have either a service or child routers router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
RUNTIME router "websecure-conflicted-other-app@file" status=enabled childRefs=[] err=[]
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 2
```

Requests, in-process and over real TLS. The HTTPRoute and the GRPCRoute of the conflicting listener answer 404; the other provider's router and the sibling listener keep working:

```console
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 2
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 404 "404 page not found" | real TLS 404 "404 page not found"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 404 "404 page not found" | real TLS 404 "404 page not found"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 2
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 404 "404 page not found" | real TLS 404 "404 page not found"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 404 "404 page not found" | real TLS 404 "404 page not found"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
```

### Controls

Same Gateway without a second provider, with a second provider on another host name, and with a second provider whose router points at the TLS options the Gateway provider generated for the listener (no conflict in any of the three): every route answers 200.

```console
# TestVerifyC1C2_Control_GatewayAlone
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
# TestVerifyC1C2_Control_OtherProviderDifferentHost
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=unrelated.example.com Host=unrelated.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
# TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
```

Same probes on the task base commit, where Gateway routers are flat (no parent, no dedicated TLS options). With `default` options on both sides there is no conflict; with `modern` on the other provider the conflict is detected, the routers are renamed, and the Gateway routes still answer 200 through the default-options fallback:

```sh
cd ~/repos/traefik-base   # 35c7530ac
EVAL_TESTS=/tmp/ev/tests ~/repos/traefik/verify/repro/run.sh c1c2 > ~/repos/traefik/verify/logs/c1c2_base_35c7530ac.log 2>&1; echo exit=$?
```

```console
exit=0
--- PASS: TestVerifyC1C2_Control_GatewayAlone (0.17s)
--- PASS: TestVerifyC1C2_Control_OtherProviderDifferentHost (0.05s)
--- PASS: TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions (0.02s)
--- PASS: TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions (0.06s)
--- PASS: TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard (0.05s)
--- SKIP: TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.399s
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=0 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=0 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions, aggregated routers
AGG router "websecure-conflicted-grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016@kubernetesgateway"
              rule=Host("abc.example.com") && PathRegexp("/helloworld\\.Greeter/SayHello") priority=59 eps=[websecure] service="grpcroute-default-grpc-app-precise-gw-default-my-gateway-ep-websecure-0-c64af9e9295b12ea4016-wrr" tls.options="" resolved="default" conflicting=false
AGG router "websecure-conflicted-httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08@kubernetesgateway"
              rule=Host("abc.example.com") && Path("/gw") priority=100016 eps=[websecure] service="httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08-wrr" tls.options="" resolved="default" conflicting=false
AGG router "websecure-conflicted-other-app@file"
              rule=Host("abc.example.com") && PathPrefix("/file") priority=46 eps=[websecure] service="other-svc" tls.options="modern" resolved="default" conflicting=false
```

Diagnostic mutation (not committed to production code, reverted after the run): rewriting `ParentRefs` to the renamed parents inside `resolveHTTPTLSOptions` makes both conflict cases pass, which ties the 404 to the stale references.

```diff
diff --git a/pkg/server/aggregator.go b/pkg/server/aggregator.go
index 357bb1f02..0838364ee 100644
--- a/pkg/server/aggregator.go
+++ b/pkg/server/aggregator.go
@@ -226,6 +226,7 @@ func resolveHTTPTLSOptions(routers map[string]*dynamic.Router, strictTLSOptions
 		conflictingRouters[ep] = findConflictingRouters(ep, epRouters, strictTLSOptions)
 	}
 
+	renamed := map[string][]string{}
 	for name, router := range routers {
 		router.EntryPoints = slices.DeleteFunc(router.EntryPoints, func(ep string) bool {
 			deleted := slices.Contains(conflictingRouters[ep], name)
@@ -240,6 +241,7 @@ func resolveHTTPTLSOptions(routers map[string]*dynamic.Router, strictTLSOptions
 				// The new name is not collision free but has very small possibility to collide.
 				// TODO: rework this naming whenever we'll introduce a resource reference mechanism not based on a string.
 				newRouters[ep+"-conflicted-"+name] = rt
+				renamed[name] = append(renamed[name], ep+"-conflicted-"+name)
 			}
 
 			return deleted
@@ -250,6 +252,20 @@ func resolveHTTPTLSOptions(routers map[string]*dynamic.Router, strictTLSOptions
 		}
 	}
 
+	for _, router := range newRouters {
+		var parentRefs []string
+		for _, ref := range router.ParentRefs {
+			names, ok := renamed[ref]
+			if _, exists := newRouters[ref]; exists || !ok {
+				parentRefs = append(parentRefs, ref)
+			}
+			parentRefs = append(parentRefs, names...)
+		}
+		if router.ParentRefs != nil {
+			router.ParentRefs = parentRefs
+		}
+	}
+
 	return newRouters
 }
```

```sh
cd ~/repos/traefik
git apply verify/repro/c1c2_parentref_rewrite_mutation.diff
EVAL_TESTS=/tmp/ev/tests verify/repro/run.sh c1c2 > verify/logs/c1c2_perfect_parentref_rewrite_mutation.log 2>&1; echo exit=$?
git apply -R verify/repro/c1c2_parentref_rewrite_mutation.diff
```

```console
exit=1
--- PASS: TestVerifyC1C2_Control_GatewayAlone (0.18s)
--- PASS: TestVerifyC1C2_Control_OtherProviderDifferentHost (0.02s)
--- PASS: TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions (0.03s)
--- PASS: TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions (0.03s)
--- FAIL: TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard (0.06s)
--- PASS: TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions (0.08s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.432s
FAIL
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
```

The archived eval fixtures do not see this: they only feed the `kubernetesgateway` and `internal` providers to the watcher (`eval_https_test.go:37-39`) and pass on the audited branch.

```sh
cd ~/repos/traefik && go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway
```

```console
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	4.503s
```

### Impact reasoning

- The Gateway provider of the audited branch gives every HTTPS listener its own generated TLS options (`listener-websecure-https-abc-example-com-84b9126a78b887b479e2`). Any HTTPS router of another provider on the same entry point and the same host name therefore differs in TLS options, including one that sets no TLS options at all. The conflict needs no unusual configuration on the other side.
- Non-strict mode is the default. `docs/content/migrate/v3.md` documents it as: Traefik "falls back to the `default` TLS options for that server name", and only `core.strictTLSOptions` "disables the routers involved". The observed behavior in non-strict mode is the strict one for the Gateway side only: the HTTPRoute and GRPCRoute of the listener answer 404 while the other provider's router keeps answering 200.
- On the base commit the same two-provider setup serves the Gateway routes (200), so this is a regression introduced by the listener parent routers, not a pre-existing limitation.
- Signals observed when it happens: the generic TLS-options conflict log line (error level, it announces the `default` fallback, not a disabled route) and the `parent router ... does not exist` / `router is not reachable` errors on the child routers in the runtime configuration.
- Observed workarounds: keep other providers off the listener host name on that entry point (control 2), or make the other router reference the generated listener TLS options by name (control 3; the name embeds a hash of the listener).
- Who is hit: a setup where a host name is served both by a Gateway HTTPS listener and by a router of another provider on the same entry point, for example while moving a host to Gateway API. The probe used a provider named `file`; the conflict line above keys on entry point, host name and TLS options, not on the provider.

Adjacent observation, outside the claim (`TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard`): with a wildcard listener `*.example.com` and another provider serving `abc.example.com`, `resolveHTTPTLSOptions` reports no conflict and no `ParentRefs` dangle, but every request for `abc.example.com` answers `421`, including the other provider's own router. It passes on the base commit and is not fixed by the mutation above.

```console
# TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 421 "Misdirected Request" | real TLS 421 "Misdirected Request"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=abc.example.com Host=abc.example.com path=/helloworld.Greeter/SayHello -> in-process 421 "Misdirected Request" | real TLS 421 "Misdirected Request"   (Gateway GRPCRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 421 "Misdirected Request" | real TLS 421 "Misdirected Request"   (other provider's router)
```

## C2

Claim: a same-host, non-strict TLS-option conflict between a Gateway listener and another provider makes the Gateway HTTPRoute unavailable because its child router references a removed listener parent (*reference-integrity*, *route-availability*). Adjudicated on [`traefik-gateway-listener-isolation-perfect`](https://github.com/kaitranntt-evals/traefik/tree/traefik-gateway-listener-isolation-perfect). This is the HTTPRoute, same-host case of the run used for C1; the key lines are repeated here.

Configuration: one Gateway, HTTPS listeners `precise` (`abc.example.com`) and `sibling` (`xyz.example.com`) on entry point `websecure`; HTTPRoute `http-app-precise` (`/gw`) and GRPCRoute `grpc-app-precise` (`SayHello`) attached to `precise`; HTTPRoute `http-app-sibling` attached to `sibling`. A second provider, `file`, declares one HTTPS router `other-app` on `websecure` with rule `Host("abc.example.com") && PathPrefix("/file")`, with the `default` TLS options (first case) or its own TLS options `modern` (second case).

```sh
cd ~/repos/traefik   # traefik-gateway-listener-isolation-perfect, 16bd9c704
EVAL_TESTS=/tmp/ev/tests verify/repro/run.sh c1c2 > verify/logs/c1c2_perfect.log 2>&1; echo exit=$?
```

```console
exit=1
--- PASS: TestVerifyC1C2_Control_GatewayAlone (0.09s)
--- PASS: TestVerifyC1C2_Control_OtherProviderDifferentHost (0.06s)
--- FAIL: TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions (0.03s)
--- FAIL: TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions (0.06s)
--- FAIL: TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard (0.06s)
--- PASS: TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions (0.04s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.371s
FAIL
```

### Reference-integrity

`loadHTTPRoute` output for HTTPRoute `http-app-precise` and its listener (`TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions`):

```console
PROVIDER(kubernetesgateway) router "httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08"
              rule=Host("abc.example.com") && Path("/gw") eps=[] service="httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08-wrr" parentRefs=[listener-websecure-https-abc-example-com-84b9126a78b887b479e2] tls=nil
PROVIDER(kubernetesgateway) router "listener-websecure-https-abc-example-com-84b9126a78b887b479e2"
              rule=Host("abc.example.com") eps=[websecure] service="" parentRefs=[] tls.options="listener-websecure-https-abc-example-com-84b9126a78b887b479e2"
```

Traefik log line for the cross-provider conflict on the same host name:

```console
error: On EntryPoint "websecure", Host "abc.example.com" is served by multiple routers with different TLS options, default TLSOptions will be applied for the following routers: [other-app@file listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway]
```

After aggregation the HTTPRoute child still names the router that `resolveHTTPTLSOptions` replaced by `websecure-conflicted-...`:

```console
AGG router "httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08@kubernetesgateway"
              rule=Host("abc.example.com") && Path("/gw") priority=100016 eps=[] service="httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08-wrr" tls=nil
      parentRef "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" existsAfterAggregation=false
AGG router "websecure-conflicted-listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway"
              rule=Host("abc.example.com") priority=23 eps=[websecure] service="" tls.options="listener-websecure-https-abc-example-com-84b9126a78b887b479e2" resolved="default" conflicting=false
AGG router "websecure-conflicted-other-app@file"
              rule=Host("abc.example.com") && PathPrefix("/file") priority=46 eps=[websecure] service="other-svc" tls.options="" resolved="default" conflicting=false
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=1 dangling=2
```

### Route-availability

```console
RUNTIME router "httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08@kubernetesgateway" status=disabled childRefs=[] err=[parent router "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" does not exist router is not reachable]
RUNTIME router "websecure-conflicted-listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" status=disabled childRefs=[] err=[router has no service and no child routers router must have either a service or child routers router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 2
```

The HTTPRoute answers 404, in-process and over real TLS, with `default` TLS options on the other provider and with custom ones:

```console
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 404 "404 page not found" | real TLS 404 "404 page not found"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 404 "404 page not found" | real TLS 404 "404 page not found"   (Gateway HTTPRoute on the conflicting listener)
REQUEST SNI=xyz.example.com Host=xyz.example.com path=/gw                          -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (Gateway HTTPRoute on the sibling listener (no conflict))
REQUEST SNI=abc.example.com Host=abc.example.com path=/file                        -> in-process 200 "10.99.0.1" | real TLS 200 "10.99.0.1"   (other provider's router)
```

### Controls

No second provider, second provider on another host name, second provider reusing the listener TLS options: the HTTPRoute answers 200.

```console
# TestVerifyC1C2_Control_GatewayAlone
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
# TestVerifyC1C2_Control_OtherProviderDifferentHost
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
# TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
```

Task base commit, same probes: the HTTPRoute answers 200 in both cases, through the default-options fallback when the conflict exists.

```sh
cd ~/repos/traefik-base   # 35c7530ac
EVAL_TESTS=/tmp/ev/tests ~/repos/traefik/verify/repro/run.sh c1c2 > ~/repos/traefik/verify/logs/c1c2_base_35c7530ac.log 2>&1; echo exit=$?
```

```console
exit=0
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions, aggregated HTTPRoute router
AGG router "websecure-conflicted-httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08@kubernetesgateway"
              rule=Host("abc.example.com") && Path("/gw") priority=100016 eps=[websecure] service="httproute-default-http-app-precise-gw-default-my-gateway-ep-websecure-0-f135e3391225e82b3b08-wrr" tls.options="" resolved="default" conflicting=false
```

Diagnostic mutation (reverted after the run): with `ParentRefs` rewritten to the renamed parents in `resolveHTTPTLSOptions` (`verify/repro/c1c2_parentref_rewrite_mutation.diff`), the same HTTPRoute answers 200.

```sh
cd ~/repos/traefik
git apply verify/repro/c1c2_parentref_rewrite_mutation.diff
EVAL_TESTS=/tmp/ev/tests verify/repro/run.sh c1c2 > verify/logs/c1c2_perfect_parentref_rewrite_mutation.log 2>&1; echo exit=$?
git apply -R verify/repro/c1c2_parentref_rewrite_mutation.diff
```

```console
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
# TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions
REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=3 dangling=0
ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: 0
REQUEST SNI=abc.example.com Host=abc.example.com path=/gw                          -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (Gateway HTTPRoute on the conflicting listener)
```

### Impact reasoning

- A Gateway HTTPRoute stops being served (404) as soon as any other provider declares an HTTPS router for the listener host name on the same entry point, because the listener's generated TLS options never equal the other router's. The other provider's router keeps working, so the breakage is one-sided.
- `docs/content/migrate/v3.md` promises the `default` TLS options fallback in non-strict mode; the base commit honors it for this exact setup (200), the audited branch does not (404).
- C2 is the HTTPRoute instance of the mechanism shown in C1; one fix in `resolveHTTPTLSOptions` (or in how children name their parents) covers both.
- The archived eval fixtures pass on the audited branch and never aggregate a second routing provider, so no existing check covers this.

## C3

Claim: with listeners `*.example.com`, `*.foo.example.com` and `abc.foo.example.com`, dedicated TLS options and no catch-all listener, an HTTPS request whose SNI and Host are both `abc.foo.example.com` is rejected as misdirected instead of reaching the exact-host listener's route. Adjudicated on [`evalon/traefik-ga-a4b7c292`](https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation/tree/evalon/traefik-ga-a4b7c292) (`68e381aaf`), in a separate worktree.

Configuration: one Gateway, HTTPS listeners `wild` (`*.example.com`), `foo` (`*.foo.example.com`), `exact` (`abc.foo.example.com`) on `websecure`, one HTTPRoute per listener (`/wild`, `/foo`, `/exact` → backends `10.10.2.1`, `10.10.0.1`, `10.10.1.1`).

Naming and setup discrepancy: the claim branch does not generate per-listener TLS options (no `SNICheck`, no TLS options in the Gateway provider; misdirected requests are caught by `HostSNI` matchers on dedicated `...-misdirected` routers). The first test below runs the provider output as is; the second injects one TLS options set per listener host name into that output so that the "dedicated TLS options" condition of the claim also holds.

```sh
cd ~/repos/traefik-c3   # evalon/traefik-ga-a4b7c292, 68e381aaf
grep -c -i "SNICheck\|TLS.Options" pkg/provider/kubernetes/gateway/kubernetes.go pkg/provider/kubernetes/gateway/httproute.go
EVAL_TESTS=/tmp/ev/tests ~/repos/traefik/verify/repro/run.sh c3 > ~/repos/traefik/verify/logs/c3_claim_branch.log 2>&1; echo exit=$?
```

```console
pkg/provider/kubernetes/gateway/kubernetes.go:0
pkg/provider/kubernetes/gateway/httproute.go:0
exit=1
--- FAIL: TestVerifyC3_ThreeListenersNoCatchAll (0.10s)
--- FAIL: TestVerifyC3_ThreeListenersNoCatchAll_DedicatedTLSOptions (0.06s)
--- PASS: TestVerifyC3_Control_TwoListeners_FooWildcard (0.04s)
--- PASS: TestVerifyC3_Control_TwoListeners_OuterWildcard (0.06s)
--- PASS: TestVerifyC3_Control_ThreeListenersPlusCatchAll (0.04s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.328s
FAIL
```

Routers the Gateway provider emits for the claim topology (`TestVerifyC3_ThreeListenersNoCatchAll`). The rule of the `exact` listener's misdirected router is `Host && (!SNI(exact) && SNI(wild) || SNI(foo))`; `&&` binds tighter than `||`, so it also matches every request whose SNI is under `*.foo.example.com`, which includes `abc.foo.example.com` itself:

```console
PROVIDER router "gateway-default-my-gateway-listener-exact-misdirected"
              rule=Host("abc.foo.example.com") && (!HostSNI("abc.foo.example.com") && HostSNI("**.example.com") || HostSNI("**.foo.example.com"))
PROVIDER router "gateway-default-my-gateway-listener-foo-misdirected"
              rule=Host("**.foo.example.com") && !Host("abc.foo.example.com") && (HostSNI("abc.foo.example.com") || (!HostSNI("**.foo.example.com") && HostSNI("**.example.com")))
PROVIDER router "gateway-default-my-gateway-listener-wild-misdirected"
              rule=Host("**.example.com") && !Host("**.foo.example.com") && !Host("abc.foo.example.com") && (HostSNI("**.foo.example.com") || HostSNI("abc.foo.example.com"))
PROVIDER router "httproute-default-route-exact-gw-default-my-gateway-ep-websecure-0-3891e3b40eb56b7ac2d7"
              rule=Host("abc.foo.example.com") && Path("/exact")
PROVIDER router "httproute-default-route-foo-gw-default-my-gateway-ep-websecure-0-7ccc044068c0c6ada3b1"
              rule=Host("**.foo.example.com") && Path("/foo") && !Host("abc.foo.example.com")
PROVIDER router "httproute-default-route-wild-gw-default-my-gateway-ep-websecure-0-cc1c32ba9eb921251f14"
              rule=Host("**.example.com") && Path("/wild") && !Host("**.foo.example.com") && !Host("abc.foo.example.com")
```

Requests, in-process and over real TLS:

```console
# TestVerifyC3_ThreeListenersNoCatchAll
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 421 "" | real TLS 421 ""   (CLAIM: SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/nope   -> in-process 421 "" | real TLS 421 ""   (same listener, unmatched path; spec-expected 404)
REQUEST SNI=xyz.foo.example.com    Host=xyz.foo.example.com    path=/foo    -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (SNI == Host on *.foo.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=xyz.example.com        path=/wild   -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (SNI == Host on *.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=abc.foo.example.com    path=/exact  -> in-process 421 "" | real TLS 421 ""   (crossed: SNI wild listener, Host exact listener; spec-expected 421)
REQUEST SNI=abc.foo.example.com    Host=xyz.example.com        path=/wild   -> in-process 421 "" | real TLS 421 ""   (crossed: SNI exact listener, Host wild listener; spec-expected 421)
# TestVerifyC3_ThreeListenersNoCatchAll_DedicatedTLSOptions
PROVIDER tls.options "dedicated-wildcardwildcard-foo-example-com"
PROVIDER tls.options "dedicated-abc-foo-example-com"
PROVIDER tls.options "dedicated-wildcardwildcard-example-com"
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 421 "" | real TLS 421 ""   (CLAIM: SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=xyz.foo.example.com    Host=xyz.foo.example.com    path=/foo    -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (SNI == Host on *.foo.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=xyz.example.com        path=/wild   -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (SNI == Host on *.example.com listener; spec-expected 200)
```

The equal SNI/Host request for the exact listener answers `421` with an empty body instead of `200` from `10.10.1.1`; an unmatched path on that host answers `421` instead of `404`. The two wildcard listeners serve their own hosts.

Controls on the same branch, same exact listener and route: with only one wildcard sibling, or with a catch-all listener added, the request answers 200, and crossed SNI/Host still answers 421.

```console
# TestVerifyC3_Control_TwoListeners_FooWildcard
PROVIDER router "gateway-default-my-gateway-listener-exact-misdirected"
              rule=Host("abc.foo.example.com") && (!HostSNI("abc.foo.example.com") && HostSNI("**.foo.example.com"))
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=xyz.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 421 "" | real TLS 421 ""   (crossed; spec-expected 421)
# TestVerifyC3_Control_TwoListeners_OuterWildcard
PROVIDER router "gateway-default-my-gateway-listener-exact-misdirected"
              rule=Host("abc.foo.example.com") && (!HostSNI("abc.foo.example.com") && HostSNI("**.example.com"))
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=abc.foo.example.com    path=/exact  -> in-process 421 "" | real TLS 421 ""   (crossed; spec-expected 421)
# TestVerifyC3_Control_ThreeListenersPlusCatchAll
PROVIDER router "gateway-default-my-gateway-listener-exact-misdirected"
              rule=Host("abc.foo.example.com") && (!HostSNI("abc.foo.example.com") && HostSNIRegexp("^.+$"))
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (SNI == Host == exact listener hostname; spec-expected 200)
```

Diagnostic mutation in the claim worktree (reverted after the run): grouping the `otherSNIs` disjunction makes every probe pass, which isolates the rejection to that expression.

```diff
diff --git a/pkg/provider/kubernetes/gateway/kubernetes.go b/pkg/provider/kubernetes/gateway/kubernetes.go
index 7d9d9b0f9..8114c8535 100644
--- a/pkg/provider/kubernetes/gateway/kubernetes.go
+++ b/pkg/provider/kubernetes/gateway/kubernetes.go
@@ -786,7 +786,7 @@ func buildMisdirectedRequestRule(listener gatewayListener) string {
 	}
 
 	if listener.Hostname != nil && len(otherSNIs) > 0 {
-		misdirectedSNIs = append(misdirectedSNIs, fmt.Sprintf("(!%s && %s)", buildHostSNIRule(*listener.Hostname), strings.Join(otherSNIs, " || ")))
+		misdirectedSNIs = append(misdirectedSNIs, fmt.Sprintf("(!%s && (%s))", buildHostSNIRule(*listener.Hostname), strings.Join(otherSNIs, " || ")))
 	}
 
 	if len(misdirectedSNIs) == 0 {
```

```sh
cd ~/repos/traefik-c3
git apply ~/repos/traefik/verify/repro/c3_parenthesize_mutation.diff
EVAL_TESTS=/tmp/ev/tests ~/repos/traefik/verify/repro/run.sh c3 > ~/repos/traefik/verify/logs/c3_claim_branch_parenthesized_mutation.log 2>&1; echo exit=$?
git apply -R ~/repos/traefik/verify/repro/c3_parenthesize_mutation.diff
```

```console
exit=0
--- PASS: TestVerifyC3_ThreeListenersNoCatchAll (0.09s)
--- PASS: TestVerifyC3_ThreeListenersNoCatchAll_DedicatedTLSOptions (0.04s)
--- PASS: TestVerifyC3_Control_TwoListeners_FooWildcard (0.08s)
--- PASS: TestVerifyC3_Control_TwoListeners_OuterWildcard (0.08s)
--- PASS: TestVerifyC3_Control_ThreeListenersPlusCatchAll (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.349s
PROVIDER router "gateway-default-my-gateway-listener-exact-misdirected"
              rule=Host("abc.foo.example.com") && (!HostSNI("abc.foo.example.com") && (HostSNI("**.example.com") || HostSNI("**.foo.example.com")))
# TestVerifyC3_ThreeListenersNoCatchAll
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (CLAIM: SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/nope   -> in-process 404 "404 page not found\n" | real TLS 404 "404 page not found"   (same listener, unmatched path; spec-expected 404)
REQUEST SNI=xyz.foo.example.com    Host=xyz.foo.example.com    path=/foo    -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (SNI == Host on *.foo.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=xyz.example.com        path=/wild   -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (SNI == Host on *.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=abc.foo.example.com    path=/exact  -> in-process 421 "" | real TLS 421 ""   (crossed: SNI wild listener, Host exact listener; spec-expected 421)
REQUEST SNI=abc.foo.example.com    Host=xyz.example.com        path=/wild   -> in-process 421 "" | real TLS 421 ""   (crossed: SNI exact listener, Host wild listener; spec-expected 421)
# TestVerifyC3_ThreeListenersNoCatchAll_DedicatedTLSOptions
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (CLAIM: SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=xyz.foo.example.com    Host=xyz.foo.example.com    path=/foo    -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (SNI == Host on *.foo.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=xyz.example.com        path=/wild   -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (SNI == Host on *.example.com listener; spec-expected 200)
```

For reference, the same probes on the audited branch [`traefik-gateway-listener-isolation-perfect`](https://github.com/kaitranntt-evals/traefik/tree/traefik-gateway-listener-isolation-perfect) (a different implementation, with real per-listener TLS options) pass:

```sh
cd ~/repos/traefik
EVAL_TESTS=/tmp/ev/tests verify/repro/run.sh c3 > verify/logs/c3_perfect_control.log 2>&1; echo exit=$?
```

```console
exit=0
--- PASS: TestVerifyC3_ThreeListenersNoCatchAll (0.15s)
--- PASS: TestVerifyC3_ThreeListenersNoCatchAll_DedicatedTLSOptions (0.05s)
--- PASS: TestVerifyC3_Control_TwoListeners_FooWildcard (0.02s)
--- PASS: TestVerifyC3_Control_TwoListeners_OuterWildcard (0.08s)
--- PASS: TestVerifyC3_Control_ThreeListenersPlusCatchAll (0.05s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.399s
# TestVerifyC3_ThreeListenersNoCatchAll
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/exact  -> in-process 200 "10.10.1.1" | real TLS 200 "10.10.1.1"   (CLAIM: SNI == Host == exact listener hostname; spec-expected 200)
REQUEST SNI=abc.foo.example.com    Host=abc.foo.example.com    path=/nope   -> in-process 404 "404 page not found\n" | real TLS 404 "404 page not found"   (same listener, unmatched path; spec-expected 404)
REQUEST SNI=xyz.foo.example.com    Host=xyz.foo.example.com    path=/foo    -> in-process 200 "10.10.0.1" | real TLS 200 "10.10.0.1"   (SNI == Host on *.foo.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=xyz.example.com        path=/wild   -> in-process 200 "10.10.2.1" | real TLS 200 "10.10.2.1"   (SNI == Host on *.example.com listener; spec-expected 200)
REQUEST SNI=xyz.example.com        Host=abc.foo.example.com    path=/exact  -> in-process 421 "Misdirected Request\n" | real TLS 421 "Misdirected Request"   (crossed: SNI wild listener, Host exact listener; spec-expected 421)
REQUEST SNI=abc.foo.example.com    Host=xyz.example.com        path=/wild   -> in-process 421 "Misdirected Request\n" | real TLS 421 "Misdirected Request"   (crossed: SNI exact listener, Host wild listener; spec-expected 421)
```

The archived eval fixtures fail on the claim branch for other reasons (`verify/logs/eval_fixtures_claim_branch_c3.log`: `SingleGatewaySpecificity/rejected_listener_not_suppressing`, `DetectMisdirectedRequests`, `SiblingGatewayReachability`); none of their failure messages concerns `abc.foo.example.com`, and no fixture builds this HTTPS topology without a catch-all, so the verdict rests on the probes above.

```sh
cd ~/repos/traefik-c3 && go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway > ~/repos/traefik/verify/logs/eval_fixtures_claim_branch_c3.log 2>&1; echo exit=$?
grep -v '^{' ~/repos/traefik/verify/logs/eval_fixtures_claim_branch_c3.log | grep -c "abc.foo.example.com"
```

```console
exit=1
0
```

### Impact reasoning

- On the claim branch, a Gateway that declares an exact-host HTTPS listener next to two or more other hostname listeners, without a catch-all, cannot serve the exact host at all: every request for it answers `421 Misdirected Request`, whatever the path, although SNI and Host agree and select that listener.
- The trigger is the number of sibling hostnames: with exactly one sibling the generated expression has a single term and is correct, with a catch-all sibling the "other SNI" side collapses to one `HostSNIRegexp` term and is correct. Two named siblings whose patterns cover the exact host are enough to break it.
- Every real-TLS probe used a fresh connection with the matching SNI and still got `421`, so retrying on a new connection does not help. Observed workaround: add a catch-all listener (control 3).
- The other listeners are not affected for their own hosts, and crossed SNI/Host requests are still rejected, so the defect is an over-rejection, not a leak across listeners.
