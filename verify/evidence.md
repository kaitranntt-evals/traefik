# Evidence — run v-a1a20d6c

Observations only. Every command was run from `~/repos/traefik` (work branch `verify/traefik-secret-references-in-middlewares-v-a1a20d6c`, created from `origin/traefik-secret-references-in-middlewares-perfect` @ `84614aa0c`) with Go 1.26.3 on Linux. Full outputs are under `verify/logs/`.

Common setup (claim branches live in the sibling repository; one detached worktree per branch, plus the eval base commit as a control):

```sh
cd ~/repos/traefik
git remote add claims https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch claims evalon/traefik-se-32a38a91 evalon/traefik-se-59e46302 evalon/traefik-se-6205ead6
git worktree add --detach ../wt-32a38a91 claims/evalon/traefik-se-32a38a91   # b594084a44cec807410ffe947cc187e37e192ae6
git worktree add --detach ../wt-59e46302 claims/evalon/traefik-se-59e46302   # 0cd5a87e9d65b6d9ea375f5a4924be73af48746b
git worktree add --detach ../wt-6205ead6 claims/evalon/traefik-se-6205ead6   # 022a9bc95bf3468654a5ff33c1ac0b53d4e584bb
git worktree add --detach ../wt-base af6d927c51191ae1df104aaf907793386b816c28 # eval base commit (merge-base of all three)
unzip -o eval_tests.zip -d ~/eval_tests                                       # fixtures, byte-exact
```

The repro files carry a `//go:build verify` tag, so they are inert on this branch and are run by copying them into the target package of a worktree and passing `-tags verify`.

## C1

Claim: on `evalon/traefik-se-32a38a91`, credential-reference construction discards the Secret data key selected by the loader and substitutes a different key in the reference. Verdict: **CONFIRMED**.

What the branch does (diff against base, `pkg/provider/kubernetes/crd/kubernetes.go`): `loadAuthCredentials` and `loadCASecret` return only values; the references are built afterwards from constants — `authSecretReferences(...)` formats `users.%d`, and both CA call sites pass the literal `"ca.crt"` to `secretReference(...)`. The ingress-nginx provider (`build.go`, `resolveBasicAuth`) formats `urn:k8s:secret:%s:users.%d` the same way.

### Probe: loader-selected key vs reference key

`verify/repro/c1_reference_key_probe_test.go` loads seven Secrets through the real CRD provider (`loadConfigurationFromCRD` on fake clientsets) behind basicAuth, digestAuth, forwardAuth `tls.caSecret` and rateLimit `redis.tls.caSecret` middlewares. For each reference found in the routing configuration it reads the value the provider bound to that reference (`Middleware.Secrets[ref]`) and finds which `Secret.Data` key holds that value — that is the loader-selected key — then compares it with the key named in the reference.

```sh
cp verify/repro/c1_reference_key_probe_test.go ../wt-32a38a91/pkg/provider/kubernetes/crd/
(cd ../wt-32a38a91 && go test -tags verify -count=1 -v -run '^TestVerifyC1_' ./pkg/provider/kubernetes/crd)
```

Output (assertion noise removed; full log `verify/logs/c1_probe_on_32a38a91.log`):

