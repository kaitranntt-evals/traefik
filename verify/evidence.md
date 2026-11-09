# Evidence — run v-37f11744

Observations only. Every command below was run in this session with go1.26.3 linux/amd64. Raw output of the two repro scripts is committed under `verify/logs/`.

Claim-target branches live in the `kaitranntt-evals/traefik-gateway-listener-isolation` repository and were checked out as detached worktrees next to the main checkout:

- C1: `evalon/traefik-ga-730e07a9` at `418e22c75447b0d5e416df4afec8413d4fdd1aaa`
- C2: `evalon/traefik-ga-968ccc91` at `838f355f034a601abbf0450f8afc70969a31ef55`
- Audited branch `traefik-gateway-listener-isolation-perfect` at `16bd9c70425e946d64be29de626ab91d7f2a1edd` (used only as a contrast run in C2)

Eval fixtures (`eval_tests.zip`, extracted byte-exact):

```console
8c529208091b36d2d93e69ba279f83f64265e757e1057965435ccd92f28c07da  tests/eval_https_test.go
51c7ae8046b66c5893a2b50f0b0f4b3e560328e36cacbd3228422183b2cda8bb  tests/eval_isolation_test.go
89aa34da72fff7f7e1dcf737585d2bb0ba2fb3cb49632dd9a7bafe29f9f331ba  tests/export_eval_test.go
```

Checks on the verification branch itself (production code untouched):

```console
$ go vet ./pkg/provider/kubernetes/gateway/... && echo VET_OK
VET_OK
$ go test -run='^$' ./pkg/provider/kubernetes/gateway/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.014s [no tests to run]
```

## C1

Claim: the four-listener status test under `pkg/provider/kubernetes/gateway` does not detect omission of the required `other` listener record when `additional-0` remains at index 1 with zero attached routes.

Target: `evalon/traefik-ga-730e07a9`.

### Which test

The only test on the branch that builds a Gateway with the listeners `current`, `other`, `additional-0`, `additional-1` is `TestListenerIsolation` in `pkg/provider/kubernetes/gateway/listener_isolation_test.go`, case `fallback excludes all more specific listeners` (`moreHostnames` has two entries, each becomes an `additional-%d` listener). It runs for HTTPRoute and GRPCRoute, on HTTP and HTTPS, so there are four four-listener subtests. Its whole Gateway status assertion is positional:

```go
// listener_isolation_test.go:226-231
gatewayStatus := report.gateways[ktypes.NamespacedName{Namespace: "default", Name: "gateway"}]
require.NotEmpty(t, gatewayStatus.Listeners)
assert.Equal(t, int32(1), gatewayStatus.Listeners[0].AttachedRoutes)
if !test.otherGateway {
	assert.Zero(t, gatewayStatus.Listeners[1].AttachedRoutes)
}
```

No listener name and no listener count is asserted. The eval fixtures contain no four-listener status fixture with `other` and `additional-0` either (`grep -n 'additional-' tests/*.go` prints nothing).

### Mutation

The test file and its expectations were not touched. The actual output was changed with `verify/repro/c1_drop_other_listener.patch`, which adds an environment-gated block to `makeGatewayStatus` in `pkg/provider/kubernetes/gateway/kubernetes.go`, right after the listener statuses are collected:

```go
if mode := os.Getenv("VERIFY_C1_DROP_OTHER"); mode == "all" || (mode == "four" && len(listeners) == 4) {
	gatewayStatus.Listeners = slices.DeleteFunc(gatewayStatus.Listeners, func(l gatev1.ListenerStatus) bool { return l.Name == "other" })
}
```

`VERIFY_C1_TRACE=1` prints the listener records that `makeGatewayStatus` returns. With both variables unset the patched code behaves like the original.

### Run

```sh
bash verify/repro/c1_repro.sh
```

The script fetches the target branch into a detached worktree, applies the patch and runs these commands in it:

```sh
RUN='^TestListenerIsolation$/^(HTTPRoute|GRPCRoute)$/^(HTTP|HTTPS)$/^fallback_excludes_all_more_specific_listeners$'
VERIFY_C1_TRACE=1 go test -count=1 -v -run "$RUN" ./pkg/provider/kubernetes/gateway
VERIFY_C1_DROP_OTHER=all VERIFY_C1_TRACE=1 go test -count=1 -v -run "$RUN" ./pkg/provider/kubernetes/gateway
VERIFY_C1_DROP_OTHER=four go test -count=1 ./pkg/provider/kubernetes/gateway/...
VERIFY_C1_DROP_OTHER=all go test -count=1 -run '^TestListenerIsolation$' ./pkg/provider/kubernetes/gateway
```

