## Setup (shared by every section)

Branches and commits used:

| Role | Ref | Commit |
|---|---|---|
| Audited branch, used as positive control | `origin/traefik-gateway-listener-isolation-perfect` | `16bd9c70425e946d64be29de626ab91d7f2a1edd` |
| C1 and C2 target | `evalon/traefik-ga-fadc98fd` in `kaitranntt-evals/traefik-gateway-listener-isolation` | `d913ede2a03e9da9eefd1555a63aa355b0acaa32` |
| C3 target | `evalon/traefik-ga-9fb05ad2` in `kaitranntt-evals/traefik-gateway-listener-isolation` | `b3beecb178fd99b6aff950bd01e5d95fb6805e2f` |
| Eval base, used as second control for C3 | `35c7530ac0906b5407a3733de173d58d9c427949` | same |

```sh
cd ~/repos/traefik
git fetch origin traefik-gateway-listener-isolation-perfect
git checkout -b verify/traefik-gateway-listener-isolation-v-df0841bb origin/traefik-gateway-listener-isolation-perfect
git remote add evalrepo https://git-manager.itsdev.in/proxy/github.com/kaitranntt-evals/traefik-gateway-listener-isolation
git fetch evalrepo evalon/traefik-ga-fadc98fd evalon/traefik-ga-9fb05ad2
git worktree add --detach ../wt-fadc evalrepo/evalon/traefik-ga-fadc98fd
git worktree add --detach ../wt-9fb0 evalrepo/evalon/traefik-ga-9fb05ad2
git worktree add --detach ../wt-base 35c7530ac0906b5407a3733de173d58d9c427949
mkdir -p ~/eval_tests && unzip -o -q eval_tests.zip -d ~/eval_tests   # byte-exact eval fixtures -> ~/eval_tests/tests
```

The probes are Go test files with a `//go:build verify` tag kept under `verify/repro/`. `verify/repro/run.sh <worktree> <fixtures dir> [-run regexp]` copies the three eval fixtures (`eval_isolation_test.go`, `eval_https_test.go`, `export_eval_test.go`) and the probes into `pkg/provider/kubernetes/gateway/` of the worktree and runs `go test -tags verify -count=1 -v`. No production file was modified in any worktree. Full outputs are committed under `verify/logs/`.

The probes reuse the eval fixtures' own helpers: the loader (`evalIsoLoad` / `EvalIsoLoadExported`), the Gateway/route renderers, `evalOneShotProvider` and `evalTagProxy` (the backend stub answers `200` with the backend IP, here `10.10.0.1`).

`verify/repro/verify_c1c2_tls_test.go` builds the pipeline exactly like the fixture `evalServeHTTPSFull` (`server.NewConfigurationWatcher` -> `tls.Manager.UpdateConfigs` -> `server.NewRouterFactory(...).CreateRouters`), then, instead of calling the handlers in-process, serves the TCP router on a real loopback entry point (`server.NewTCPEntryPoints` + `Switch` + `Start`) and makes real TLS handshakes and HTTPS requests against it.

Fixture status on the two claim branches, for context only (no verdict rests on it):

```sh
cd ~/repos/wt-fadc && go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway -v 2>&1 | grep -E '^(--- |ok|FAIL|PASS)'
cd ~/repos/wt-9fb0 && go test -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/gateway -v 2>&1 | grep -E '^(--- |ok|FAIL|PASS)'
```

```console
# evalon/traefik-ga-fadc98fd: 16 PASS, 4 FAIL
--- FAIL: TestEval_GatewayHTTPListenerIsolation_TrailingDotWildcard (0.00s)
--- FAIL: TestEval_GatewayHTTPListenerIsolation_TrailingDotWildcardFallthrough (0.00s)
--- FAIL: TestEval_GatewayHTTPSListenerDetectMisdirectedRequests (0.97s)
--- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability (1.70s)
# evalon/traefik-ga-9fb05ad2: 15 PASS, 5 FAIL
--- FAIL: TestEval_GatewayHTTPListenerIsolation_SingleGatewaySpecificity (0.06s)
--- FAIL: TestEval_GatewayHTTPListenerIsolation_ProtocolSeparation (0.00s)
--- FAIL: TestEval_GatewayHTTPSListenerDetectMisdirectedRequests (1.02s)
--- FAIL: TestEval_GatewayHTTPSSiblingGatewayReachability (1.71s)
--- FAIL: TestEval_GatewayHTTPSListenerIsolation_RouteKindRestrictedListeners (0.05s)
--- PASS: TestEval_GatewayRouteAcceptanceIndependentOfIsolation (0.01s)
```

