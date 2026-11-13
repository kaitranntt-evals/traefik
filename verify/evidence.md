# Evidence — audit v-e030e582

Observations only. Solution branch `traefik-gateway-listener-isolation-perfect` at `58b8f07dcc99b3b0f01e1ab4fcddf14a81745df2` (base `35c7530ac0906b5407a3733de173d58d9c427949`), checked out in `~/repos/traefik`; production code untouched.

## Setup and method

- Eval fixtures: `eval_tests.zip` extracted byte-exact to `~/fx/tests` and copied into `pkg/provider/kubernetes/gateway/` by `verify/repro/run.sh` (untracked, not committed).
- Probes: `verify/repro/verify_*_test.go` (build tag `verify`), copied into the same package by `run.sh`, which then runs `go test -tags verify -count=1 -v -run <regex> ./pkg/provider/kubernetes/gateway`.
- The harness (`verify/repro/verify_harness_test.go`) feeds the Gateway provider output (fake Kubernetes client, real provider) plus an optional `file` provider configuration through the production `server.NewConfigurationWatcher` (aggregation + `resolveHTTPTLSOptions`), builds routers with the production `server.NewRouterFactory`, selects TLS options from the SNI with the production `HTTP3TLSConfigMatcherFunc`, and dispatches the request through the production HTTPS handler chain (incl. `SNICheck`), the same way the eval fixture `evalHTTPSRuntimeFor` does.
- `ROUTER` lines show the aggregated router: rule, `ParentRefs`, entry points, TLS options as declared (`options=`) and as resolved by the aggregator (`resolved=`), runtime status and errors. `REQ` lines show SNI, Host, path, response status/body, and the TLS options name selected from the SNI.
- The file provider declares `tls.options.default.minVersion=VersionTLS13`, so `minVersion=0x304` on a `REQ` line shows the user-configured default options were selected.
- Sibling branches were fetched from `https://github.com/kaitranntt-evals/traefik-gateway-listener-isolation` into detached worktrees `~/repos/wt-<id>`:

```text
1d676a20 0cd33b0c27a4c2e7ed7779b7c135d3ce97fac8f1
12ff0e63 0847b94c34dd50d76f02abbc4f521a4ea5c8e803
91d5329e b8ca183de59db2a27db2dd26830ae61a6f6311fb
ad374a2e 44b3033d0e6b2f104f5196ebd03986f1b93661d0
c3d242fd 74955c6a93d3d0537040a9f068c1c49ee4a6782a
b6839099 82c63a8d5022197839853c8ee70acff2351da231
```

Baseline on the solution branch (fixtures and probes present in the package):

```text
$ go vet ./pkg/provider/kubernetes/gateway/...
vet exit=0
$ go test -count=1 -timeout=5m ./pkg/provider/kubernetes/gateway/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	5.327s
exit=0
$ go test -race -count=1 -run 'TestEval_' ./pkg/provider/kubernetes/gateway/
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	9.082s
$ go test -tags verify -race -count=1 -run 'TestVerify_C2C5|TestVerify_C3C6|TestVerify_C4C7' ./pkg/provider/kubernetes/gateway/
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	2.802s
```

The eval fixtures pass on the solution branch; the probes below log behaviour and do not assert, so their `ok` only means they ran.

## C1

Probe `TestVerify_C1_DocumentedTLSOptions` (`verify/repro/verify_c1_test.go`), run on each branch worktree. Scenarios: one Gateway with HTTPS listeners `wild=*.example.com` and `exact=foo.example.com` and hostname-free routes `/w` and `/e`; the same with the wildcard listener's route restricted to `bar.example.com`; a single exact listener; a single wildcard listener; two wildcard listeners (`*.example.com`, `*.foo.example.com`); and the two-listener Gateway beside a file-provider router ``Host("bar.example.com") && Path("/other")`` (a host of the wildcard listener) or ``Host("foo.example.com") && Path("/other")`` (the exact listener host) using the default TLS options, in non-strict and strict mode.

Command, per branch `<id>`:

```text
$ cd ~/repos/traefik && verify/repro/run.sh TestVerify_C1 ../wt-<id>
```

### C1 — evalon/traefik-ga-1d676a20

Documented (`git -C ../wt-1d676a20 diff 35c7530ac HEAD -- docs/content/reference/routing-configuration/kubernetes/gateway-api.md`, added lines):

```text
+HTTPS listeners use distinct TLS options to detect misdirected requests.
+When another provider declares a router with conflicting TLS options for the same hostname on the same EntryPoint, the affected routers continue serving with the default TLS options, following the [TLS options conflict behavior](../http/tls/tls-options.md#conflicting-tls-options).
```

Key output (request lines and declared TLS options per scenario; full router dumps are printed by the command above):