```console
middleware           | Secret.Data keys                   | reference in routing configuration               | ref key       | loader-selected key      | same?
basic-auth-key       | auth                               | urn:k8s:secret:basic-auth-key:users.0            | users.0       | auth                     | false (ref key exists in Secret: false, bound value: "alice")
basic-auth-key       | auth                               | urn:k8s:secret:basic-auth-key:users.1            | users.1       | auth                     | false (ref key exists in Secret: false, bound value: "bob")
basic-users-key      | users                              | urn:k8s:secret:basic-users-key:users.0           | users.0       | users                    | false (ref key exists in Secret: false, bound value: "carol")
digest-htdigest-key  | htdigest                           | urn:k8s:secret:digest-htdigest-key:users.0       | users.0       | htdigest                 | false (ref key exists in Secret: false, bound value: "dave")
fwd-ca-cacrt         | ca.crt                             | urn:k8s:secret:ca-cacrt:ca.crt                   | ca.crt        | ca.crt                   | true (ref key exists in Secret: true, bound value: "PEM-FROM-ca.crt")
fwd-ca-tlsca         | tls.ca                             | urn:k8s:secret:ca-tlsca:ca.crt                   | ca.crt        | tls.ca                   | false (ref key exists in Secret: false, bound value: "PEM-FROM-tls.ca")
fwd-ca-custom        | corp-root.pem                      | urn:k8s:secret:ca-custom:ca.crt                  | ca.crt        | corp-root.pem            | false (ref key exists in Secret: false, bound value: "PEM-FROM-corp-root.pem")
fwd-ca-both          | ca.crt,tls.ca                      | urn:k8s:secret:ca-both:ca.crt                    | ca.crt        | tls.ca                   | false (ref key exists in Secret: true, bound value: "PEM-FROM-tls.ca-IN-BOTH")
redis-ca-tlsca       | tls.ca                             | urn:k8s:secret:ca-tlsca:ca.crt                   | ca.crt        | tls.ca                   | false (ref key exists in Secret: false, bound value: "PEM-FROM-tls.ca")
redis-ca-custom      | corp-root.pem                      | urn:k8s:secret:ca-custom:ca.crt                  | ca.crt        | corp-root.pem            | false (ref key exists in Secret: false, bound value: "PEM-FROM-corp-root.pem")
redis-ca-both        | ca.crt,tls.ca                      | urn:k8s:secret:ca-both:ca.crt                    | ca.crt        | tls.ca                   | false (ref key exists in Secret: true, bound value: "PEM-FROM-tls.ca-IN-BOTH")
--- FAIL: TestVerifyC1_ReferenceKeyVersusLoaderSelectedKey (0.06s)
```

Reading it against the claim's cases:

- Authentication (`loadAuthCredentials`): the loader takes the single data key (`auth`, `users`, `htdigest`); every reference names `users.N`, a key that exists in none of the Secrets — including when the real key is `users`.
- CA (`loadCASecret`), `tls.ca`: the loader selects `tls.ca`, the reference names `ca.crt`.
- CA, sole entry with a custom key (`corp-root.pem`): the reference names `ca.crt`.
- CA, Secret holding both `tls.ca` and `ca.crt` with different contents: the loader binds the `tls.ca` value, the reference names `ca.crt` — a key that exists and holds a different certificate than the one in use.
- Only the `ca.crt`-only Secret gets a matching reference.

### Control: same probe on the audited branch

```sh
cp verify/repro/c1_reference_key_probe_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -v -run '^TestVerifyC1_' ./pkg/provider/kubernetes/crd
rm pkg/provider/kubernetes/crd/c1_reference_key_probe_test.go
```

```console
basic-auth-key       | auth                               | urn:k8s:secret:basic-auth-key:auth               | auth          | auth                     | true (ref key exists in Secret: true)
basic-users-key      | users                              | urn:k8s:secret:basic-users-key:users             | users         | users                    | true (ref key exists in Secret: true)
digest-htdigest-key  | htdigest                           | urn:k8s:secret:digest-htdigest-key:htdigest      | htdigest      | htdigest                 | true (ref key exists in Secret: true)
fwd-ca-tlsca         | tls.ca                             | "PEM-FROM-tls.ca"                                | (inline)      | -                        | n/a
--- PASS: TestVerifyC1_ReferenceKeyVersusLoaderSelectedKey (0.06s)
```

(That branch keeps the real data key for auth Secrets and leaves CA certificates inline; log `verify/logs/c1_probe_control_perfect.log`.)

### Eval fixtures on the claim branch

```sh
cp ~/eval_tests/tests/{eval_crd_behavior_test.go,export_eval_test.go,eval_provider_to_builder_runtime_test.go} ../wt-32a38a91/pkg/provider/kubernetes/crd/
cp ~/eval_tests/tests/{eval_ingress_nginx_test.go,export_eval_ingress_nginx_test.go} ../wt-32a38a91/pkg/provider/kubernetes/ingress-nginx/
(cd ../wt-32a38a91 && go test -count=1 -v -run '^TestEval_' ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx)
```

Key lines (full log `verify/logs/c1_eval_fixtures_on_32a38a91.log`):