## C1

Claim: the docs promise dedicated TLS options instead of `default` for wildcard Gateway HTTPS listeners, but those listeners use `default` in ordinary configuration, with no documented fallback. Target: `evalon/traefik-ga-fadc98fd`. Verdict: **CONFIRMED** (both parts hold).

### Part 1 - Documentation: holds

```sh
cd ~/repos/traefik && sh verify/repro/c1_docs_check.sh   # reads the docs at evalrepo/evalon/traefik-ga-fadc98fd through git show / git grep
```

```console
## docs files changed by the branch
 .../routing-configuration/kubernetes/gateway-api.md         | 13 +++++++++++++
 1 file changed, 13 insertions(+)
## the promise: Listener Isolation section of docs/content/reference/routing-configuration/kubernetes/gateway-api.md
### Listener Isolation

When several listeners of a `Gateway` share the same port and protocol, a request is served only by the routes attached to the listener with the most specific hostname matching it.
For example, with listeners for `foo.example.com` and `*.example.com`, a request to `foo.example.com` is never served by the routes attached to the `*.example.com` listener,
and a request for a hostname matching a listener without matching any of its routes is answered with a `404`.

For HTTPS listeners, Traefik also detects misdirected requests, i.e. requests whose SNI and `Host` header are served by different listeners, and answers them with a `421 Misdirected Request`.

!!! info "TLS options"
    To detect misdirected requests, Traefik associates dedicated, empty [TLS options](../http/tls/tls-options.md) with each HTTPS listener that has a hostname
    and shares its port with other listeners of the `Gateway`.
    Those TLS options replace the `default` TLS options for the connections that are matched on the listener hostname.

## qualifications inside docs/content/reference/routing-configuration/kubernetes/gateway-api.md (wildcard / HostRegexp / fallback / default TLS options / strict)
154:    Those TLS options replace the `default` TLS options for the connections that are matched on the listener hostname.
## every docs/ mention of Gateway listeners together with TLS options
evalrepo/evalon/traefik-ga-fadc98fd:docs/content/reference/routing-configuration/kubernetes/gateway-api.md:152:    To detect misdirected requests, Traefik associates dedicated, empty [TLS options](../http/tls/tls-options.md) with each HTTPS listener that has a hostname
evalrepo/evalon/traefik-ga-fadc98fd:docs/content/reference/routing-configuration/kubernetes/gateway-api.md:154:    Those TLS options replace the `default` TLS options for the connections that are matched on the listener hostname.
## generic (non-Gateway) statements about HostRegexp and TLS options under docs/
evalrepo/evalon/traefik-ga-fadc98fd:docs/content/migrate/v3.md:626:Previously, TLSOptions selection was limited to exact `Host` matches, and using `HostRegexp` or wildcards would fall back to the default TLS options with a warning message like: `No domain found in rule HostRegexp(...) the TLS option foo cannot be applied`.
evalrepo/evalon/traefik-ga-fadc98fd:docs/content/migrate/v3.md:628:Note: TLSOptions for `HostRegexp` matchers remains unsupported. Use wildcard `Host` matchers as an alternative.
evalrepo/evalon/traefik-ga-fadc98fd:docs/content/reference/routing-configuration/http/routing/rules-and-priority.md:58:    It should be preferred over the `HostRegexp` matcher as it allows attaching a TLS option and is more efficient.
```

Observations:

- The only docs file the branch touches is `gateway-api.md`; `docs/content/migrate/v3.md` is unchanged.
- The promise is scoped to "each HTTPS listener that has a hostname and shares its port with other listeners of the `Gateway`". A wildcard listener (`*.example.com`) has a hostname, so it is included; nothing excludes it.
- The section carries no qualification: no mention of wildcard listeners, `HostRegexp`, a fallback to `default`, or `strictTLSOptions`.
- The only text under `docs/` relating `HostRegexp` to TLS options is generic router-rule text that predates the branch (`migrate/v3.md:626-628`, `rules-and-priority.md:58`): "TLSOptions for `HostRegexp` matchers remains unsupported". It never mentions Gateway listeners, and nothing under `docs/` says that a wildcard listener is compiled to a `HostRegexp` rule, so a Gateway user cannot derive the fallback from it. I do not count it as documenting the applicable fallback; a reader who does would see this part as weaker, but the promise in `gateway-api.md` stays unqualified either way.