```text
=== RUN   TestVerify_C1_DocumentedTLSOptions
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-and-exact-listeners
PROVIDER TLS OPTIONS (2): [listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375 listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-listener-route-with-hostname
PROVIDER TLS OPTIONS (2): [listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375 listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 404 body="" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=baz.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=baz.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-exact-listener
PROVIDER TLS OPTIONS (1): [listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375]
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-wildcard-listener
PROVIDER TLS OPTIONS (1): [listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/two-wildcard-listeners
PROVIDER TLS OPTIONS (2): [listener-default-sub-gw-default-gw-ep-websecure-0-d3e4f37d57ba134602b7 listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/s      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-sub-gw-default-gw-ep-websecure-0-d3e4f37d57ba134602b7@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=a.foo.example.com      path=/s      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=a.foo.example.com      host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-sub-gw-default-gw-ep-websecure-0-d3e4f37d57ba134602b7@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host-strict
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-exact-gw-default-gw-ep-websecure-0-bda4ed5e9a574fd3b375@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-exact-host
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-wild-gw-default-gw-ep-websecure-0-6e989ccae51dcabd9083@kubernetesgateway" minVersion=0x303
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.393s
```

Observed: in normal operation each listener (also a single one) gets distinct options and crossed requests are 421, as documented. With the file-provider router on `bar.example.com` (default options), no Gateway router is marked conflicting or falls back: the wildcard listener routers keep their dedicated options while the SNI `bar.example.com` selects `default`, and the Gateway route `/w` on that host answers 421 in both modes instead of "continue serving with the default TLS options". The fallback holds only for the exact listener host (`foo.example.com` -> 200).

### C1 — evalon/traefik-ga-12ff0e63

Documented (`git -C ../wt-12ff0e63 diff 35c7530ac HEAD -- docs/content/reference/routing-configuration/kubernetes/gateway-api.md`, added lines):

```text
+To detect the requests misdirected to an HTTPS listener, Traefik dedicates a TLS options to each HTTPS listener defining a hostname,
+named `gateway-<namespace>-<gateway-name>-listener-<listener-name>`.
+The HTTPS listeners without hostname rely on the `default` TLS options.
+To reject the latter, a router named `gateway-<namespace>-<gateway-name>-ep-<entrypoint>-misdirected` catches,
+    When a router from another provider serves the same host name on the same entry point with other TLS options,
+    the conflict is resolved as described in the [TLS options](../http/tls/tls-options.md#conflicting-tls-options) documentation:
+    the routes keep being served, with the `default` TLS options, but the misdirected requests to that host name are no longer detected.
```

Key output (request lines and declared TLS options per scenario; full router dumps are printed by the command above):

```text
=== RUN   TestVerify_C1_DocumentedTLSOptions
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-and-exact-listeners
PROVIDER TLS OPTIONS (2): [gateway-default-gw-listener-exact gateway-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gateway-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gateway-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-listener-route-with-hostname
PROVIDER TLS OPTIONS (2): [gateway-default-gw-listener-exact gateway-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 404 body="" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=baz.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=foo.example.com        host=baz.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gateway-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-exact-listener
PROVIDER TLS OPTIONS (1): [gateway-default-gw-listener-exact]
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gateway-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-wildcard-listener
PROVIDER TLS OPTIONS (1): [gateway-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/two-wildcard-listeners
PROVIDER TLS OPTIONS (2): [gateway-default-gw-listener-sub gateway-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/s      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gateway-default-gw-listener-sub@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=a.foo.example.com      path=/s      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=a.foo.example.com      host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gateway-default-gw-listener-sub@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gateway-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host-strict
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gateway-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-exact-host
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gateway-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.430s
```

Observed: option names match the documented `gateway-<namespace>-<gateway-name>-listener-<listener-name>`. But (a) with the wildcard listener's route restricted to `bar.example.com`, the SNI `baz.example.com` (inside the wildcard listener hostname) selects `default`, not the listener's dedicated options; (b) with the file-provider router on `bar.example.com`, the Gateway route is not "kept being served with the default TLS options": its router keeps `gateway-default-gw-listener-wild`, the SNI selects `default`, and `/w` answers 421. The fallback holds only on the exact listener host.

### C1 — evalon/traefik-ga-91d5329e

Documented (`git -C ../wt-91d5329e diff 35c7530ac HEAD -- docs/content/reference/routing-configuration/kubernetes/gateway-api.md`, added lines):

```text
+Each HTTPS listener gets dedicated [TLS options](../http/tls/tls-options.md), used by the routers matching an exact hostname attached to it.
+A request whose TLS connection was established for the hostname of one listener (SNI), but whose `Host` header is served by another listener, is rejected with a `421 Misdirected Request` status code.
+    When a router from another provider serves the same host on the same entry point with different TLS options,
+    the conflict is arbitrated as documented in the [TLS options](../http/tls/tls-options.md#conflicting-tls-options) reference:
+    the routers fall back to the default TLS options and keep serving, unless `core.strictTLSOptions` is enabled.
```

Key output (request lines and declared TLS options per scenario; full router dumps are printed by the command above):

```text
=== RUN   TestVerify_C1_DocumentedTLSOptions
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-and-exact-listeners
PROVIDER TLS OPTIONS (1): [default-gw-exact]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=foo.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default-gw-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-listener-route-with-hostname
PROVIDER TLS OPTIONS (2): [default-gw-exact default-gw-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild@kubernetesgateway" minVersion=0x303
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=baz.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=foo.example.com        host=baz.example.com        path=/w      -> 404 body="404 page not found" | SNI-selected tlsOptions="default-gw-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-exact-listener
PROVIDER TLS OPTIONS (1): [default-gw-exact]
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-wildcard-listener
PROVIDER TLS OPTIONS (0): []
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/two-wildcard-listeners
PROVIDER TLS OPTIONS (0): []
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/s      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=bar.example.com        host=a.foo.example.com      path=/s      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host-strict
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-exact-host
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x304
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.423s
```

