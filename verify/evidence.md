# Evidence — traefik secret references in middlewares (run v-bd0d6d7a)

Audited branch: `origin/traefik-secret-references-in-middlewares-perfect` @ `5ca0225b2` (base `af6d927c5`), checked out as `verify/traefik-secret-references-in-middlewares-v-bd0d6d7a` in `~/repos/traefik`.
Claim-target branches were fetched from `kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into detached worktrees: `~/wt/902c56a6` @ `4601514c3`, `~/wt/64d2c141` @ `4b934c1c5`, `~/wt/d4a0f793` @ `4d72dd4af`, `~/wt/86df1f82` @ `7b46757de`, `~/wt/8ca00183` @ `f4c9f7fa0`, `~/wt/9568168b` @ `a1742e8cc`, `~/wt/cff842f1` @ `0424766a4`.
Fixtures from `eval_tests.zip` were extracted to `~/eval_tests/tests/` and copied byte-exact to the paths the prompt lists before each fixture run; they were removed again afterwards so production packages stay untouched.

Baseline on the audited branch (`verify/run_main_checks.sh`):

```sh
cd ~/repos/traefik && go test -count=1 -timeout=5m ./pkg/config/dynamic/... ./pkg/server/middleware/... ./pkg/middlewares/auth/... ./pkg/provider/kubernetes/crd/... ./pkg/provider/kubernetes/ingress-nginx/...   # exit=0
cd ~/repos/traefik && go vet ./pkg/config/dynamic/... ./pkg/server/middleware/... ./pkg/middlewares/auth/... ./pkg/provider/kubernetes/crd/... ./pkg/provider/kubernetes/ingress-nginx/...                        # exit=0
# all six fixtures copied to their listed paths, then:
go test -v -race -count=1 -timeout=5m -run '^TestEval_' ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx ./pkg/middlewares/auth   # exit=0
```

```console
--- PASS: TestEval_CRD_TypedBasicAuth_SecretRetention (0.03s)
--- PASS: TestEval_ProviderToBuilder_ForwardAuthClientCertificate (1.03s)
--- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (0.36s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	2.586s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	2.175s
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	1.133s
```

(All 27 `TestEval_*` fixtures pass on the audited branch.)

## C1

Target: `evalon/traefik-se-902c56a6` (worktree `~/wt/902c56a6` @ `4601514c3`). Claim: typed BasicAuth Secret usernames appear in publicly serialized middleware user entries.

Source inspected: `pkg/provider/kubernetes/crd/kubernetes.go` `loadBasicAuthCredentials` (lines ~1325-1339) reads `secret.Data["username"]` and returns `string(username) + ":" + ref`, where only the password is turned into `urn:k8s:secret:<name>:password`.

Command (fixture copied byte-exact from the archive; `export_eval_test.go` is needed for the exported helpers):

```sh
cd ~/wt/902c56a6
cp ~/eval_tests/tests/eval_crd_behavior_test.go ~/eval_tests/tests/export_eval_test.go pkg/provider/kubernetes/crd/
go test -count=1 -run 'TestEval_CRD_TypedBasicAuth_SecretRetention' -v ./pkg/provider/kubernetes/crd
```

Key output:

```console
=== RUN   TestEval_CRD_TypedBasicAuth_SecretRetention
    eval_crd_behavior_test.go: user entry must be a URN reference, got "evalq7user:urn:k8s:secret:typed-auth-sec:password"
    "evalq7user:urn:k8s:secret:typed-auth-sec:password" should not contain "evalq7user"
    JSON serialization must not leak Secret username
    YAML serialization must not leak Secret username
--- FAIL: TestEval_CRD_TypedBasicAuth_SecretRetention
FAIL
```

The JSON the fixture serialized (what `/api/rawdata`, the dashboard and support dumps carry) contained `{"http":{"middlewares":{"default-typed-basicauth":{"basicAuth":{"users":["evalq7user:urn:k8s:secret:typed-auth-sec:password"]}}}}`.

Impact: the `username` key of a typed BasicAuth Secret is a Secret-managed value; on this branch it is copied verbatim into `dynamic.BasicAuth.Users`, so it is exposed in the routing configuration, API, dashboard, debug logs and support dumps for every IngressRoute that uses the typed Secret form (the ordinary CRD way to configure BasicAuth from a Secret). Only the password hash is referenced. Workaround: use the htpasswd `users` key form instead of `username`/`password` keys.

Repro: `verify/repro/c1_typed_basicauth_username_leak.sh`.

For comparison the same fixture passes on the audited branch (`--- PASS: TestEval_CRD_TypedBasicAuth_SecretRetention (0.03s)`, see baseline above).

## C2

Targets: `evalon/traefik-se-64d2c141` (`~/wt/64d2c141` @ `4b934c1c5`), `evalon/traefik-se-d4a0f793` (`~/wt/d4a0f793` @ `4d72dd4af`), `evalon/traefik-se-86df1f82` (`~/wt/86df1f82` @ `7b46757de`). Claim: the delivered tests fail to assert resolved nested credentials plus preservation of non-secret siblings across multiple middleware types.

Method: mutation testing of each branch's resolver (`ResolveSecrets`/`resolveSecrets`), then running the delivered suites. Exact mutations are in `verify/repro/c2_mutations/<branch>_M1.diff` and `<branch>_M2.diff` (produced by the `m1_*.py`/`m2_*.py` scripts next to them):

- **M1** – a Secret reference nested inside a plugin map (depth ≥ 1 below the plugin's top-level keys) resolves to the literal `"MUTATED"` instead of the Secret value.
- **M2** – after resolution, non-secret siblings are dropped: `ForwardAuth.Address = ""`, `RateLimit.Redis.Endpoints = nil`, and every top-level plugin string value that is not a Secret reference is deleted.

Command per mutation (runner `verify/repro/c2_mutations/run_mut.sh` applies, tests, and reverts):

```sh
cd ~/wt/<branch> && python3 <mX_branch.py>
go test -count=1 ./pkg/config/dynamic ./pkg/server/middleware ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx
git checkout -- <mutated file>
```

Output (only FAIL/ok lines kept):

```console
### 64d2c141_M1
--- FAIL: TestMiddlewareSecretSnapshot (0.01s)
FAIL	github.com/traefik/traefik/v3/pkg/config/dynamic	0.027s
--- FAIL: TestBuilderSecretPluginIsolation (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.046s
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.01s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_deep_secret (0.01s)
--- FAIL: TestMiddlewareSecretReferences (0.04s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.783s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.802s
### 64d2c141_M2
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.038s
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.040s
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route_with_middleware_ratelimit (0.01s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_secret (0.02s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_basic_auth_middleware (0.02s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.702s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.693s
### d4a0f793_M1
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.033s
--- FAIL: TestResolveSecrets (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.084s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	1.288s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	1.215s
### d4a0f793_M2
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.018s
--- FAIL: TestResolveSecrets (0.00s)
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.038s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.652s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.647s
### 86df1f82_M1
--- FAIL: TestMiddlewareResolveSecrets (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/config/dynamic	0.024s
--- FAIL: TestBuilderSecretPluginErrors (0.00s)
    --- FAIL: TestBuilderSecretPluginErrors/false/"private-token" (0.00s)
    (… 10 subtests …)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.058s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.612s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.692s
### 86df1f82_M2
--- FAIL: TestMiddlewareResolveSecrets (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/config/dynamic	0.025s
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.051s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.630s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.501s
```

`d4a0f793_M2` detail (`go test -count=1 -run 'TestResolveSecrets$' -v ./pkg/server/middleware`):

```console
Error Trace:	/home/ubuntu/wt/d4a0f793/pkg/server/middleware/secrets_test.go:50
            	expected: string("prefix urn:k8s:secret:credentials:key")
            	actual  : <nil>(<nil>)
Test:       	TestResolveSecrets
```

Reading: on all three branches a corrupted nested plugin value is caught (M1), and dropped non-secret siblings on ForwardAuth (`Address`, via `TestBuilderSecretForwardAuthTLS`) and on plugins (literal sibling via `TestResolveSecrets`/`TestMiddlewareResolveSecrets`; on 64d2c141 also Redis `Endpoints` via the ratelimit snapshot and the `read_config_from_secret` snapshot) are caught (M2). The delivered tests therefore do assert resolved nested credentials together with preserved non-secret siblings across at least two middleware types on every branch. Per-branch result: 64d2c141 REFUTED, d4a0f793 REFUTED, 86df1f82 REFUTED.

## C3

Target: audited branch (`~/repos/traefik` @ `5ca0225b2`). Claim: plugin construction with a nil `PluginsBuilder` causes a reflection panic, and production reaches that path.

Source: `pkg/server/middleware/middlewares.go:413`:

```go
if config.Plugin != nil && !reflect.ValueOf(b.pluginBuilder).IsNil() { // Using "reflect" because "b.pluginBuilder" is an interface.
```

This line is unchanged from the base commit (`git show af6d927c5:pkg/server/middleware/middlewares.go` line 408 has the identical statement), i.e. it is pre-existing Traefik code, not introduced by the branch.

Test (`verify/repro/c3_nil_pluginbuilder_test.go.txt`, copied into `pkg/server/middleware/` as `verify_c3_test.go` while running):

```sh
cd ~/repos/traefik && go test -count=1 -run 'TestVerifyC3' -v ./pkg/server/middleware
```

```console
=== RUN   TestVerifyC3_NilInterface
    verify_c3_test.go: recovered panic: reflect: call of reflect.Value.IsNil on zero Value
=== RUN   TestVerifyC3_TypedNilPointer
    verify_c3_test.go: constructor=<nil> err=invalid middleware "plug" configuration: invalid middleware type or middleware does not exist
    verify_c3_test.go: recovered panic: <nil>
PASS
```

- `NewBuilder(configs, nil, nil)` (a nil *interface*) panics in `reflect.Value.IsNil` when a Plugin middleware is built — the reflection part holds.
- `NewBuilder(configs, nil, (*plugins.Builder)(nil))` (a typed nil pointer, the only shape production can produce) does not panic; the middleware is reported as an invalid type.
- Production wiring: `cmd/traefik/traefik.go` `createPluginBuilder` returns `*plugins.Builder` (a concrete pointer, possibly nil when plugins are disabled) which flows through `server.NewRouterFactory` → `middleware.NewBuilder`; a nil interface value is never constructed, so the panic path is unreachable from the binary. The production-trigger part does not hold → REFUTED.

## C4

Target: audited branch (`~/repos/traefik` @ `5ca0225b2`). Claim: ForwardAuth TLS construction exposes Secret-derived filesystem paths in certificate/key loading errors.

Source: `pkg/types/tls.go` `loadKeyPair` (lines 80-95): tries `tls.X509KeyPair([]byte(cert), []byte(key))`, then `os.Stat(cert)` / `os.Stat(key)`, then `tls.LoadX509KeyPair(cert, key)` whose error embeds the path. `pkg/server/middleware/middlewares.go:224` calls `auth.NewForward(ctx, next, *config.ForwardAuth, middlewareName)` on the resolved config; only the Plugin branch (lines 426/432) wraps errors with `info.RedactURNRefs`, ForwardAuth errors are returned as-is.

Test (`verify/repro/c4_forwardauth_tls_path_leak_test.go`, copied into `pkg/server/middleware/`): a ForwardAuth middleware with `TLS.Cert`/`TLS.Key` set to `urn:k8s:secret:tlssecret:tls.crt`/`tls.key`, `URNRefs` resolving both to an existing directory `<tmp>/SECRET-CERT-DIR-xyz` (a Secret value that is a path to an existing filesystem entry), built through `Builder.BuildMiddlewareChain(...).Then(...)`.

```sh
cd ~/repos/traefik && go test -count=1 -run 'TestVerifyC4' -v ./pkg/server/middleware
```

```console
=== RUN   TestVerifyC4_ForwardAuthSecretDirPathInError
    verify_c4_test.go: constructor error: unable to create client TLS configuration: read /tmp/TestVerifyC4_ForwardAuthSecretDirPathInError3285085307/001/SECRET-CERT-DIR-xyz: is a directory
    verify_c4_test.go: runtime MiddlewareInfo.Err (what /api/http/middlewares exposes): [unable to create client TLS configuration: read /tmp/TestVerifyC4_ForwardAuthSecretDirPathInError3285085307/001/SECRET-CERT-DIR-xyz: is a directory]
    verify_c4_test.go: dynamic config still holds refs: cert="urn:k8s:secret:tlssecret:tls.crt" key="urn:k8s:secret:tlssecret:tls.key"
--- PASS: TestVerifyC4_ForwardAuthSecretDirPathInError (0.00s)
```

Lower-level confirmation (`verify/repro/c7_loadkeypair_path_in_error_test.go` copied into `pkg/types/`):

```sh
cd ~/repos/traefik && go test -count=1 -run 'TestVerifyC4' -v ./pkg/types
```

```console
    verify_c4_test.go: loadKeyPair error: read /tmp/TestVerifyC4_LoadKeyPairDirectoryPaths3495941400/001/SECRET-CERT-DIR-xyz: is a directory
    verify_c4_test.go: loadKeyPair(/etc/hostname,/etc/hostname): tls: failed to find any PEM data in certificate input
```

Impact: the Secret-derived value (the resolved content of `tls.crt`/`tls.key`) is echoed back verbatim inside the middleware's construction error when it names an existing directory (or any path for which `os.Stat` succeeds but reading fails). That error is stored in `runtime.MiddlewareInfo.Err`, which the API (`/api/http/middlewares`), dashboard, and logs expose, while the routing configuration itself correctly still shows only the `urn:` references. The triggering situation (Secret contents that happen to be an existing path) is unusual for real certificates; the common misconfiguration (non-PEM content) produces a path-free error. CONFIRMED (behavior holds; exposure limited to the odd-value case).

## C5

Target: `evalon/traefik-se-8ca00183` (`~/wt/8ca00183` @ `f4c9f7fa0`). Claim: plugin constructor-error sanitization depends on matching resolved Secret text.

Source: `pkg/server/middleware/secrets.go` `secretSafeError` lines 49-51:

```go
	// so the whole message is withheld instead.
	if config.ContainsSecretValue(err.Error()) {
		return fmt.Errorf("error withheld as it contains the value of one of %s", strings.Join(config.SecretReferences(), ", "))
```

Test (`verify/repro/c5_plugin_ctor_error_oracle_test.go`, copied into `pkg/server/middleware/`): a fake `plugins.PluginsBuilder` whose `Build` succeeds but whose constructor returns a fixed error; the middleware has one plugin field `token: urn:k8s:secret:plugsecret:token` whose resolved value is varied.

```sh
cd ~/wt/8ca00183 && go test -count=1 -run TestVerifyC5 -v ./pkg/server/middleware
```

```console
    secret equals a word of the message           ctorErr="invalid token" -> exposed="error withheld as it contains the value of one of urn:k8s:secret:plugsecret:token" api=[error withheld as it contains the value of one of urn:k8s:secret:plugsecret:token]
    unrelated secret                              ctorErr="invalid token" -> exposed="invalid token" api=[invalid token]
    whitespace-only secret                        ctorErr="invalid token" -> exposed="invalid token" api=[invalid token]
    secret with newline, quoted in message        ctorErr="plugin rejected \"tok\\nen\"" -> exposed="error withheld as it contains the value of one of urn:k8s:secret:plugsecret:token" api=[error withheld as it contains the value of one of urn:k8s:secret:plugsecret:token]
    wrapped, secret=token                         -> exposed="error withheld as it contains the value of one of urn:k8s:secret:plugsecret:token"
--- PASS: TestVerifyC5_ConstructorErrorDependsOnSecretText (0.00s)
```

Impact: the identical constructor error `invalid token` is exposed to the API/dashboard when the Secret value is `s3cr3t-unrelated`, but replaced by a "withheld" message when the Secret value happens to be the word `token`. Whether an operator sees the real diagnostic therefore depends on the Secret's text, not on the error; a Secret value that is whitespace is never matched (so a value consisting of spaces would be echoed if a plugin included it), and legitimate diagnostics disappear whenever a Secret value coincides with any word in the message. This is the everyday plugin-misconfiguration path (any plugin that reports its own error). CONFIRMED.

## C6

Target: `evalon/traefik-se-8ca00183` (`~/wt/8ca00183` @ `f4c9f7fa0`). Claim: provider tests omit assertions of resolved nested plugin and Ingress default-backend credentials.

Inspection: `pkg/provider/kubernetes/crd/kubernetes_test.go` and `pkg/provider/kubernetes/ingress-nginx/kubernetes_test.go` call `clearSecretValues(conf)` before `assert.Equal`; the dedicated `crd/secrets_test.go` covers `with_auth.yml`, `with_plugin_read_secret.yml` (top-level plugin key) and `with_ratelimit`, not `with_plugin_deep_read_secret.yml` (`secret_0.secret_1.secret_2.secret`); `ingress-nginx/secrets_test.go` covers `ingress-with-basicauth.yml` only, not the default-backend fixtures (`ingress-with-default-backend-no-rules-basic-auth.yml`).

Mutation (`verify/repro/c6_mutation.diff`, runner `verify/repro/c6_mutation_provider_suites.sh`): in `crd/kubernetes.go` `loadSecretKeys`, every reference found inside a nested map resolves to `"MUTATED"`; in `ingress-nginx/build.go` `applyDefaultBackendAuth`, all resolved default-backend auth values are replaced by `"MUTATED"`. Public references are unchanged.

```sh
cd ~/wt/8ca00183 && git apply verify/repro/c6_mutation.diff
git diff --stat
go test -count=1 ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx
git checkout -- pkg/provider/kubernetes/crd/kubernetes.go pkg/provider/kubernetes/ingress-nginx/build.go
```

```console
 pkg/provider/kubernetes/crd/kubernetes.go      | 7 +++++++
 pkg/provider/kubernetes/ingress-nginx/build.go | 3 +++
 2 files changed, 10 insertions(+)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.492s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.452s
```

Impact: both provider suites stay green while the nested CRD plugin credential and the Ingress default-backend credentials are wrong, so a regression in either resolution path (the deep-plugin case is the one real plugins with structured config hit; default-backend auth is the standard Ingress-NGINX catch-all) would ship undetected. CONFIRMED.

## C7

Target: audited branch (`~/repos/traefik` @ `5ca0225b2`). Claim: ForwardAuth TLS certificate/key loading errors retain Secret-derived filesystem paths in middleware construction errors.

Source: `pkg/types/tls.go` `loadKeyPair` lines 80-95 (`tls.X509KeyPair` → `os.Stat(cert)`/`os.Stat(key)` → `tls.LoadX509KeyPair(cert, key)`); `pkg/server/middleware/middlewares.go:224` returns `auth.NewForward(...)` errors unredacted (RedactURNRefs is applied only at lines 426/432 in the Plugin branch).

Test on `pkg/types` (`verify/repro/c7_loadkeypair_path_in_error_test.go` copied into `pkg/types/`):

```sh
cd ~/repos/traefik && go test -count=1 -run 'TestVerifyC4' -v ./pkg/types
```

```console
=== RUN   TestVerifyC4_LoadKeyPairDirectoryPaths
    verify_c4_test.go: loadKeyPair error: read /tmp/TestVerifyC4_LoadKeyPairDirectoryPaths3495941400/001/SECRET-CERT-DIR-xyz: is a directory
    verify_c4_test.go: loadKeyPair(/etc/hostname,/etc/hostname): tls: failed to find any PEM data in certificate input
--- PASS
```

Propagation into the middleware construction error (`verify/repro/c4_forwardauth_tls_path_leak_test.go` copied into `pkg/server/middleware/`):

```sh
cd ~/repos/traefik && go test -count=1 -run 'TestVerifyC4' -v ./pkg/server/middleware
```

```console
    verify_c4_test.go: constructor error: unable to create client TLS configuration: read /tmp/TestVerifyC4_ForwardAuthSecretDirPathInError3285085307/001/SECRET-CERT-DIR-xyz: is a directory
    verify_c4_test.go: runtime MiddlewareInfo.Err (what /api/http/middlewares exposes): [unable to create client TLS configuration: read /tmp/TestVerifyC4_ForwardAuthSecretDirPathInError3285085307/001/SECRET-CERT-DIR-xyz: is a directory]
    verify_c4_test.go: dynamic config still holds refs: cert="urn:k8s:secret:tlssecret:tls.crt" key="urn:k8s:secret:tlssecret:tls.key"
```

Impact: identical to C4 — the resolved Secret value survives into `runtime.MiddlewareInfo.Err` when it is a readable-path-like string; the routing configuration itself keeps the references. Only Secret values that `os.Stat` accepts reach `LoadX509KeyPair`; ordinary non-PEM garbage yields a path-free error. CONFIRMED.

## C8

Target: `evalon/traefik-se-9568168b` (`~/wt/9568168b` @ `a1742e8cc`). Claim: emitted authentication Secret references name source keys that do not exist.

Source: `pkg/provider/kubernetes/crd/kubernetes.go:1749` in `storeAuthSecret`: `key := "users"` is used for the typed (`username`/`password`) BasicAuth Secret as well as for htpasswd Secrets. `pkg/provider/kubernetes/ingress-nginx/build.go:1262` emits `:auth` for auth-file Secrets (key exists) and per-user keys for auth-map Secrets (keys exist).

Test (`verify/repro/c8_typed_basicauth_ref_key_test.go` copied into `pkg/provider/kubernetes/crd/`): loads two IngressRoutes through the provider with a fake clientset, one BasicAuth Middleware using a typed Secret (`username`/`password` keys) and one using an htpasswd Secret (`users` key), and compares emitted `urn:k8s:secret:<name>:<key>` references against `secret.Data` keys.

```sh
cd ~/wt/9568168b && go test -count=1 -run TestVerifyC8 -v ./pkg/provider/kubernetes/crd
```

```console
=== RUN   TestVerifyC8_TypedBasicAuthRefKey
    verify_c8_test.go: secret "typed" data keys=[password username] -> emitted refs=[urn:k8s:secret:typed:users]
    verify_c8_test.go: secret "htpasswd" data keys=[users] -> emitted refs=[urn:k8s:secret:htpasswd:users]
--- PASS: TestVerifyC8_TypedBasicAuthRefKey (0.00s)
```

Impact: for the typed BasicAuth Secret form (the documented CRD way to give `username`/`password`), the routing configuration advertises `urn:k8s:secret:typed:users` — a key the Secret does not contain. Anyone (operator, tooling, or a resolver that looks the key up by name) following the reference finds nothing; it misrepresents the data source in the API/dashboard/support dumps. The htpasswd form is correct. CONFIRMED.

## C9

Target: `evalon/traefik-se-cff842f1` (`~/wt/cff842f1` @ `0424766a4`). Claim: debug-logging a nil HTTP middleware entry panics before validation, and the production configuration flow reaches logging first.

Source: `pkg/server/configurationwatcher.go:233-236` `logConfiguration` does `for _, middleware := range copyConf.HTTP.Middlewares { middleware.Secrets = nil }` on the deep copy (generated `DeepCopyInto` keeps nil map entries as nil); `receiveConfigurations` calls `logConfiguration(logger, configMsg)` (line 134) before any comparison or listener publication.

Test (`verify/repro/c9_nil_middleware_logging_panic_test.go` copied into `pkg/server/`):

```sh
cd ~/wt/cff842f1 && go test -count=1 -run TestVerifyC9 -v ./pkg/server
```

```console
=== RUN   TestVerifyC9_LogConfigurationNilEntry
    c9_nil_middleware_logging_panic_test.go:30: recovered: runtime error: invalid memory address or nil pointer dereference
--- PASS: TestVerifyC9_LogConfigurationNilEntry (0.00s)
=== RUN   TestVerifyC9_LogConfigurationNilEntry_InfoLevel
    c9_nil_middleware_logging_panic_test.go:41: no panic at info level
--- PASS: TestVerifyC9_LogConfigurationNilEntry_InfoLevel (0.00s)
=== RUN   TestVerifyC9_DecodeNullMiddleware
    c9_nil_middleware_logging_panic_test.go:50: yaml: middlewares=1, nil-mid present=false, nil-mid==nil: true
    c9_nil_middleware_logging_panic_test.go:56: json: middlewares=1, nil-mid present=false, nil-mid==nil: true
    c9_nil_middleware_logging_panic_test.go:62: encoding/json (plugin provider path): middlewares=1, nil-mid present=true, nil-mid==nil: true
--- PASS: TestVerifyC9_DecodeNullMiddleware (0.00s)
=== RUN   TestVerifyC9_ReceiveConfigurationsNilEntry
    c9_nil_middleware_logging_panic_test.go:92: receiveConfigurations recovered: <nil>
    c9_nil_middleware_logging_panic_test.go:95: second recovered: runtime error: invalid memory address or nil pointer dereference
--- PASS: TestVerifyC9_ReceiveConfigurationsNilEntry (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server	0.053s
```

Reading of the four tests:

- `logConfiguration` with a debug-level logger and `HTTP.Middlewares{"nil-mid": nil}` panics with a nil-pointer dereference; at info level the function returns early and does not panic.
- `receiveConfigurations` (the real watcher goroutine body) fed a message carrying that configuration via `allProvidersConfigs`, with a debug logger in the context (as `log.Logger.WithContext` provides in the binary), panics inside `logConfiguration` before the configuration reaches comparison/`preLoadConfiguration`/listeners ("second recovered"). With no logger in the context (zerolog disabled logger) it does not panic ("receiveConfigurations recovered: <nil>"), which is why the first attempt was silent.
- Provider decoding: paerser `file.DecodeContent` (file and HTTP providers) drops a `null`/empty middleware entry for both YAML and JSON, so those providers cannot produce the nil entry. `pkg/plugins/providers.go:189` decodes provider-plugin output with `encoding/json`, which keeps `"nil-mid": null` as a present nil entry — a provider plugin emitting a null middleware is the production source.

Impact: with debug logging enabled, a single null middleware entry from a provider plugin crashes the whole configuration watcher goroutine (Traefik stops applying any further dynamic configuration, or exits), instead of the entry being rejected during middleware validation. Triggering requires a provider plugin that emits a null entry plus `log.level=DEBUG`; file/HTTP/Kubernetes providers cannot trigger it. CONFIRMED (both parts hold; the trigger is a narrow but real production path).