Output (filtered by the script to result and trace lines, `uniq -c` counts in the first column):

```console
### 1. original output (four-listener subtests, test file untouched)
      1     --- PASS: TestListenerIsolation/GRPCRoute/HTTP/fallback_excludes_all_more_specific_listeners (0.05s)
      1     --- PASS: TestListenerIsolation/GRPCRoute/HTTPS/fallback_excludes_all_more_specific_listeners (0.05s)
      1     --- PASS: TestListenerIsolation/HTTPRoute/HTTP/fallback_excludes_all_more_specific_listeners (0.05s)
      1     --- PASS: TestListenerIsolation/HTTPRoute/HTTPS/fallback_excludes_all_more_specific_listeners (0.05s)
      1 --- PASS: TestListenerIsolation (0.00s)
      1 PASS
      4 VERIFY-C1 gateway=default/gateway specListeners=4 statusListeners=[0]current:attachedRoutes=1 [1]other:attachedRoutes=0 [2]additional-0:attachedRoutes=0 [3]additional-1:attachedRoutes=0
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.071s
### 2. `other` listener record removed from the actual Gateway status (expectations untouched)
      1     --- PASS: TestListenerIsolation/GRPCRoute/HTTP/fallback_excludes_all_more_specific_listeners (0.05s)
      1     --- PASS: TestListenerIsolation/GRPCRoute/HTTPS/fallback_excludes_all_more_specific_listeners (0.05s)
      1     --- PASS: TestListenerIsolation/HTTPRoute/HTTP/fallback_excludes_all_more_specific_listeners (0.05s)
      1     --- PASS: TestListenerIsolation/HTTPRoute/HTTPS/fallback_excludes_all_more_specific_listeners (0.05s)
      1 --- PASS: TestListenerIsolation (0.00s)
      1 PASS
      4 VERIFY-C1 gateway=default/gateway specListeners=4 statusListeners=[0]current:attachedRoutes=1 [1]additional-0:attachedRoutes=0 [2]additional-1:attachedRoutes=0
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.072s
### 3. same removal limited to four-listener Gateways, whole package
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.261s
### 4. control: removal for every Gateway, whole TestListenerIsolation (two-listener cases index Listeners[1])
--- FAIL: TestListenerIsolation (0.06s)
panic: runtime error: index out of range [1] with length 1 [recovered, repanicked]
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.075s
FAIL
```

### What the output shows

- Step 1, original output: index 1 is `other` with 0 attached routes, all four subtests pass.
- Step 2, `other` removed: the status has three records, index 1 is `additional-0` with 0 attached routes, and the same four subtests still pass. No assertion fails.
- Step 3: with the removal applied to every four-listener Gateway, the whole package on the branch still reports `ok`.
- Step 4, control: the mutation is live. Removing `other` from the two-listener cases leaves one record and the test dies with an index-out-of-range panic on `Listeners[1]`, not with an assertion about the missing record.

With the eval fixtures copied into the package, the set of failing tests was identical with and without `VERIFY_C1_DROP_OTHER=four` (121 `FAIL` lines in each run), so the fixtures do not detect the removal either.

### Impact reasoning

The Gateway API requires one status entry per listener. A regression that stops reporting a listener in a multi-listener Gateway, or reports listeners in the wrong order, passes this test as long as some listener with zero attached routes sits at index 1. In the four-listener case every listener after `current` has zero attached routes, so any of them satisfies the assertion. The test therefore gives no protection for the listener set or for which listener a count belongs to. Asserting the listener names in order, or looking the record up by name before checking `AttachedRoutes`, closes the gap.

## C2

Claim: a trailing-dot Host allows an HTTPS request using the catch-all listener's SNI-selected TLS options to reach the exact listener's backend despite the parents having distinct TLS options.

Target: `evalon/traefik-ga-968ccc91`.

### Naming drift

- `buildListenerRouters` does not exist on this branch. The nearest equivalent is `loadMisdirectedRequestDetection` in `pkg/provider/kubernetes/gateway/kubernetes.go`, which emits one `misdirectedRequest` middleware per entry point plus a fallback router.
- The middleware (`pkg/middlewares/gatewayapi/misdirected/misdirected_request.go`) does not select TLS options. It maps the request Host and the SNI to a listener hostname each and answers 421 when the two differ.
- The provider on this branch never sets `router.TLS.Options` and never creates a TLS option. Every Gateway router resolves to the `default` TLS options, also when the two listeners reference different certificate Secrets (scenarios A and B below). "Distinct parent TLS options" can only be produced on this branch by editing the provider output by hand (scenarios C and D).