Observed: "Each HTTPS listener gets dedicated TLS options" does not hold for a wildcard listener with hostname-free routes: no options are declared for it (`PROVIDER TLS OPTIONS` lists only `default-gw-exact`, or nothing with wildcard-only listeners) and every name of its hostname space selects `default`. With two wildcard listeners both use `default`, and the crossed requests the same paragraph promises to reject with 421 are served 200. The documented cross-provider fallback itself holds here (200 on `bar.example.com` and `foo.example.com`).

### C1 — evalon/traefik-ga-ad374a2e

Documented (`git -C ../wt-ad374a2e diff 35c7530ac HEAD -- docs/content/reference/routing-configuration/kubernetes/gateway-api.md`, added lines):

```text
+To do so, Traefik associates to each HTTPS listener with a hostname dedicated TLS options, equivalent to the default ones.
+As for any other router, when another router serves the same host on the same entryPoint with different TLS options,
+the [conflict](../http/tls/tls-options.md#conflicting-tls-options) is resolved by falling back to the default TLS options,
+and the routes keep being served, unless [`core.strictTLSOptions`](../../install-configuration/configuration-options.md#opt-core-stricttlsoptions) is enabled.
```

Key output (request lines and declared TLS options per scenario; full router dumps are printed by the command above):

```text
=== RUN   TestVerify_C1_DocumentedTLSOptions
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-and-exact-listeners
PROVIDER TLS OPTIONS (2): [default-gw-exact-listener-48f206592a5e3e2c664a default-gw-wild-listener-a6fc5a2a9f113a1184f0]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact-listener-48f206592a5e3e2c664a@kubernetesgateway" minVersion=0x0
REQ sni=bar.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default-gw-exact-listener-48f206592a5e3e2c664a@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-listener-route-with-hostname
PROVIDER TLS OPTIONS (2): [default-gw-exact-listener-48f206592a5e3e2c664a default-gw-wild-listener-a6fc5a2a9f113a1184f0]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=baz.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=foo.example.com        host=baz.example.com        path=/w      -> 404 body="404 page not found" | SNI-selected tlsOptions="default-gw-exact-listener-48f206592a5e3e2c664a@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-exact-listener
PROVIDER TLS OPTIONS (0): []
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-wildcard-listener
PROVIDER TLS OPTIONS (0): []
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/two-wildcard-listeners
PROVIDER TLS OPTIONS (2): [default-gw-sub-listener-617bba644696e2d3c0b2 default-gw-wild-listener-a6fc5a2a9f113a1184f0]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/s      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-sub-listener-617bba644696e2d3c0b2@kubernetesgateway" minVersion=0x0
REQ sni=bar.example.com        host=a.foo.example.com      path=/s      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default-gw-sub-listener-617bba644696e2d3c0b2@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact-listener-48f206592a5e3e2c664a@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host-strict
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default-gw-exact-listener-48f206592a5e3e2c664a@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-exact-host
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default-gw-wild-listener-a6fc5a2a9f113a1184f0@kubernetesgateway" minVersion=0x0
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.425s
```

Observed: "each HTTPS listener with a hostname" gets dedicated options only when the Gateway has several HTTPS listeners: a single exact or wildcard listener declares none and uses `default`. With the wildcard listener's route restricted to `bar.example.com`, the SNI `baz.example.com` (inside the listener hostname) selects `default`. With the file-provider router on `bar.example.com`, the documented fallback does not happen: the Gateway router keeps its dedicated options, the SNI selects `default`, and `/w` answers 421 in non-strict mode.

### C1 — evalon/traefik-ga-c3d242fd

Documented (`git -C ../wt-c3d242fd diff 35c7530ac HEAD -- docs/content/migrate/v3.md`, added lines):

```text
+Each HTTPS listener with a hostname now gets its own TLS options, built from the Traefik default TLS options values,
+so that a request whose host name does not belong to the listener matched by its server name (SNI) is answered with a `421 Misdirected Request` status code.
+When another router declares the same host on the same entryPoint with different TLS options, the routers fall back to the `default` TLS options.
```

Key output (request lines and declared TLS options per scenario; full router dumps are printed by the command above):

```text
=== RUN   TestVerify_C1_DocumentedTLSOptions
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-and-exact-listeners
PROVIDER TLS OPTIONS (2): [gw-default-gw-listener-exact gw-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gw-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gw-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/wildcard-listener-route-with-hostname
PROVIDER TLS OPTIONS (2): [gw-default-gw-listener-exact gw-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 404 body="" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=baz.example.com        host=foo.example.com        path=/e      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=foo.example.com        host=baz.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gw-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-exact-listener
PROVIDER TLS OPTIONS (1): [gw-default-gw-listener-exact]
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gw-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/single-wildcard-listener
PROVIDER TLS OPTIONS (1): [gw-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/two-wildcard-listeners
PROVIDER TLS OPTIONS (2): [gw-default-gw-listener-sub gw-default-gw-listener-wild]
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/s      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gw-default-gw-listener-sub@kubernetesgateway" minVersion=0x303
REQ sni=bar.example.com        host=a.foo.example.com      path=/s      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=a.foo.example.com      host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="gw-default-gw-listener-sub@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gw-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-wildcard-host-strict
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=baz.example.com        host=baz.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="gw-default-gw-listener-exact@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C1_DocumentedTLSOptions/fallback-external-router-on-exact-host
REQ sni=foo.example.com        host=foo.example.com        path=/e      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/w      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="gw-default-gw-listener-wild@kubernetesgateway" minVersion=0x303
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.479s
```