### Part 2 - Configuration behavior: holds

```sh
cd ~/repos/traefik && sh verify/repro/run.sh ~/repos/wt-fadc ~/eval_tests/tests '^TestVerifyC1C2_' > verify/logs/c1c2_fadc98fd.log
cd ~/repos/traefik && sh verify/repro/run.sh ~/repos/traefik ~/eval_tests/tests '^TestVerifyC1C2_' > verify/logs/c1c2_perfect_control.log   # control
grep -n 'with no domains in its rule' ~/repos/wt-fadc/pkg/server/aggregator.go
```

Scenario "wildcard+exact": one Gateway, HTTPS port 443, listeners `wildcard=*.example.com` and `precise=abc.example.com`; HTTPRoutes `http-app-wildcard` (no hostnames, `/wildcard`) and `http-app-bar` (hostname `bar.example.com`, `/bar`) on the wildcard listener, `http-app-precise` (`/precise`) on the precise listener. Default install configuration (`strictTLSOptions=false`).

Output on `evalon/traefik-ga-fadc98fd` (router names shortened by the probe):

```console
C1C2 wildcard+exact strict=false |   aggregated router httproute-default-ht…45e004c@kubernetesgateway rule=Host("bar.example.com") && Path("/bar") configuredOptions="listener-example-com-47287a8f16ec75e6073f" resolvedOptions="listener-example-com-47287a8f16ec75e6073f@kubernetesgateway"
C1C2 wildcard+exact strict=false |   aggregated router httproute-default-ht…41269fe@kubernetesgateway rule=Host("abc.example.com") && Path("/precise") configuredOptions="listener-abc-example-com-5dc2b6bb543787c97533" resolvedOptions="listener-abc-example-com-5dc2b6bb543787c97533@kubernetesgateway"
C1C2 wildcard+exact strict=false |   aggregated router listener-default-pre…c584434@kubernetesgateway rule=Host("abc.example.com") configuredOptions="listener-abc-example-com-5dc2b6bb543787c97533" resolvedOptions="listener-abc-example-com-5dc2b6bb543787c97533@kubernetesgateway"
C1C2 wildcard+exact strict=false |   aggregated router websecure-conflicted…ff44f97@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && Path("/wildcard") && !Host("abc.example.com") configuredOptions="listener-example-com-47287a8f16ec75e6073f" resolvedOptions="default"
C1C2 wildcard+exact strict=false |   aggregated router websecure-conflicted…3f4e92b@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !Host("abc.example.com") configuredOptions="listener-example-com-47287a8f16ec75e6073f" resolvedOptions="default"
C1C2 wildcard+exact strict=false |   aggregator log {"level":"error","message":"On EntryPoint \"websecure\", router \"httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-d82801289c9a6ff44f97@kubernetesgateway\", with no domains in its rule, is configured with a TLS options different than the default one, default TLSOptions will be applied for it"}
C1C2 wildcard+exact strict=false |   aggregator log {"level":"error","message":"On EntryPoint \"websecure\", router \"listener-default-wildcard-gw-default-my-gateway-ep-websecure-0-8de922e8e29ca3f4e92b@kubernetesgateway\", with no domains in its rule, is configured with a TLS options different than the default one, default TLSOptions will be applied for it"}
C1C2 wildcard+exact strict=false |   SNI abc.example.com        -> TCP router selects TLS options "listener-abc-example-com-5dc2b6bb543787c97533@kubernetesgateway" NextProtos=[] | real handshake offering [h2 http/1.1] negotiated ""
C1C2 wildcard+exact strict=false |   SNI xyz.example.com        -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 wildcard+exact strict=false |   SNI bar.example.com        -> TCP router selects TLS options "listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" NextProtos=[] | real handshake offering [h2 http/1.1] negotiated ""
C1C2 wildcard+exact strict=false |   SNI other.test             -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
```