### Run

```sh
EVAL_TESTS_DIR=/path/to/eval_tests/tests bash verify/repro/c2_repro.sh
```

The script fetches the target branch into a detached worktree, copies the three eval fixtures and the two probes into it and runs:

```sh
go test -tags verify -count=1 -v -run TestVerifyC2Classification ./pkg/middlewares/gatewayapi/misdirected/
go test -tags verify -count=1 -v -run TestVerifyC2Reachability ./pkg/provider/kubernetes/gateway/
```

Both probes only observe and print. They assert nothing about the claim, so `PASS` means only that they ran.

### Part 1 — hostname classification

`verify/repro/c2_classification_probe_test.go` builds the branch's middleware for the listener hostnames `abc.example.com` and `*` (catch-all), wraps it in the request decorator exactly as an entry point does, and feeds the same request to the production Host matcher for `Host(`abc.example.com`)`.

```console
VERIFY-C2 SNI=xyz.example.com  Host="abc.example.com"      canonicalHost="abc.example.com"    middleware{hostListener="abc.example.com" sniListener="*"               status=421 nextCalled=false} HostMatcher(`abc.example.com`)=true
VERIFY-C2 SNI=xyz.example.com  Host="abc.example.com."     canonicalHost="abc.example.com."   middleware{hostListener="*"               sniListener="*"               status=200 nextCalled=true } HostMatcher(`abc.example.com`)=true
VERIFY-C2 SNI=xyz.example.com  Host="ABC.Example.COM.:443" canonicalHost="abc.example.com."   middleware{hostListener="*"               sniListener="*"               status=200 nextCalled=true } HostMatcher(`abc.example.com`)=true
VERIFY-C2 SNI=xyz.example.com  Host="xyz.example.com"      canonicalHost="xyz.example.com"    middleware{hostListener="*"               sniListener="*"               status=200 nextCalled=true } HostMatcher(`abc.example.com`)=false
VERIFY-C2 SNI=xyz.example.com  Host="xyz.example.com."     canonicalHost="xyz.example.com."   middleware{hostListener="*"               sniListener="*"               status=200 nextCalled=true } HostMatcher(`abc.example.com`)=false
VERIFY-C2 SNI=abc.example.com  Host="abc.example.com"      canonicalHost="abc.example.com"    middleware{hostListener="abc.example.com" sniListener="abc.example.com" status=200 nextCalled=true } HostMatcher(`abc.example.com`)=true
VERIFY-C2 SNI=abc.example.com  Host="abc.example.com."     canonicalHost="abc.example.com."   middleware{hostListener="*"               sniListener="abc.example.com" status=421 nextCalled=false} HostMatcher(`abc.example.com`)=true
VERIFY-C2 SNI=abc.example.com  Host="ABC.Example.COM.:443" canonicalHost="abc.example.com."   middleware{hostListener="*"               sniListener="abc.example.com" status=421 nextCalled=false} HostMatcher(`abc.example.com`)=true
VERIFY-C2 SNI=abc.example.com  Host="xyz.example.com"      canonicalHost="xyz.example.com"    middleware{hostListener="*"               sniListener="abc.example.com" status=421 nextCalled=false} HostMatcher(`abc.example.com`)=false
VERIFY-C2 SNI=abc.example.com  Host="xyz.example.com."     canonicalHost="xyz.example.com."   middleware{hostListener="*"               sniListener="abc.example.com" status=421 nextCalled=false} HostMatcher(`abc.example.com`)=false
--- PASS: TestVerifyC2Classification (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/middlewares/gatewayapi/misdirected	0.009s
```

Second line: for SNI `xyz.example.com` and Host `abc.example.com.` the middleware classifies the Host as the catch-all listener (`hostListener="*"`), equal to the SNI's listener, and calls the next handler. The Host matcher for the exact listener returns `true` for the same request. The first line is the control without the trailing dot: the middleware classifies the Host as `abc.example.com` and answers 421.

Why: `requestHost` returns `requestdecorator.GetCanonicalHost` when present. That value comes from `types.CanonicalDomain`, which lowercases and trims spaces but keeps the trailing dot (`canonicalHost="abc.example.com."`). The `TrimSuffix(host, ".")` in `requestHost` only applies to the fallback path that is not taken behind an entry point. The Host matcher in `pkg/muxer/http/matcher.go` retries without the trailing dot.

The 7th line shows the reverse direction: exact SNI with Host `abc.example.com.` is rejected with 421 although SNI and Host name the same listener.