Observed: listener options are selected only for the hostnames of the generated routers: with the wildcard listener's route restricted to `bar.example.com`, the SNI `baz.example.com` (inside the listener hostname) selects `default`. With the file-provider router on `bar.example.com`, "the routers fall back to the `default` TLS options" does not happen for the wildcard listener: its router keeps `gw-default-gw-listener-wild`, the SNI selects `default`, and `/w` answers 421. The fallback holds only on the exact listener host.

## C2

Probe `TestVerify_C2C5_BroadRouteBesideExternalExactHost` (`verify/repro/verify_c2_c5_test.go`): a hostname-free HTTPRoute `/gw` under one broad HTTPS listener (no hostname, or `*.example.com`), alone (control) and beside a file-provider leaf router ``Host("foo.example.com") && Path("/other")`` with default TLS options.

```text
$ cd ~/repos/traefik && verify/repro/run.sh 'TestVerify_C2C5'
```

```text
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/control-gateway-only
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-82dd4456916dac293d52@kubernetesgateway
rule=Path("/gw")
parents=[listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway
rule=Host("*")
parents=[] eps=[websecure] tls[options="listener-websecure-https-439feda8e3f1dbdca529" resolved="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/with-external-router
ROUTER ext-router@file
rule=Host("foo.example.com") && Path("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-82dd4456916dac293d52@kubernetesgateway
rule=Path("/gw")
parents=[listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway
rule=Host("*")
parents=[] eps=[websecure] tls[options="listener-websecure-https-439feda8e3f1dbdca529" resolved="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/control-gateway-only
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-0ed279ff964251bccace@kubernetesgateway
rule=Host("**.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/with-external-router
ROUTER ext-router@file
rule=Host("foo.example.com") && Path("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-0ed279ff964251bccace@kubernetesgateway
rule=Host("**.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost (0.17s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/control-gateway-only (0.07s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/with-external-router (0.02s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/control-gateway-only (0.04s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/with-external-router (0.04s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.198s
```

- conflict_resolution: with the external router present, the aggregated configuration keeps the Gateway listener parent on `resolved="listener-websecure-https-…@kubernetesgateway"` and the file router on `resolved="default"`; neither is renamed `websecure-conflicted-…` nor carries a conflict warning (status `enabled`), in contrast to the exact-hostname case in C4's control where both are.
- request_execution: SNI=Host=`foo.example.com` `/gw` is 200 without the external router and 421 `Misdirected Request` with it; the SNI selects `default` (`minVersion=0x304`) while the matched Gateway parent carries its dedicated options. `bar.example.com` is unaffected (200).

Impact reasoning: any hostname that another provider serves on the same entry point with default (or other) TLS options becomes unreachable on a catch-all or wildcard Gateway HTTPS listener whose routes declare no hostnames — the common "wildcard listener, hostname-free route" shape — while the task requires existing HTTPS routes to keep serving on cross-provider conflicts. Workaround observed: list the hostname explicitly on the HTTPRoute (C4 control `control-external-leaf` -> 200).

## C5

Same probe and run as C2 (`TestVerify_C2C5_BroadRouteBesideExternalExactHost`, `verify/repro/verify_c2_c5_test.go`): an unrestricted (hostname-free) HTTPRoute `/gw` beneath a broad HTTPS listener (no hostname, or `*.example.com`), alone (control) and beside a file-provider router ``Host("foo.example.com") && Path("/other")`` with default TLS options.

```text
$ cd ~/repos/traefik && verify/repro/run.sh 'TestVerify_C2C5'
```

```text
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/control-gateway-only
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-82dd4456916dac293d52@kubernetesgateway
rule=Path("/gw")
parents=[listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway
rule=Host("*")
parents=[] eps=[websecure] tls[options="listener-websecure-https-439feda8e3f1dbdca529" resolved="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/with-external-router
ROUTER ext-router@file
rule=Host("foo.example.com") && Path("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-82dd4456916dac293d52@kubernetesgateway
rule=Path("/gw")
parents=[listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway
rule=Host("*")
parents=[] eps=[websecure] tls[options="listener-websecure-https-439feda8e3f1dbdca529" resolved="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-439feda8e3f1dbdca529@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/control-gateway-only
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-0ed279ff964251bccace@kubernetesgateway
rule=Host("**.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/with-external-router
ROUTER ext-router@file
rule=Host("foo.example.com") && Path("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-0ed279ff964251bccace@kubernetesgateway
rule=Host("**.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=bar.example.com        host=bar.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost (0.17s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/control-gateway-only (0.07s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/catchall/with-external-router (0.02s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/control-gateway-only (0.04s)
--- PASS: TestVerify_C2C5_BroadRouteBesideExternalExactHost/wildcard/with-external-router (0.04s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.198s
```