```console
$ grep -n 'with no domains in its rule' ~/repos/wt-fadc/pkg/server/aggregator.go
284:				log.Error().Msgf("On EntryPoint %q, router %q, with no domains in its rule, is configured with a TLS options different than the default one, t
288:			log.Error().Msgf("On EntryPoint %q, router %q, with no domains in its rule, is configured with a TLS options different than the default one, de
```

Reading:

- The provider (`isolateListeners`) assigns `listener-example-com-47287a8f16ec75e6073f` to every router of the wildcard listener (`configuredOptions`).
- After the ordinary aggregation, the wildcard listener's own router and its hostname-less route are emitted as `websecure-conflicted-…` copies with `resolvedOptions="default"`. The two error lines come from `findConflictingRouters` (`pkg/server/aggregator.go:284/288`), which flags routers whose rule has no `Host` domain; `resolveHTTPTLSOptions` then writes `default` on the copy. Naming nuance: the claim attributes the replacement to `findConflictingRouters` alone.
- A connection with SNI `xyz.example.com`, which matches the wildcard listener hostname, is served with `default`, contradicting "Those TLS options replace the `default` TLS options for the connections that are matched on the listener hostname".
- Not every router of the wildcard listener falls back: the route with an exact hostname (`Host("bar.example.com")`) keeps the dedicated options, and SNI `bar.example.com` selects them.

Control on `traefik-gateway-listener-isolation-perfect`, same scenario: the wildcard listener is emitted as `Host("**.example.com")`, keeps its dedicated options, and no error is logged.

```console
C1C2 wildcard+exact strict=false |   aggregated router listener-websecure-h…7b479e2@kubernetesgateway rule=Host("abc.example.com") configuredOptions="listener-websecure-https-abc-example-com-84b9126a78b887b479e2" resolvedOptions="listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway"
C1C2 wildcard+exact strict=false |   aggregated router listener-websecure-h…a2e7cbb@kubernetesgateway rule=(Host("**.example.com")) && !(Host("abc.example.com")) configuredOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolvedOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"
C1C2 wildcard+exact strict=false |   SNI xyz.example.com        -> TCP router selects TLS options "listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
```

### Impact observed

1. Two wildcard listeners on one port (`wildcard=*.example.com`, `nested=*.foo.example.com`): all four routers fall back to `default`, both SNIs select `default`, and a misdirected request is served instead of answered with `421`.

```console
C1C2 two-wildcards strict=false |   aggregated router websecure-conflicted…b448fa8@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") && Path("/nested") configuredOptions="listener-foo-example-com-06a3e25dcad9ba39c7da" resolvedOptions="default"
C1C2 two-wildcards strict=false |   aggregated router websecure-conflicted…9d7c85a@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && Path("/wildcard") && !HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") configuredOptions="listener-example-com-47287a8f16ec75e6073f" resolvedOptions="default"
C1C2 two-wildcards strict=false |   aggregated router websecure-conflicted…415f8c6@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") configuredOptions="listener-foo-example-com-06a3e25dcad9ba39c7da" resolvedOptions="default"
C1C2 two-wildcards strict=false |   aggregated router websecure-conflicted…5f2f808@kubernetesgateway rule=HostRegexp("^[a-z0-9-\\.]+\\.example\\.com$") && !HostRegexp("^[a-z0-9-\\.]+\\.foo\\.example\\.com$") configuredOptions="listener-example-com-47287a8f16ec75e6073f" resolvedOptions="default"
C1C2 two-wildcards strict=false |   SNI xyz.example.com        -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 two-wildcards strict=false |   SNI a.foo.example.com      -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=xyz.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=a.foo.example.com Host=a.foo.example.com /nested -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=a.foo.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=xyz.example.com Host=a.foo.example.com /nested -> proto=HTTP/2.0 status=200 body="10.10.0.1"
```

Control (`traefik-gateway-listener-isolation-perfect`), same scenario:

```console
C1C2 two-wildcards strict=false |   SNI xyz.example.com        -> TCP router selects TLS options "listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 two-wildcards strict=false |   SNI a.foo.example.com      -> TCP router selects TLS options "listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=xyz.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=a.foo.example.com Host=a.foo.example.com /nested -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=a.foo.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=421 body="Misdirected Request"
C1C2 two-wildcards strict=false |   real HTTPS request SNI=xyz.example.com Host=a.foo.example.com /nested -> proto=HTTP/2.0 status=421 body="Misdirected Request"
```