### Part 2 — production request reachability

`verify/repro/c2_reachability_probe_test.go` loads a Gateway with an HTTPS listener `abc.example.com` (route `http-app-precise`, backend `10.10.1.1`, reported as `EXACT-listener-backend`) and a catch-all HTTPS listener (route `http-app-catchall`, backend `10.10.2.1`, reported as `catchall-listener-backend`), runs the provider, aggregates the configuration with `server.NewConfigurationWatcher`, and builds the routers with the production router factory. Each request is sent twice: through the HTTPS handler chain with the TLS options the TCP router selects for the SNI (the fixture helper `evalServeHTTPSFull` is run alongside and agreed on every line), and over a real loopback TLS connection to a `TCPEntryPoint` (TLS 1.2 and TLS 1.3 clients, HTTP/2).

Scenarios:

- A: provider output, both listeners reference the same certificate Secret.
- B: provider output, the listeners reference different certificate Secrets.
- C: provider output, then the exact parent's router is given a TLS option (`exact-listener-tls13`) by hand.
- D: as C, and the catch-all parent's router is given a different TLS option (`catchall-listener-tls12`).

Each scenario ran with `strictTLSOptions=false` and `strictTLSOptions=true`. The lines below are the `strictTLSOptions=false` runs unless noted. The requests shown are catch-all SNI `xyz.example.com` with Host `abc.example.com` (control) and `abc.example.com.` (claim). The full matrix, including `ABC.Example.COM.:443` and empty SNI, is in `verify/logs/c2_repro.log`.

Scenario A, provider output, shared certificate:

```console
VERIFY-C2 ROUTER httproute-default-http-app-catchall… rule=Path("/app") && !Host("abc.example.com") prio=100001 … tls.options="" resolvedOptions="default" conflictingOptions=false
VERIFY-C2 ROUTER httproute-default-http-app-precise… rule=Host("abc.example.com") && Path("/app") prio=100016 … tls.options="" resolvedOptions="default" conflictingOptions=false
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com"      sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com."     sniSelectedTLSOptions="default"                                -> 200 EXACT-listener-backend
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[example.com *.example.com foo.com] -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[example.com *.example.com foo.com] -> 200 EXACT-listener-backend
```

Scenario B, provider output, distinct listener certificates:

```console
VERIFY-C2 ROUTER httproute-default-http-app-catchall… rule=Path("/app") && !Host("abc.example.com") prio=100001 … tls.options="" resolvedOptions="default" conflictingOptions=false
VERIFY-C2 ROUTER httproute-default-http-app-precise… rule=Host("abc.example.com") && Path("/app") prio=100016 … tls.options="" resolvedOptions="default" conflictingOptions=false
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com"      sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com."     sniSelectedTLSOptions="default"                                -> 200 EXACT-listener-backend
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 200 EXACT-listener-backend
```

Scenario C, distinct TLS option injected on the exact parent:

```console
VERIFY-C2 ROUTER httproute-default-http-app-catchall… rule=Path("/app") && !Host("abc.example.com") prio=100001 … tls.options="" resolvedOptions="default" conflictingOptions=false
VERIFY-C2 ROUTER httproute-default-http-app-precise… rule=Host("abc.example.com") && Path("/app") prio=100016 … tls.options="exact-listener-tls13" resolvedOptions="exact-listener-tls13@kubernetesgateway" conflictingOptions=false
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com"      sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com."     sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
```

Scenario D, distinct TLS options injected on both parents, `strictTLSOptions=false`:

```console
VERIFY-C2 ROUTER httproute-default-http-app-precise… rule=Host("abc.example.com") && Path("/app") prio=100016 … tls.options="exact-listener-tls13" resolvedOptions="exact-listener-tls13@kubernetesgateway" conflictingOptions=false
VERIFY-C2 ROUTER websecure-conflicted-httproute-default-http-app-catchall… rule=Path("/app") && !Host("abc.example.com") prio=100001 … tls.options="catchall-listener-tls12" resolvedOptions="default" conflictingOptions=false
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com"      sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com."     sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
```

Scenario D, `strictTLSOptions=true`:

```console
VERIFY-C2 ROUTER httproute-default-http-app-precise… rule=Host("abc.example.com") && Path("/app") prio=100016 … tls.options="exact-listener-tls13" resolvedOptions="exact-listener-tls13@kubernetesgateway" conflictingOptions=false
VERIFY-C2 ROUTER websecure-conflicted-httproute-default-http-app-catchall… rule=Path("/app") && !Host("abc.example.com") prio=100001 … tls.options="catchall-listener-tls12" resolvedOptions="catchall-listener-tls12@kubernetesgateway" conflictingOptions=true
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com"      sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com."     sniSelectedTLSOptions="default"                                -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
```