- conflict_discovery: the Gateway child rule is `Path("/gw")` (catch-all) or `Host("**.example.com") && Path("/gw")` (wildcard) — it carries no exact hostname. In the aggregated configuration the file router resolves to `default` and the Gateway parent keeps its dedicated options; no router is renamed `websecure-conflicted-…` or warned.
- request_execution: SNI=Host=`foo.example.com` `/gw` -> 421 `Misdirected Request` with `SNI-selected tlsOptions="default"`, versus 200 in the control without the file router.

Impact reasoning: same as C2 — a file-provider router on one host silently turns the Gateway's broad route on that host into 421 for correctly addressed requests; the eval fixture `eval_tlsconflict_test.go` only covers a Gateway route with an exact hostname, so it passes.

## C3

Probe `TestVerify_C3C6_ChildHostnamePropagation` (`verify/repro/verify_c3_c6_test.go`): HTTPS listeners `outer=*.example.com` and `inner=*.foo.example.com` on the same entry point; a route constrained to `a.foo.example.com` (`/inner`) and a hostname-free route on `outer` (`/outer`). Scenarios: `control-sectioned` (every route names its listener), `unsectioned` (the constrained route attaches without `sectionName`, one Gateway), `sibling-gateways` (two Gateways sharing `websecure`; the constrained route names listener `outer` of `gw-outer`). Each in non-strict and strict TLS options mode.

```text
$ cd ~/repos/traefik && verify/repro/run.sh 'TestVerify_C3C6'
```

```text
=== RUN   TestVerify_C3C6_ChildHostnamePropagation
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/non-strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
ROUTER listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=enabled err=[]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
ROUTER listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=enabled err=[]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/unsectioned/non-strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/unsectioned/strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/non-strict
ROUTER httproute-default-r-inner-gw-default-gw-outer-ep-websecure-0-c1e443b6c4ec5cfea994@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-inner2-gw-default-gw-inner-ep-websecure-0-953c7d26b34d7417326b@kubernetesgateway
rule=Host("**.foo.example.com") && Path("/inner2")
parents=[websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-outer-ep-websecure-0-a6bc6c0aefb238792eb0@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/strict
ROUTER httproute-default-r-inner-gw-default-gw-outer-ep-websecure-0-c1e443b6c4ec5cfea994@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-inner2-gw-default-gw-inner-ep-websecure-0-953c7d26b34d7417326b@kubernetesgateway
rule=Host("**.foo.example.com") && Path("/inner2")
parents=[websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-outer-ep-websecure-0-a6bc6c0aefb238792eb0@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
--- PASS: TestVerify_C3C6_ChildHostnamePropagation (0.30s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/non-strict (0.10s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/strict (0.03s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/unsectioned/non-strict (0.04s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/unsectioned/strict (0.04s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/non-strict (0.08s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/strict (0.03s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.333s
```

- incompatible_propagation: in `unsectioned` and `sibling-gateways`, the child `Host("a.foo.example.com") && Path("/inner")` has both listener parents in `ParentRefs`, including the outer parent whose rule is `(Host("**.example.com")) && !(Host("**.foo.example.com"))` and therefore excludes `a.foo.example.com`. Both parents end up conflicting on that hostname (renamed `websecure-conflicted-…`, error "router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host"). In `control-sectioned` neither is.
- attachment_reachability: both the unsectioned attachment and the sibling-Gateway layout, loaded through the real provider from Gateway API objects, produce that configuration.
- strict_mode_effect: in strict mode both listener parents are `status=disabled`; every request, including the equal-SNI/Host ones that are 200 in the control (`/inner`, `/outer`) and `/inner2` in the sibling layout, is 404.
- non_strict_mode_effect: in non-strict mode both parents resolve to `default`; the crossed requests (SNI `b.example.com` + Host `a.foo.example.com`, and the reverse) are served 200 instead of the 421 seen in the control.

Impact reasoning: an HTTPRoute with an exact hostname attached without `sectionName` to a Gateway with nested wildcard HTTPS listeners (or the equivalent across two Gateways on the same port) is an ordinary Gateway API configuration, with no other provider involved. It silently removes misdirected-request detection for both listeners (non-strict, the default), or takes down every route of both listeners (strict). Workaround observed: attach with `sectionName` within a single Gateway (control).

## C6

Probe `TestVerify_C3C6_ChildHostnamePropagation` (`verify/repro/verify_c3_c6_test.go`): HTTPS listeners `outer=*.example.com` and `inner=*.foo.example.com` on the same entry point; a route constrained to `a.foo.example.com` (`/inner`) and a hostname-free route on `outer` (`/outer`). Scenarios: `control-sectioned` (every route names its listener), `unsectioned` (the constrained route attaches without `sectionName`, one Gateway), `sibling-gateways` (two Gateways sharing `websecure`; the constrained route names listener `outer` of `gw-outer`). Each in non-strict and strict TLS options mode.

```text
$ cd ~/repos/traefik && verify/repro/run.sh 'TestVerify_C3C6'
```