2. With `core.strictTLSOptions=true` (the setting `migrate/v3.md` recommends), there is no fallback: the wildcard listener's routers are disabled, so a legitimate request to the wildcard listener gets `404`.

```console
C1C2 wildcard+exact strict=true |   aggregator log {"level":"error","message":"On EntryPoint \"websecure\", router \"httproute-default-http-app-wildcard-gw-default-my-gateway-ep-websecure-0-d82801289c9a6ff44f97@kubernetesgateway\", with no domains in its rule, is configured with a TLS options different than the default one, the fallback to the default TLS options being disabled, it will be disabled"}
C1C2 wildcard+exact strict=true |   aggregator log {"level":"error","message":"On EntryPoint \"websecure\", router \"listener-default-wildcard-gw-default-my-gateway-ep-websecure-0-8de922e8e29ca3f4e92b@kubernetesgateway\", with no domains in its rule, is configured with a TLS options different than the default one, the fallback to the default TLS options being disabled, it will be disabled"}
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/1.1 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=xyz.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=404 body="404 page not found"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=bar.example.com Host=bar.example.com /bar -> proto=HTTP/1.1 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=abc.example.com Host=xyz.example.com /wildcard -> proto=HTTP/1.1 status=404 body="404 page not found"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=xyz.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=421 body="Misdirected Request"
```

Control (`traefik-gateway-listener-isolation-perfect`), `strictTLSOptions=true`:

```console
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=xyz.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=bar.example.com Host=bar.example.com /bar -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=abc.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=421 body="Misdirected Request"
C1C2 wildcard+exact strict=true |   real HTTPS request SNI=xyz.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=421 body="Misdirected Request"
```

3. Every configuration build with a wildcard isolated listener logs one error-level line per affected router (shown above).

Impact reasoning: wildcard plus exact listeners on one HTTPS port is the canonical listener-isolation layout (the branch's own doc example uses `foo.example.com` and `*.example.com`). With default settings the user-visible effect is limited to error logs and a doc that is wrong for wildcard listeners, because exact-vs-wildcard misdirection is still detected (`421` in both crossed directions above, since `default` differs from the exact listener's options). The detection is lost when the crossed listeners are both wildcards, and traffic to the wildcard listener is dropped when `strictTLSOptions` is on. Workaround observed: routes with exact hostnames on the wildcard listener keep the dedicated options.

## C2

Claim: isolated Gateway HTTPS listeners fail to negotiate h2 because their active TLS configurations have empty ALPN protocol lists. Target: `evalon/traefik-ga-fadc98fd`. Verdict: **CONFIRMED** (both parts hold; one scope nuance below).

```sh
cd ~/repos/traefik && sh verify/repro/run.sh ~/repos/wt-fadc ~/eval_tests/tests '^TestVerifyC1C2_' > verify/logs/c1c2_fadc98fd.log
cd ~/repos/traefik && sh verify/repro/run.sh ~/repos/traefik ~/eval_tests/tests '^TestVerifyC1C2_' > verify/logs/c1c2_perfect_control.log   # control
```

Scenario "wildcard+exact": one Gateway, HTTPS port 443, listeners `wildcard=*.example.com` and `precise=abc.example.com`; HTTPRoutes `http-app-wildcard` (no hostnames, `/wildcard`) and `http-app-bar` (hostname `bar.example.com`, `/bar`) on the wildcard listener, `http-app-precise` (`/precise`) on the precise listener.

### Part 1 - TLS configuration construction: holds

`provider tls.Options[...]` is the raw output of `isolateListeners`; `tlsManager.Get` is `pkg/tls/tlsmanager.go:Get`, which calls `buildTLSConfig`.