Probe result lines:

```console
--- PASS: TestVerifyC2Reachability (1.86s)
    --- PASS: TestVerifyC2Reachability/A-provider-output-shared-cert/strictTLSOptions=false (0.32s)
    --- PASS: TestVerifyC2Reachability/A-provider-output-shared-cert/strictTLSOptions=true (0.16s)
    --- PASS: TestVerifyC2Reachability/B-provider-output-distinct-listener-certs/strictTLSOptions=false (0.15s)
    --- PASS: TestVerifyC2Reachability/B-provider-output-distinct-listener-certs/strictTLSOptions=true (0.09s)
    --- PASS: TestVerifyC2Reachability/C-injected-distinct-tls-options-exact-only/strictTLSOptions=false (0.68s)
    --- PASS: TestVerifyC2Reachability/C-injected-distinct-tls-options-exact-only/strictTLSOptions=true (0.22s)
    --- PASS: TestVerifyC2Reachability/D-injected-distinct-tls-options-both/strictTLSOptions=false (0.17s)
    --- PASS: TestVerifyC2Reachability/D-injected-distinct-tls-options-both/strictTLSOptions=true (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	1.887s
```

### What the output shows

- A and B, the only states the branch's provider produces: the request with catch-all SNI and Host `abc.example.com.` gets `200` from the exact listener's backend, in the handler chain and over real TLS. In B the client was served the catch-all listener's certificate (`catchall-listener.test`) and still reached the exact listener's backend. The same request without the trailing dot gets 421.
- C and D, distinct TLS options forced onto the parents: the same request gets 421 and does not reach the exact backend. The middleware and `SNICheck` (`pkg/middlewares/snicheck/snicheck.go`) both answer 421 with the body `Misdirected Request`, so the output does not name the rejecting handler. Inference, not observation: the middleware classification is the same as in part 1, where it lets this request through, and `SNICheck` is the handler in the chain that compares the exact router's TLS options with the options selected for the SNI.
- So the request reaches the exact backend exactly when both parents resolve to the same TLS options, which on this branch is always.

### Contrast on the audited branch

The same reachability probe, scenarios A and B, on `traefik-gateway-listener-isolation-perfect` (fixtures and probe copied into `pkg/provider/kubernetes/gateway/`, then removed):

```sh
go test -tags verify -count=1 -v -run 'TestVerifyC2Reachability/(A|B)-' ./pkg/provider/kubernetes/gateway/
```

```console
      4 VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com"      sniSelectedTLSOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" -> 421 -("Misdirected Request")
      4 VERIFY-C2 REQ mode=handler-chain SNI="xyz.example.com" Host="abc.example.com."     sniSelectedTLSOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" -> 421 -("Misdirected Request")
      2 VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
      2 VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com"      negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[example.com *.example.com foo.com] -> 421 -("Misdirected Request")
      2 VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[catchall-listener.test *.example.com] -> 421 -("Misdirected Request")
      2 VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=TLS 1.3 SNI="xyz.example.com" Host="abc.example.com."     negotiated=TLS 1.3/HTTP/2.0 servedCertDNSNames=[example.com *.example.com foo.com] -> 421 -("Misdirected Request")
```

There the provider creates one TLS option per listener and the trailing-dot request is rejected with 421 in every run.

### Impact reasoning

On `evalon/traefik-ga-968ccc91` the 421 check that separates HTTPS listeners is the only isolation between two listeners that share an entry point, because all Gateway routers share the `default` TLS options and `SNICheck` never fires. A client that connects with any SNI that belongs to the catch-all listener, and sends `Host: abc.example.com.`, is routed to the exact listener's routes and backend. It is served the catch-all listener's certificate and never has to satisfy the exact listener's TLS configuration. The trigger needs no special configuration: one exact and one catch-all (or wildcard) HTTPS listener on the same port, and a Host header with a trailing dot, which is a valid fully qualified name that any HTTP client can send. The reverse was observed at the middleware level only (part 1, 7th line): exact SNI with a trailing-dot Host is answered 421. The probe found no provider-produced state in which the trailing-dot request is rejected, and no workaround was tested.

The fix is to normalise the Host in `requestHost` the same way the Host matcher does (strip one trailing dot from the canonical host as well), and to add an HTTPS regression case with a trailing-dot Host for both directions.