```text
=== RUN   TestVerify_C3C6_ChildHostnamePropagation
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/non-strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
ROUTER listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=enabled err=[]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
ROUTER listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=enabled err=[]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway" minVersion=0x0
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/unsectioned/non-strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/unsectioned/strict
ROUTER httproute-default-r-inner-gw-default-gw-ep-websecure-0-7c185e754ec7f0615b30@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-ep-websecure-0-f03ed948ff65fb13ab06@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/non-strict
ROUTER httproute-default-r-inner-gw-default-gw-outer-ep-websecure-0-c1e443b6c4ec5cfea994@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-inner2-gw-default-gw-inner-ep-websecure-0-953c7d26b34d7417326b@kubernetesgateway
rule=Host("**.foo.example.com") && Path("/inner2")
parents=[websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-outer-ep-websecure-0-a6bc6c0aefb238792eb0@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 200 body="10.10.1.1" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 200 body="10.10.2.1" | SNI-selected tlsOptions="default" minVersion=0x303
=== RUN   TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/strict
ROUTER httproute-default-r-inner-gw-default-gw-outer-ep-websecure-0-c1e443b6c4ec5cfea994@kubernetesgateway
rule=Host("a.foo.example.com") && Path("/inner")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-inner2-gw-default-gw-inner-ep-websecure-0-953c7d26b34d7417326b@kubernetesgateway
rule=Host("**.foo.example.com") && Path("/inner2")
parents=[websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER httproute-default-r-outer-gw-default-gw-outer-ep-websecure-0-a6bc6c0aefb238792eb0@kubernetesgateway
rule=Host("**.example.com") && Path("/outer")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=(Host("**.example.com")) && !(Host("**.foo.example.com"))
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
ROUTER websecure-conflicted-listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway
rule=Host("**.foo.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-foo-example-com-1822c2677801f90ccf48" resolved="listener-websecure-https-foo-example-com-1822c2677801f90ccf48@kubernetesgateway"] status=disabled err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host]
-- equal SNI/Host (must be served)
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
-- crossed SNI/Host between the two listeners (must be 421)
REQ sni=b.example.com          host=a.foo.example.com      path=/inner  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=b.example.com          host=a.foo.example.com      path=/inner2 -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
REQ sni=a.foo.example.com      host=b.example.com          path=/outer  -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x303
--- PASS: TestVerify_C3C6_ChildHostnamePropagation (0.30s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/non-strict (0.10s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/control-sectioned/strict (0.03s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/unsectioned/non-strict (0.04s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/unsectioned/strict (0.04s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/non-strict (0.08s)
--- PASS: TestVerify_C3C6_ChildHostnamePropagation/sibling-gateways/strict (0.03s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.333s
```

- incompatible_propagation: in `unsectioned` and `sibling-gateways`, the child `Host("a.foo.example.com") && Path("/inner")` has both listener parents in `ParentRefs`, including the outer parent whose rule is `(Host("**.example.com")) && !(Host("**.foo.example.com"))` and therefore excludes `a.foo.example.com`. Both parents end up conflicting on that hostname (renamed `websecure-conflicted-…`, error "router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host"). In `control-sectioned` neither is.
- attachment_reachability: both the unsectioned attachment and the sibling-Gateway layout, loaded through the real provider from Gateway API objects, produce that configuration.
- strict_mode_effect: in strict mode both listener parents are `status=disabled`; every request, including the equal-SNI/Host ones that are 200 in the control (`/inner`, `/outer`) and `/inner2` in the sibling layout, is 404.

Impact reasoning: with `core.strictTLSOptions` enabled, one constrained route attached without `sectionName` (or sibling Gateways on one port) disables both listener parent routers, so all routes of both listeners return 404 although no real TLS options conflict exists (both option sets are copies of the defaults and no other provider is involved). Workaround observed: attach with `sectionName` within a single Gateway (control).

## C4

Probe `TestVerify_C4C7_WildcardRouteBesideExternalParent` (`verify/repro/verify_c4_c7_test.go`): Gateway HTTPS listener `*.example.com` with an HTTPRoute for hostname `foo.example.com` serving `/gw`, beside a file-provider router for the same host with default TLS options, as a leaf (control, the shape `eval_tlsconflict_test.go` covers) and as a parent router with a child (`ext-parent` + `ext-child`), with variants on the external parent's priority and rule, and a control with the external parent on an unrelated host.

```text
$ cd ~/repos/traefik && verify/repro/run.sh 'TestVerify_C4C7'
```

```text
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-leaf
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-ext-router@file
rule=Host("foo.example.com") && Path("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("foo.example.com")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-priority1
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("foo.example.com")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-short-host
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("a.example.com")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-2ddcb509115762d3db9e@kubernetesgateway
rule=Host("a.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=a.example.com          host=a.example.com          path=/gw     -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=a.example.com          host=a.example.com          path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-scoped-to-other-path
ROUTER ext-child@file
rule=Method("GET")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("foo.example.com") && PathPrefix("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-parent-other-host-priority1
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("other.example.org")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent (0.35s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-leaf (0.13s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent (0.06s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-priority1 (0.02s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-short-host (0.02s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-scoped-to-other-path (0.08s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-parent-other-host-priority1 (0.05s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.385s
```

- hostname_suppression: with an external leaf (`control-external-leaf`) the conflict is found: the Gateway parent and the file router are renamed `websecure-conflicted-…`, the Gateway parent has `resolved="default"` with the fallback warning, and `/gw` is 200. With an external parent that has a child (all `external-parent*` scenarios) nothing is resolved: the Gateway parent keeps `resolved="listener-websecure-https-example-com-…@kubernetesgateway"`, status `enabled`, no rename, while `ext-parent@file` resolves to `default`. The only difference between the two is that the external router owning `foo.example.com` has children.
- request_execution: the SNI `foo.example.com` selects `default` in every external-parent scenario. Whenever the request reaches the Gateway parent it is rejected: `external-parent-scoped-to-other-path` (external parent `Host("foo.example.com") && PathPrefix("/other")`, default priorities) `/gw` -> 421; `external-parent-priority1` `/gw` -> 421 (and `/other` -> 421 too, as the Gateway parent then also shadows the external one). With a bare `Host("foo.example.com")` external parent at default priority the external parent wins the match and `/gw` is 404 from the external tree, i.e. the request never reaches the Gateway route. Control with the external parent on `other.example.org`: `/gw` -> 200.