```console
C1C2 wildcard+exact strict=false |   provider tls.Options["listener-abc-example-com-5dc2b6bb543787c97533"].ALPNProtocols=[] (len=0)
C1C2 wildcard+exact strict=false |   provider tls.Options["listener-example-com-47287a8f16ec75e6073f"].ALPNProtocols=[] (len=0)
C1C2 wildcard+exact strict=false |   aggregated tls.Options["default"].ALPNProtocols=[h2 http/1.1 acme-tls/1] -> tlsManager.Get NextProtos=[h2 http/1.1 acme-tls/1] (len=3)
C1C2 wildcard+exact strict=false |   aggregated tls.Options["listener-abc-example-com-5dc2b6bb543787c97533@kubernetesgateway"].ALPNProtocols=[] -> tlsManager.Get NextProtos=[] (len=0)
C1C2 wildcard+exact strict=false |   aggregated tls.Options["listener-example-com-47287a8f16ec75e6073f@kubernetesgateway"].ALPNProtocols=[] -> tlsManager.Get NextProtos=[] (len=0)
```

The two listener options are published with `ALPNProtocols=[]` and reach `tls.Config.NextProtos` empty; only `default` carries `h2`. Nothing restores `h2` between the provider and the TLS manager.

### Part 2 - Production handshake: holds

`TCP router selects` is the result of `Router.HTTP3TLSConfigMatcherFunc()`, the same selector the eval fixture `evalServeHTTPSFull` uses. The handshake is a real `crypto/tls` client handshake against the running entry point, offering `[h2 http/1.1]`. The requests use Go's standard HTTPS client with HTTP/2 enabled, and an HTTP/2-only client (`golang.org/x/net/http2.Transport`, the transport a gRPC client uses).

```console
C1C2 wildcard+exact strict=false |   SNI abc.example.com        -> TCP router selects TLS options "listener-abc-example-com-5dc2b6bb543787c97533@kubernetesgateway" NextProtos=[] | real handshake offering [h2 http/1.1] negotiated ""
C1C2 wildcard+exact strict=false |   SNI xyz.example.com        -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 wildcard+exact strict=false |   SNI bar.example.com        -> TCP router selects TLS options "listener-example-com-47287a8f16ec75e6073f@kubernetesgateway" NextProtos=[] | real handshake offering [h2 http/1.1] negotiated ""
C1C2 wildcard+exact strict=false |   SNI other.test             -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 wildcard+exact strict=false |   real HTTPS request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/1.1 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=false |   real HTTP/2-only (gRPC-like) request SNI=abc.example.com Host=abc.example.com /precise -> error: Get "https://abc.example.com/precise": http2: failed reading the frame payload: http2: frame too large, note that the frame header looked like an HTTP/1.1 header
C1C2 wildcard+exact strict=false |   real HTTPS request SNI=xyz.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=false |   real HTTP/2-only (gRPC-like) request SNI=xyz.example.com Host=xyz.example.com /wildcard -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=false |   real HTTPS request SNI=bar.example.com Host=bar.example.com /bar -> proto=HTTP/1.1 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=false |   real HTTP/2-only (gRPC-like) request SNI=bar.example.com Host=bar.example.com /bar -> error: Get "https://bar.example.com/bar": http2: failed reading the frame payload: http2: frame too large, note that the frame header looked like an HTTP/1.1 header
```

- SNI `abc.example.com` (the isolated exact listener) and SNI `bar.example.com` (an exact-hostname route on the isolated wildcard listener) select the empty-ALPN configuration, negotiate no protocol (`""`), are served over `HTTP/1.1`, and the HTTP/2-only client fails.
- Scope nuance: SNI `xyz.example.com` on the same isolated wildcard listener does negotiate `h2`, only because that listener's `HostRegexp` routers fall back to the `default` options during aggregation (the C1 behavior). So "isolated listeners" is true for exact-hostname listeners and exact-hostname routes, not for wildcard-only traffic.

Controls:

```console
# evalon/traefik-ga-fadc98fd, Gateway with a single HTTPS listener (no isolation, default options)
C1C2 single-listener control |   SNI abc.example.com        -> TCP router selects TLS options "default" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 single-listener control |   real HTTPS request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 single-listener control |   real HTTP/2-only (gRPC-like) request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=200 body="10.10.0.1"
# traefik-gateway-listener-isolation-perfect, same wildcard+exact scenario
C1C2 wildcard+exact strict=false |   provider tls.Options["listener-websecure-https-abc-example-com-84b9126a78b887b479e2"].ALPNProtocols=[h2 http/1.1 acme-tls/1] (len=3)
C1C2 wildcard+exact strict=false |   provider tls.Options["listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb"].ALPNProtocols=[h2 http/1.1 acme-tls/1] (len=3)
C1C2 wildcard+exact strict=false |   SNI abc.example.com        -> TCP router selects TLS options "listener-websecure-https-abc-example-com-84b9126a78b887b479e2@kubernetesgateway" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 wildcard+exact strict=false |   SNI bar.example.com        -> TCP router selects TLS options "listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" NextProtos=[h2 http/1.1 acme-tls/1] | real handshake offering [h2 http/1.1] negotiated "h2"
C1C2 wildcard+exact strict=false |   real HTTPS request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=200 body="10.10.0.1"
C1C2 wildcard+exact strict=false |   real HTTP/2-only (gRPC-like) request SNI=abc.example.com Host=abc.example.com /precise -> proto=HTTP/2.0 status=200 body="10.10.0.1"
```

The audited branch calls `listenerTLSOptions.SetDefaults()` (`pkg/provider/kubernetes/gateway/kubernetes.go:741-742`), which is what fills `ALPNProtocols`; the claim branch publishes `tls.Options{}`.

Fixture note: the eval HTTPS fixtures never perform a handshake (`evalServeHTTPSFull` calls the HTTPS handler in-process with a synthetic `tls.ConnectionState`), so they cannot observe this.

Impact reasoning: any Gateway with two or more HTTPS listeners on a port where one has an exact hostname is affected, which is the ordinary listener-isolation layout. Plain HTTPS keeps answering `200`, so the downgrade to HTTP/1.1 is silent; clients that require HTTP/2 over TLS fail outright (observed with the HTTP/2-only transport: `http2: frame too large, note that the frame header looked like an HTTP/1.1 header`), which covers GRPCRoutes attached to such a listener. No workaround at the Gateway API level was observed: the options are generated by the provider; a Gateway with a single listener on the port is unaffected.

## C3

Claim: HTTPRoute and GRPCRoute with hostname `outside.test` and a nonexistent Service backend keep `ResolvedRefs=True` when the parent Gateway only has `*.example.com` and `abc.example.com` listeners, because empty hostname intersections bypass backend-reference validation. Target: `evalon/traefik-ga-9fb05ad2`. Verdict: **CONFIRMED** (same result for both route kinds).

```sh
cd ~/repos/traefik && sh verify/repro/run.sh ~/repos/wt-9fb0 ~/eval_tests/tests '^TestVerifyC3_' > verify/logs/c3_9fb05ad2.log
cd ~/repos/traefik && sh verify/repro/run.sh ~/repos/traefik ~/eval_tests/tests '^TestVerifyC3_' > verify/logs/c3_perfect_control.log   # control 1
cp verify/repro/verify_c3_resolvedrefs_test.go ~/eval_tests/tests/eval_isolation_test.go ../wt-base/pkg/provider/kubernetes/gateway/
(cd ../wt-base && go test -tags verify -count=1 -v -run '^TestVerifyC3_' ./pkg/provider/kubernetes/gateway) > verify/logs/c3_base_control.log   # control 2
```