```console
--- FAIL: TestEval_CRD_BasicAndDigestAuth_SecretRetentionAndSerialization (0.08s)
        	Error:      	Not equal: 
        	            	expected: "users"
        	            	actual  : "users.0"
        	Messages:   	reference key must match actual secret key 'users', got "users.0"
        	Error:      	"{\"http\":{\"middlewares\":{\"default-basicauth\":{\"basicAuth\":{\"users\":[\"urn:k8s:secret:authsecret:users.0\",\"urn:k8s:secret:authsecret:users.1\"]}},\"default-digestauth\":{\"digestAuth\":{\"users\":[\"urn:k8s:secret:authsecret:users.0\",\"urn:k8s:secret:authsecret:users.1\"]}},\"default-forwardauth\":{\"forwardAuth\":{\"address\":\"test.com\",\"tls\":{\"ca\":\"urn:k8s:secret:casecret:ca.crt\",\"cert\":\"urn:k8s:secret:tlssecret:tls.crt\",\"key\":\"urn:k8s:secret:tlssecret:tls.key\"},\"headerField\":\"X-Header-Field\",\"maxBodySize\":-1}}}},\"tcp\":{},\"udp\":{},\"tls\":{}}" does not contain "\"urn:k8s:secret:authsecret:users\""
--- FAIL: TestEval_CRD_TypedBasicAuth_SecretRetention (0.01s)
        	Error:      	[]string{"username", "password"} does not contain "users.0"
        	Messages:   	reference key "users.0" must exist in typed basic-auth Secret
--- FAIL: TestEval_IngressNGINX_BasicAndDigestAuth_SecretRetentionAndSerialization (0.07s)
        	Messages:   	ingress auth-file reference must name key 'auth', got "users.0"
--- FAIL: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation (0.10s)
        	Messages:   	rotated configuration must retain URN references, got "urn:k8s:secret:auth-map-secret:users.0"
--- PASS: TestEval_ProviderToBuilder_BasicAuthLateBinding (0.01s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthLateBinding (0.03s)
--- PASS: TestEval_ProviderToBuilder_ForwardAuthClientCertificate (0.01s)
--- PASS: TestEval_CRD_SecretRotationAndHashStability (0.03s)
```

The JSON printed by the first failure is the repository's own `fixtures/with_auth.yml`: its `casecret` Secret has the single data key `ca`, and the serialized configuration shows `"ca":"urn:k8s:secret:casecret:ca.crt"`.

### Impact reasoning

The wrong key is visible wherever the routing configuration is shown (`/api/rawdata`, dashboard, dump, logs): an operator sees `urn:k8s:secret:<name>:users.0` or `...:ca.crt` and cannot find that key in the Secret; with a Secret holding both `tls.ca` and `ca.crt` the reference points at a real key whose certificate is not the one trusted. It happens for every basicAuth/digestAuth Secret and every `auth-secret` annotation (the key is never `users.N`), and for any CA Secret not keyed `ca.crt`. Runtime behavior is unaffected: the late-binding, client-certificate and rotation fixtures pass on this branch, because resolution looks the reference string up in the binding map rather than re-reading the Secret. The task statement asks for references "of the form `urn:k8s:secret:name:key`".

## C2

Claim: on `evalon/traefik-se-59e46302`, constructing a Redis-backed rate limiter fails for a literal inline password beginning `urn:k8s:secret:` when no provider-loaded Secret bindings exist. Verdict: **CONFIRMED** (both parts).