Impact reasoning: a parent/child router tree from another provider on a hostname that a Gateway wildcard listener's route also serves defeats the cross-provider fallback that works for leaf routers; correctly addressed requests to the Gateway route get 421. Triggering needs the multi-layer routing feature in the other provider plus a host overlap, so it is less common than C2/C5. No Gateway-side workaround observed.

## C7

Probe `TestVerify_C4C7_WildcardRouteBesideExternalParent` (`verify/repro/verify_c4_c7_test.go`): Gateway HTTPS listener `*.example.com` with an HTTPRoute for hostname `foo.example.com` serving `/gw`, beside a file-provider router for the same host with default TLS options, as a leaf (control, the shape `eval_tlsconflict_test.go` covers) and as a parent router with a child (`ext-parent` + `ext-child`), with variants on the external parent's priority and rule, and a control with the external parent on an unrelated host.

```text
$ cd ~/repos/traefik && verify/repro/run.sh 'TestVerify_C4C7'
```

```text
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-leaf
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER websecure-conflicted-ext-router@file
rule=Host("foo.example.com") && Path("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER websecure-conflicted-listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="default"] status=warning err=[router's TLSOptions configuration is conflicting with other routers on the same entrypoint and host, default TLS options will be used instead]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("foo.example.com")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-priority1
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("foo.example.com")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-short-host
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("a.example.com")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-2ddcb509115762d3db9e@kubernetesgateway
rule=Host("a.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=a.example.com          host=a.example.com          path=/gw     -> 404 body="404 page not found" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=a.example.com          host=a.example.com          path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-scoped-to-other-path
ROUTER ext-child@file
rule=Method("GET")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("foo.example.com") && PathPrefix("/other")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 421 body="Misdirected Request" | SNI-selected tlsOptions="default" minVersion=0x304
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 200 body="ext-backend" | SNI-selected tlsOptions="default" minVersion=0x304
=== RUN   TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-parent-other-host-priority1
ROUTER ext-child@file
rule=Path("/other")
parents=[ext-parent@file] eps=[] tls[-] status=enabled err=[]
ROUTER ext-parent@file
rule=Host("other.example.org")
parents=[] eps=[websecure] tls[options="" resolved="default"] status=enabled err=[]
ROUTER httproute-default-gw-route-gw-default-broad-gw-ep-websecure-0-95ff86aeb9335cd5ddb0@kubernetesgateway
rule=Host("foo.example.com") && Path("/gw")
parents=[listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway] eps=[] tls[-] status=enabled err=[]
ROUTER listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway
rule=Host("**.example.com")
parents=[] eps=[websecure] tls[options="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb" resolved="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway"] status=enabled err=[]
REQ sni=foo.example.com        host=foo.example.com        path=/gw     -> 200 body="10.10.0.1" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
REQ sni=foo.example.com        host=foo.example.com        path=/other  -> 404 body="404 page not found" | SNI-selected tlsOptions="listener-websecure-https-example-com-aa0f3a4a63da2a2e7cbb@kubernetesgateway" minVersion=0x0
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent (0.35s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-leaf (0.13s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent (0.06s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-priority1 (0.02s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-short-host (0.02s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/external-parent-scoped-to-other-path (0.08s)
--- PASS: TestVerify_C4C7_WildcardRouteBesideExternalParent/control-external-parent-other-host-priority1 (0.05s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.385s
```

- hostname_suppression: with an external leaf (`control-external-leaf`) the conflict is found: the Gateway parent and the file router are renamed `websecure-conflicted-…`, the Gateway parent has `resolved="default"` with the fallback warning, and `/gw` is 200. With an external parent that has a child (all `external-parent*` scenarios) nothing is resolved: the Gateway parent keeps `resolved="listener-websecure-https-example-com-…@kubernetesgateway"`, status `enabled`, no rename, while `ext-parent@file` resolves to `default`. The only difference between the two is that the external router owning `foo.example.com` has children.
- request_execution: the SNI `foo.example.com` selects `default` in every external-parent scenario. Whenever the request reaches the Gateway parent it is rejected: `external-parent-scoped-to-other-path` (external parent `Host("foo.example.com") && PathPrefix("/other")`, default priorities) `/gw` -> 421; `external-parent-priority1` `/gw` -> 421 (and `/other` -> 421 too, as the Gateway parent then also shadows the external one). With a bare `Host("foo.example.com")` external parent at default priority the external parent wins the match and `/gw` is 404 from the external tree, i.e. the request never reaches the Gateway route. Control with the external parent on `other.example.org`: `/gw` -> 200.