`verify/repro/verify_c3_resolvedrefs_test.go` mirrors the fixture loader `evalIsoLoadWithStatus` (HTTP Gateway `my-gateway`, port 80, listeners `wildcard=*.example.com` and `precise=abc.example.com`, rendered by the fixture's `evalIsoGateway`), runs `Provider.loadConfigurationFromGateways` with a debug logger in the context, publishes the report with `statusReport.Flush` (what the provider loop does), and reads the route status back from the fake Gateway API clientset. The route references Service `missing-svc`, which does not exist; no `sectionName`.

Output on `evalon/traefik-ga-9fb05ad2`, the claim's case:

```console
C3 | http | CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: Accepted=False reason=NoMatchingListenerHostname message=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: ResolvedRefs=True reason=ResolvedRefs message=""
C3 |   log {"level":"debug","http_route":"http-probe","namespace":"default","message":"Skipping HTTPRoute, its hostnames are owned by more specific listeners"}
C3 |   log {"level":"debug","http_route":"http-probe","namespace":"default","message":"Skipping HTTPRoute, its hostnames are owned by more specific listeners"}
C3 | grpc | CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: Accepted=False reason=NoMatchingListenerHostname message=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: ResolvedRefs=True reason=ResolvedRefs message=""
C3 |   log {"level":"debug","grpc_route":"grpc-probe","namespace":"default","message":"Skipping GRPCRoute, its hostnames are owned by more specific listeners"}
C3 |   log {"level":"debug","grpc_route":"grpc-probe","namespace":"default","message":"Skipping GRPCRoute, its hostnames are owned by more specific listeners"}
```

Same case on the controls:

```console
# traefik-gateway-listener-isolation-perfect
C3 | http | CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: Accepted=False reason=NoMatchingListenerHostname message=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load HTTPBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
C3 | grpc | CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: Accepted=False reason=NoMatchingListenerHostname message=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load GRPCBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
# eval base 35c7530ac0906b5407a3733de173d58d9c427949
C3 | http | CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: Accepted=False reason=NoMatchingListenerHostname message=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load HTTPBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
C3 | grpc | CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: Accepted=False reason=NoMatchingListenerHostname message=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load GRPCBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
```

Controls on `evalon/traefik-ga-9fb05ad2` showing validation works when the intersection is not empty, and that the bypass does not depend on isolation:

```console
C3 | http | CONTROL bar.example.com (intersects wildcard listener) + missing Service | hostnames=[bar.example.com] backend=missing-svc section=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load HTTPBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
C3 | grpc | CONTROL bar.example.com (intersects wildcard listener) + missing Service | hostnames=[bar.example.com] backend=missing-svc section=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load GRPCBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
C3 | http | CONTROL no route hostnames + missing Service | hostnames=[] backend=missing-svc section=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load HTTPBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
C3 | grpc | CONTROL no route hostnames + missing Service | hostnames=[] backend=missing-svc section=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: ResolvedRefs=False reason=BackendNotFound message="Cannot load GRPCBackendRef default/missing-svc: getting service: service \"missing-svc\" not found"
C3 | http | EXTRA outside.test + missing Service, single listener *.example.com (no isolation involved) | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published HTTPRoute/http-probe parent=my-gateway: ResolvedRefs=True reason=ResolvedRefs message=""
C3 | grpc | EXTRA outside.test + missing Service, single listener *.example.com (no isolation involved) | hostnames=[outside.test] backend=missing-svc section=""
C3 |   published GRPCRoute/grpc-probe parent=my-gateway: ResolvedRefs=True reason=ResolvedRefs message=""
```

Reading:

- For both kinds the loader logs `Skipping HTTPRoute/GRPCRoute, its hostnames are owned by more specific listeners` once per listener and never logs the backend error; no router is produced. That log comes from the early return the branch added at the top of `loadHTTPRoute` / `loadGRPCRoute` (`len(route.Spec.Hostnames) > 0 && len(hostnames) == 0`), which returns the initial `ResolvedRefs=True` condition before the backend loop.
- The published status keeps `ResolvedRefs=True reason=ResolvedRefs` next to `Accepted=False reason=NoMatchingListenerHostname`.
- The eval base and the audited branch both publish `ResolvedRefs=False reason=BackendNotFound` for the same objects, so this is a regression introduced by the branch.
- The bypass is keyed on the empty hostname intersection, not on isolation: it also happens with a single `*.example.com` listener (`EXTRA` rows). The log message is therefore also misleading (`outside.test` is not owned by any listener).
- No difference between HTTPRoute and GRPCRoute.

Fixture note: `TestEval_GatewayRouteAcceptanceIndependentOfIsolation` passes on this branch; the eval fixtures contain no `ResolvedRefs` assertion (`grep -n 'ResolvedRefs\|BackendNotFound' ~/eval_tests/tests/*.go` prints nothing), so they do not catch this.

Impact reasoning: a route whose hostnames match none of its parent's listeners and whose backend Service does not exist is reported with `ResolvedRefs=True`, so `kubectl describe` shows only the hostname problem; the same route with a hostname that does match a listener is reported `ResolvedRefs=False reason=BackendNotFound` (the `bar.example.com` control), so the status flips on a backend it had declared resolved once the hostname is corrected. The route is not attached in that state (`Accepted=False`), so no traffic is misrouted; the defect is confined to the status, on a misconfigured route.