Naming drift: the claim points at `pkg/config/dynamic/urnrefs.go`, which does not exist on this branch (it is the audited branch's file). The equivalent here is `pkg/config/dynamic/secrets.go` (`IsSecretReference`, `Secrets.Resolve`, `Middleware.ResolveSecrets`, `resolveRedis`).

### Part 1 — Inline credential classification

`verify/repro/c2_resolve_classification_probe_test.go` calls the branch's resolution API directly, with nil and empty bindings.

```sh
cp verify/repro/c2_*.go ../wt-59e46302/pkg/server/middleware/
(cd ../wt-59e46302 && go test -tags verify -count=1 -v -run '^TestVerifyC2_' ./pkg/server/middleware)
```

```console
C2 IsSecretReference("urn:k8s:secret:this-is-my-literal-redis-password") = true
C2 Secrets.Resolve [nil bindings, urn-prefixed literal]: value="" err=unknown secret reference urn:k8s:secret:this-is-my-literal-redis-password
C2 Secrets.Resolve [empty bindings, urn-prefixed literal]: value="" err=unknown secret reference urn:k8s:secret:this-is-my-literal-redis-password
C2 Secrets.Resolve [nil bindings, ordinary literal (control)]: value="ordinary-password" err=<nil>
C2 Middleware.ResolveSecrets (Secrets == nil: true): resolved-nil=true err=rateLimit: redis password: unknown secret reference urn:k8s:secret:this-is-my-literal-redis-password
--- FAIL: TestVerifyC2_InlineCredentialClassification (0.00s)
```

The classification is by prefix alone: with no bindings at all, the literal is treated as an unresolved reference. Part verdict: CONFIRMED.

### Part 2 — Middleware construction path

`verify/repro/c2_redis_inline_password_probe_test.go` builds a `rateLimit` middleware (`Redis.Password` = the literal, `Middleware.Secrets` unset, as any non-Kubernetes provider would produce) through `Builder.buildConstructor` and through `Builder.BuildMiddlewareChain(...).Then(...)`, against a local fake Redis that records the handshake. Same command as above; output on the claim branch (log `verify/logs/c2_probe_on_59e46302.log`):

```console
C2 buildConstructor error: rateLimit: redis password: unknown secret reference urn:k8s:secret:this-is-my-literal-redis-password
C2 BuildMiddlewareChain(...).Then error: rateLimit: redis password: unknown secret reference urn:k8s:secret:this-is-my-literal-redis-password
C2 runtime MiddlewareInfo: status="disabled" err=["rateLimit: redis password: unknown secret reference urn:k8s:secret:this-is-my-literal-redis-password"]
C2 configuration password after build: "urn:k8s:secret:this-is-my-literal-redis-password"
C2 credentials received by the Redis server (client options in effect): []
--- FAIL: TestVerifyC2_RedisInlineURNPrefixedPassword (0.00s)
```

`buildConstructor` returns the resolution error before `ratelimiter.New`/`newRedisLimiter` is reached; the chain propagates it, the middleware is marked `disabled`, and no Redis client is created (the server sees no connection). Part verdict: CONFIRMED.

### Controls: same portable probe on the eval base commit and on the audited branch

```sh
cp verify/repro/c2_redis_inline_password_probe_test.go ../wt-base/pkg/server/middleware/
(cd ../wt-base && go test -tags verify -count=1 -v -run '^TestVerifyC2_RedisInlineURNPrefixedPassword$' ./pkg/server/middleware)
cp verify/repro/c2_redis_inline_password_probe_test.go pkg/server/middleware/
go test -tags verify -count=1 -v -run '^TestVerifyC2_RedisInlineURNPrefixedPassword$' ./pkg/server/middleware
rm pkg/server/middleware/c2_redis_inline_password_probe_test.go
```

Identical output on both (logs `verify/logs/c2_probe_control_base.log`, `verify/logs/c2_probe_control_perfect.log`):

```console
C2 buildConstructor error: <nil>
C2 constructor(next) [ratelimiter.New -> newRedisLimiter] error: <nil>
C2 BuildMiddlewareChain(...).Then error: <nil>
C2 runtime MiddlewareInfo: status="enabled" err=[]
C2 configuration password after build: "urn:k8s:secret:this-is-my-literal-redis-password"
C2 request through the limiter: HTTP 200
C2 credentials received by the Redis server (client options in effect): [["hello" "3" "auth" "default" "urn:k8s:secret:this-is-my-literal-redis-password"] ["auth" "urn:k8s:secret:this-is-my-literal-redis-password"]]
--- PASS: TestVerifyC2_RedisInlineURNPrefixedPassword (0.00s)
```

So the failure is a regression introduced by the claim branch: before the change (and on the audited branch) the literal reaches the Redis client options unchanged and the request is served.

### Impact reasoning

A rate limiter that worked before the change is disabled, and the router using it fails to build, when the Redis password (or username: `resolveRedis` runs both through the same `Resolve`) is a literal starting with `urn:k8s:secret:` and the middleware comes from a provider that loads no Secret (file, HTTP, labels, KV). The trigger is unusual — a password must literally start with that prefix. The only workaround observed is changing the password. `Secrets.Resolve` is also what the branch uses for basicAuth/digestAuth users, forwardAuth certificates and plugin values; those paths were not probed here.

## C3

Claim: on `evalon/traefik-se-6205ead6`, the plugin Secret-reference test's stored-diagnostic assertion accepts missing or unsanitized recorded errors. Verdict: **CONFIRMED** (both parts).

Naming drift: `TestBuilder_BuildMiddlewareChain_pluginErrorURNRefs` and the comparison with `[]string{expectedErr}` belong to the audited branch (`pkg/server/middleware/middlewares_test.go:429,477`). On the claim branch the plugin Secret-reference test is `TestBuilder_buildConstructor_pluginSecretReference` in `pkg/server/middleware/secrets_test.go`; its stored-diagnostic assertion is the last line:

```go
			_, err := builder.buildConstructor(t.Context(), "plugin")
			...
			require.EqualError(t, err, test.expectedError)
			assert.NotContains(t, rtConf.Middlewares["plugin"].Err, test.secret)
```

Call path: the test calls `buildConstructor` directly. On this branch the only `AddError` calls of the package are in `BuildMiddlewareChain` (`middlewares.go:78,84,90`), which the test never goes through.

Baseline, unmodified branch (log `verify/logs/c3_baseline_on_6205ead6.log`):

```sh
(cd ../wt-6205ead6 && go test -count=1 -v -run '^TestBuilder_buildConstructor_pluginSecretReference$' ./pkg/server/middleware)
```

```console
--- PASS: TestBuilder_buildConstructor_pluginSecretReference (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.039s
```

### Probe: call path and assertion evaluated against defective states

`verify/repro/c3_stored_diagnostic_assertion_probe_test.go` (a) replays the test's setup and call and prints the `Err` slice the assertion inspects, then the `Err` produced by the chain path; (b) evaluates the exact assertion expression, and the stricter `assert.Equal(t, []string{expectedErr}, Err)`, against each state with a recording `TestingT`.

```sh
cp verify/repro/c3_stored_diagnostic_assertion_probe_test.go ../wt-6205ead6/pkg/server/middleware/
(cd ../wt-6205ead6 && go test -tags verify -count=1 -v -run '^TestVerifyC3_' ./pkg/server/middleware)
rm ../wt-6205ead6/pkg/server/middleware/c3_stored_diagnostic_assertion_probe_test.go
```

```console
C3 call path buildConstructor: returned error = "plugin: failed to decode configuration: 'token' expected type 'int', got unconvertible type 'string', value: 'urn:k8s:secret:pluginsecret:token'"
C3 call path buildConstructor: MiddlewareInfo.Err = []string(nil) (len 0)
C3 the test's assertion on that Err: rejected=false
C3 call path BuildMiddlewareChain: MiddlewareInfo.Err = ["plugin: failed to decode configuration: 'token' expected type 'int', got unconvertible type 'string', value: 'urn:k8s:secret:pluginsecret:token'"]
--- PASS: TestVerifyC3_CallPathLeavesErrEmpty (0.00s)
C3 part 1: nil Err (nothing recorded)                   | defective=true  | assert.NotContains(Err, secret) rejects=false | assert.Equal([]string{expectedErr}, Err) rejects=true
C3 part 1: empty Err (nothing recorded)                 | defective=true  | assert.NotContains(Err, secret) rejects=false | assert.Equal([]string{expectedErr}, Err) rejects=true
C3 part 2: Err entry embedding the secret value         | defective=true  | assert.NotContains(Err, secret) rejects=false | assert.Equal([]string{expectedErr}, Err) rejects=true
C3 control: Err entry equal to the bare secret value    | defective=true  | assert.NotContains(Err, secret) rejects=true  | assert.Equal([]string{expectedErr}, Err) rejects=true
C3 control: sanitized Err entry                         | defective=false | assert.NotContains(Err, secret) rejects=false | assert.Equal([]string{expectedErr}, Err) rejects=false
--- FAIL: TestVerifyC3_AssertionAgainstDefectiveStates (0.00s)
```

(The probe test fails on purpose: it asserts that the branch's assertion rejects each defective state. Log `verify/logs/c3_probe_on_6205ead6.log`.)

- Part 1 — Recorded-error population: the assertion accepts a nil and an empty `Err`, and in the real test `Err` is always nil. Part verdict: CONFIRMED.
- Part 2 — Recorded-message sanitation: `assert.NotContains` on a `[]string` tests element equality, so an entry that embeds the Secret value in a longer message is accepted; only an entry equal to the bare value is rejected. Part verdict: CONFIRMED.

### Mutations: the branch's own test under defective production code

`verify/repro/c3_mutations.sh` adds one `t.Logf` before the assertion (instrumentation only), then applies each mutation to `pkg/server/middleware/middlewares.go`, runs the branch's test, and restores both files.

```sh
verify/repro/c3_mutations.sh ~/repos/wt-6205ead6
```

Key lines (log `verify/logs/c3_mutations_on_6205ead6.log`):

```console
== M0: unmodified production code (test instrumented with one t.Logf)
        secrets_test.go:194: C3 Err inspected by the assertion: []
    --- PASS: TestBuilder_buildConstructor_pluginSecretReference (0.00s)
== M1: buildConstructor records the plugin failure with the resolved (secret) configuration in MiddlewareInfo.Err
        secrets_test.go:194: C3 Err inspected by the assertion: ["plugin: rejected configuration map[test:map[nested:map[token:secret-token] token:secret-token]]"]
        secrets_test.go:194: C3 Err inspected by the assertion: ["plugin: rejected configuration map[test:map[nested:map[token:type] token:type]]"]
    --- PASS: TestBuilder_buildConstructor_pluginSecretReference (0.00s)
== M2: BuildMiddlewareChain no longer records any error (AddError calls removed)
        secrets_test.go:194: C3 Err inspected by the assertion: []
    --- PASS: TestBuilder_buildConstructor_pluginSecretReference (0.00s)
== M3 (sensitivity control): buildConstructor records the bare Secret value as an Err entry
        secrets_test.go:194: C3 Err inspected by the assertion: ["secret-token"]
    --- FAIL: TestBuilder_buildConstructor_pluginSecretReference (0.00s)
== restored: 0 modified tracked files left
```

- M1: production code stores the Secret value inside a recorded diagnostic; the test stays green (part 2 in the real test).
- M2: production code records nothing at all; the test stays green (part 1 in the real test).
- M3: the test does react, but only when an `Err` entry is exactly the Secret value.

The eval fixtures `TestEval_ProviderToBuilder_Plugin(BuildErrorRedaction|CachedErrorRedaction)` could not serve as a mutation oracle on this branch: they already fail on the unmodified branch for reasons they measure themselves (for example `"plugin: plugin build failed: internal error" does not contain "urn:k8s:secret:name:key"`), so their status does not change with M1/M2.

```sh
cp ~/eval_tests/tests/{eval_crd_behavior_test.go,export_eval_test.go,eval_provider_to_builder_runtime_test.go} ../wt-6205ead6/pkg/provider/kubernetes/crd/
(cd ../wt-6205ead6 && go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(BuildErrorRedaction|CachedErrorRedaction)$' ./pkg/provider/kubernetes/crd)
```

```console
--- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction (0.00s)
--- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (1.55s)
```

(Log `verify/logs/c3_eval_fixtures_on_6205ead6.log`.)

### Impact reasoning

The task requires that a rejected plugin configuration be reported "on the middleware in the API" with the reference instead of the value. On this branch the stored diagnostic produced by the chain path is currently sanitized (probe line `C3 call path BuildMiddlewareChain`), so this is a test gap, not a live leak: the branch's only check of the stored diagnostic never sees a stored diagnostic, and would not flag a leaking one unless the entry were exactly the Secret value. A later change that leaks the value into `MiddlewareInfo.Err`, or stops recording the error, would pass this test.