Impact reasoning: a parent/child router tree from another provider on a hostname that a Gateway wildcard listener's route also serves defeats the cross-provider fallback that works for leaf routers; correctly addressed requests to the Gateway route get 421. Triggering needs the multi-layer routing feature in the other provider plus a host overlap, so it is less common than C2/C5. No Gateway-side workaround observed.

## C8

Target: `evalon/traefik-ga-b6839099` at `82c63a8d5022197839853c8ee70acff2351da231` (worktree `~/repos/wt-b6839099`). Its `loadMisdirectedRequestsDetection` names listener TLS options `provider.Normalize(fmt.Sprintf("listener-%s-%s-%s", gateway.Namespace, gateway.Name, listener.Name))`.

Probe `TestVerify_C8_NormalizedListenerNames` (`verify/repro/verify_c8_test.go`): one Gateway with two HTTPS listeners on hostnames `a.example.com` and `b.example.com`, each with one route (`/a`, `/b`); listener names `edge-a`/`edge-b` (control) and `edge-a`/`edge--a`.

```text
$ cd ~/repos/traefik && verify/repro/run.sh TestVerify_C8 ../wt-b6839099
```

```text
=== RUN   TestVerify_C8_NormalizedListenerNames
=== RUN   TestVerify_C8_NormalizedListenerNames/control-edge-a-edge-b
PROVIDER TLS OPTIONS (2): [listener-default-gw-edge-a listener-default-gw-edge-b]
ROUTER httproute-default-r-a-gw-default-gw-ep-websecure-0-1e3ec90b1a17659970c5@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-a" resolved="listener-default-gw-edge-a@kubernetesgateway"] status=enabled err=[]
ROUTER httproute-default-r-b-gw-default-gw-ep-websecure-0-907003a734d6281f0de9@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-b" resolved="listener-default-gw-edge-b@kubernetesgateway"] status=enabled err=[]
ROUTER listener-default-edge-a-gw-default-gw-ep-websecure-0-1a200e019af056554064@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-a" resolved="listener-default-gw-edge-a@kubernetesgateway"] status=enabled err=[]
ROUTER listener-default-edge-b-gw-default-gw-ep-websecure-0-f521f1e7e1d445aa878e@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-b" resolved="listener-default-gw-edge-b@kubernetesgateway"] status=enabled err=[]
-- equal SNI/Host
REQ sni=a.example.com          host=a.example.com          path=/a      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-gw-edge-a@kubernetesgateway" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/b      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-gw-edge-b@kubernetesgateway" minVersion=0x303
-- crossed SNI/Host (must be 421)
REQ sni=a.example.com          host=b.example.com          path=/b      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-gw-edge-a@kubernetesgateway" minVersion=0x303
REQ sni=b.example.com          host=a.example.com          path=/a      -> 421 body="Misdirected Request" | SNI-selected tlsOptions="listener-default-gw-edge-b@kubernetesgateway" minVersion=0x303
=== RUN   TestVerify_C8_NormalizedListenerNames/edge-a-edge--a
PROVIDER TLS OPTIONS (1): [listener-default-gw-edge-a]
ROUTER httproute-default-r-a-gw-default-gw-ep-websecure-0-1e3ec90b1a17659970c5@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-a" resolved="listener-default-gw-edge-a@kubernetesgateway"] status=enabled err=[]
ROUTER httproute-default-r-b-gw-default-gw-ep-websecure-0-907003a734d6281f0de9@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-a" resolved="listener-default-gw-edge-a@kubernetesgateway"] status=enabled err=[]
ROUTER listener-default-edge-a-gw-default-gw-ep-websecure-0-1a200e019af056554064@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-a" resolved="listener-default-gw-edge-a@kubernetesgateway"] status=enabled err=[]
ROUTER listener-default-edge-a-gw-default-gw-ep-websecure-0-42acaee1b30eb49066af@kubernetesgateway
parents=[] eps=[websecure] tls[options="listener-default-gw-edge-a" resolved="listener-default-gw-edge-a@kubernetesgateway"] status=enabled err=[]
-- equal SNI/Host
REQ sni=a.example.com          host=a.example.com          path=/a      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-gw-edge-a@kubernetesgateway" minVersion=0x303
REQ sni=b.example.com          host=b.example.com          path=/b      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-gw-edge-a@kubernetesgateway" minVersion=0x303
-- crossed SNI/Host (must be 421)
REQ sni=a.example.com          host=b.example.com          path=/b      -> 200 body="10.10.2.1" | SNI-selected tlsOptions="listener-default-gw-edge-a@kubernetesgateway" minVersion=0x303
REQ sni=b.example.com          host=a.example.com          path=/a      -> 200 body="10.10.1.1" | SNI-selected tlsOptions="listener-default-gw-edge-a@kubernetesgateway" minVersion=0x303
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway	0.222s
```

Control: two TLS options (`listener-default-gw-edge-a`, `listener-default-gw-edge-b`), crossed SNI/Host requests -> 421. With `edge-a`/`edge--a`: both listeners are accepted and serve their routes (equal requests 200), the provider declares a single TLS options entry `listener-default-gw-edge-a` used by both listeners' routers, and both crossed requests are served 200 instead of 421.

Impact reasoning: two listeners whose names differ only in characters that normalization collapses (a valid, if unusual, pair of names) lose misdirected-request detection between them with no error or status condition, while the Gateway advertises `GatewayHTTPSListenerDetectMisdirectedRequests`. Workaround: rename one listener.
