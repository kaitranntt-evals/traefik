# Observational evidence — audit v-24d3d319

Target implementation: `traefik-secret-references-in-middlewares-perfect` (`84614aa0`), base `af6d927c5`. Claim branches were fetched from `kaitranntt-evals/traefik-secret-references-in-middlewares` as remote `evalon` (local refs `evalon/evalon/traefik-se-<hash>`) and checked out in separate `jj workspace`s at `~/wt/<hash>`; all claim branches share merge-base `af6d927c5` with one solution commit each. Go `go1.26.3 linux/amd64`.

Probe/mutation files live under `verify/repro/`. Every probe is copied into the package directory as `verify_probe_*_test.go`, run, and the copy is not committed (excluded via `.git/info/exclude`). Mutation scripts back up the mutated production file and restore it afterwards (`diff -q` reported `clean` after every run); no production code is changed on any branch.

Baseline sanity on the target branch — the eval fixtures from `eval_tests.zip` were copied byte-exact to the paths listed in the prompt and run:

```sh
cd ~/repos/traefik && go test -race -count=1 -timeout=10m -run '^TestEval_' ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx ./pkg/middlewares/auth ./pkg/server
```

```text
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	2.813s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	1.620s
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	1.101s
ok  	github.com/traefik/traefik/v3/pkg/server	1.085s
EXIT=0
```

Each claim branch's own Secret tests were also run green before any mutation (`go test -count=1 -v -run 'Secret|secret' ./pkg/config/dynamic ./pkg/server/middleware ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx ./pkg/middlewares/auth` → `ok` for all five packages on all eight branches `3a48b648 3a9c96a8 af980b0a 1d04f11a 8624f8f2 413d5677 c89bfec6 2839e93a`).

## C1

Claim: the added/modified tests cover fewer than two eligible middleware types with assertions for both a resolved Secret value and a configured non-secret field.

### What the tests assert (both branches)

`3a48b648` — `pkg/config/dynamic/secrets_test.go` `TestMiddlewareSecrets` configures BasicAuth, DigestAuth, ForwardAuth TLS, RateLimit Redis and a Plugin (`"nested": []any{map[string]any{"token": tokenRef}}, "literal": "prefix-" + tokenRef`) and asserts resolved values for every type plus the non-secret literal:

```go
assert.Equal(t, "private-certificate", resolved.ForwardAuth.TLS.Cert)
assert.Equal(t, "private-key", resolved.ForwardAuth.TLS.Key)
...
assert.Equal(t, "private-token", resolved.Plugin["example"]["nested"].([]any)[0].(map[string]any)["token"])
assert.Equal(t, "prefix-"+tokenRef, resolved.Plugin["example"]["literal"])
```

`pkg/server/middleware/secrets_test.go` `TestBuildForwardAuthSecretTLS` configures `ForwardAuth{Address: server.URL, TLS: {CA, Cert, Key: <secret refs>}}` against an HTTPS server with `ClientAuth: tls.RequireAnyClientCert`, asserts `assert.Len(t, r.TLS.PeerCertificates, 1)` on the server, `http.StatusNoContent` on the response and `assert.Equal(t, before, config)` (published config incl. `Address` unchanged).

`3a9c96a8` — `pkg/config/dynamic/secrets_test.go` `TestMiddlewareSecrets` configures the same five types; plugin `"nested": []any{ref, map[string]any{"token": ref, "literal": "unchanged"}}` and asserts:

```go
assert.Equal(t, "private-token", resolved.ForwardAuth.TLS.Key)
...
assert.Equal(t, []any{"private-token", map[string]any{"token": "private-token", "literal": "unchanged"}}, resolved.Plugin["test"]["nested"])
```

`pkg/server/middleware/secrets_test.go` `TestBuilderSecretForwardAuthTLS` configures `ForwardAuth{Address: server.URL, TLS: {Cert: certRef, Key: keyRef, InsecureSkipVerify: true}}` against a client-cert-requiring HTTPS server, asserts `PeerCertificates` length 1, `204` and `assert.Equal(t, keyRef, config.ForwardAuth.TLS.Key)`.

Both branches also extend `pkg/provider/kubernetes/crd/kubernetes_test.go` `TestLoadIngressRoutes` whole-object expectations with `Secrets: {...: {"user"}, ...: {"password"}}` next to the non-secret `Redis.Endpoints`/`ForwardAuth.Address` fields already in those expected objects.

### Mutation demonstration (`verify/repro/c1_resolver_mutations.sh`)

Four single-point mutations of the branch resolver (`pkg/config/dynamic/secrets.go`), each run alone and reverted:

```sh
~/notes/c1_mut.sh   # == verify/repro/c1_resolver_mutations.sh <hash> run in ~/wt/<hash>
```

Key output (`~/logs/c1_mut.log`):

```text
=== 3a48b648 BASELINE
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.017s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.027s
--- [3a48b648] mutation: M1 drop non-secret ForwardAuth.Address in resolved copy
> 		config.ForwardAuth.Address = ""; tls := config.ForwardAuth.TLS
--- FAIL: TestBuildForwardAuthSecretTLS (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a48b648/pkg/server/middleware/secrets_test.go:177
--- [3a48b648] mutation: M2 leave ForwardAuth.TLS.Key unresolved
> 		tls.Key = tls.Key
--- FAIL: TestMiddlewareSecrets (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a48b648/pkg/config/dynamic/secrets_test.go:62
--- FAIL: TestBuildForwardAuthSecretTLS (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a48b648/pkg/server/middleware/secrets_test.go:174
        	Error:      	Received unexpected error:
--- [3a48b648] mutation: M3 blank non-secret plugin strings
> 		if _, ok := s[value]; !ok { return "" }; return s.resolveString(value)
--- FAIL: TestMiddlewareSecrets (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a48b648/pkg/config/dynamic/secrets_test.go:68
--- [3a48b648] mutation: M4 leave nested plugin map values unresolved
> 			value[key] = v
--- FAIL: TestMiddlewareSecrets (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a48b648/pkg/config/dynamic/secrets_test.go:67
--- FAIL: TestBuildMiddlewareSecretErrors (0.00s)

=== 3a9c96a8 BASELINE
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.010s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.029s
--- [3a9c96a8] mutation: M1 drop non-secret ForwardAuth.Address in resolved copy
> 		config.ForwardAuth.Address = ""; config.ForwardAuth.TLS.CA = resolve(config.ForwardAuth.TLS.CA)
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a9c96a8/pkg/server/middleware/secrets_test.go:220
--- [3a9c96a8] mutation: M2 leave ForwardAuth.TLS.Key unresolved
> 		config.ForwardAuth.TLS.Key = config.ForwardAuth.TLS.Key
--- FAIL: TestMiddlewareSecrets (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a9c96a8/pkg/config/dynamic/secrets_test.go:50
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a9c96a8/pkg/server/middleware/secrets_test.go:217
        	Error:      	Received unexpected error:
--- [3a9c96a8] mutation: M3 blank non-secret plugin strings
> 		if resolved := resolve(value); resolved == value { return "" } else { return resolved }
--- FAIL: TestMiddlewareSecrets (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a9c96a8/pkg/config/dynamic/secrets_test.go:55
--- [3a9c96a8] mutation: M4 leave nested plugin map values unresolved
> 			value[key] = v
--- FAIL: TestMiddlewareSecrets (0.00s)
        	Error Trace:	/home/ubuntu/wt/3a9c96a8/pkg/config/dynamic/secrets_test.go:55
--- FAIL: TestBuilderSecretPluginIsolation (0.00s)
```

Reading: on each branch at least two middleware types (ForwardAuth and Plugin) have tests that fail both when a resolved Secret value is not resolved (M2, M4) and when a configured non-secret field is lost (M1 `Address`, M3 plugin literal). The ForwardAuth non-secret coverage is behavioural on `3a9c96a8` (dropping `Address` makes the request fail → `220` is the `204` assertion) rather than a direct field assertion, which is a naming/shape discrepancy from the claim's wording but not a coverage gap.

Verdict: **REFUTED** on `3a48b648` and **REFUTED** on `3a9c96a8`.

## C2

Claim: tests require a diagnostic to retain credential content or omit URN references after a plugin error interpolates a resolved Secret.

### Assertion text

`af980b0a` `pkg/server/middleware/middlewares_test.go` `TestBuilder_buildConstructor_pluginSecrets`, case `"secret value rejected while the reference is accepted"`: the mock returns `fmt.Errorf("invalid token %q", config["token"])` for `rejected-token` and the test requires (`require.EqualError`):

```go
expectedError:    "plugin: invalid configuration: the plugin rejects the values of the referenced secrets",
notExpectedError: "rejected-token",
```

`1d04f11a` `pkg/server/middleware/middlewares_test.go` `TestBuilder_BuildChainPluginSecrets`, case `"an error caused by the secret value itself does not reveal it"` (`fmt.Errorf("invalid token %q", config["token"])`), asserted with `assert.Equal(t, test.expectedErrors, testConfig["plugin@kubernetescrd"].Err)`:

```go
expectedErrors: []string{"plugin: invalid value read from a secret in the configuration of plugin test"},
```

`8624f8f2` `pkg/server/middleware/secrets_test.go` `TestBuilder_BuildChain_pluginErrorShowsSecretReference`, case `"decoding only fails with the value"` (`rejectValue: "this_is_the_secret"`), asserted with `assert.Equal(t, test.expectedErr, err.Error())` and `assert.Equal(t, []string{test.expectedErr}, rtConf.Middlewares["plugin"].Err)`:

```go
expectedErr:   "plugin: invalid configuration: a value read from a secret is rejected",
```

`413d5677` `pkg/server/middleware/secrets_test.go` `TestBuilder_PluginSecretReferenceError`, case `"error only with the values"`, is substring-based:

```go
expected:    "referenced secret values",
notExpected: "this_is_the_secret",
...
assert.Contains(t, err.Error(), test.expected)
assert.NotContains(t, err.Error(), test.notExpected)
assert.NotContains(t, configs["plugin"].Err[0], test.notExpected)
```

### Demonstration run (`verify/repro/c2_append_reference_mutation.sh`)

The branch's generic value-caused error string is extended with the Secret reference(s); a test that requires the reference to be absent turns red (`~/logs/c2_mut.log`):

```text
=== [af980b0a] mutation diff:
> 	return errors.New("invalid configuration: the plugin rejects the values of the referenced secrets urn:k8s:secret:creds:token")
        	Error:      	Error message not equal:
        	            	expected: "plugin: invalid configuration: the plugin rejects the values of the referenced secrets"
        	            	actual  : "plugin: invalid configuration: the plugin rejects the values of the referenced secrets urn:k8s:secret:creds:token"
--- FAIL: TestBuilder_buildConstructor_pluginSecrets (0.00s)
=== [1d04f11a] mutation diff:
> 	return fmt.Errorf("invalid value read from a secret (urn:k8s:secret:name:token) in the configuration of plugin %s", pluginType)
        	            	expected: []string{"plugin: invalid value read from a secret in the configuration of plugin test"}
        	            	actual  : []string{"plugin: invalid value read from a secret (urn:k8s:secret:name:token) in the configuration of plugin test"}
--- FAIL: TestBuilder_BuildChainPluginSecrets (0.00s)
=== [8624f8f2] mutation diff:
> 	return errors.New("invalid configuration: a value read from a secret (urn:k8s:secret:plugin:token) is rejected")
        	            	expected: "plugin: invalid configuration: a value read from a secret is rejected"
        	            	actual  : "plugin: invalid configuration: a value read from a secret (urn:k8s:secret:plugin:token) is rejected"
--- FAIL: TestBuilder_BuildChain_pluginErrorShowsSecretReference (0.00s)
=== [413d5677] mutation diff:
> 	return errors.New("failed to build the plugin with the referenced secret values urn:k8s:secret:name:key, urn:k8s:secret:name:token")
--- PASS: TestBuilder_PluginSecretReferenceError (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.084s
```

Reading: on `af980b0a`, `1d04f11a` and `8624f8f2` the exact-match assertion pins a diagnostic with no `urn:k8s:secret:` reference for the case where the plugin error interpolates the resolved Secret — the test *requires* omitting the reference, contrary to the task requirement that errors expose references. On `413d5677` the assertions only require the generic phrase and the absence of the value, so adding references keeps the test green; nothing requires retention of credential content or omission of references in the plugin path (`TestMalformedUserEntryError` in `pkg/middlewares/auth/auth_test.go` expects a username in an auth parse error, but from literal `Users`, not a resolved Secret, and not a plugin error — outside this claim).

Impact (confirmed branches): operators triaging a disabled plugin middleware see only "a value read from a secret is rejected"-style text and cannot tell which of several Secret keys is wrong; the tests cement that regression.

Verdict: **CONFIRMED** on `af980b0a`, `1d04f11a`, `8624f8f2`; **REFUTED** on `413d5677`.

## C3

Claim: Secret-change tests fail to assert configuration-hash changes or middleware rebuilding.

### Assertion text

`c89bfec6` `pkg/provider/kubernetes/crd/kubernetes_test.go` `TestLoadConfigurationFromCRD_secretValuesAreHidden` and `pkg/provider/kubernetes/ingress-nginx/kubernetes_test.go` `TestLoadConfiguration_authSecretValuesAreHidden`: after rotating the Secret the only change assertions are

```go
assert.Equal(t, conf, loadConfiguration(t, test.paths, nil))
...
assert.NotEqual(t, conf, updated)
```

`2839e93a` `pkg/provider/kubernetes/crd/kubernetes_test.go` `TestLoadConfigurationFromCRD_SecretValues`:

```go
// An unchanged Secret must not produce a different configuration.
assert.Equal(t, conf, p.loadConfigurationFromCRD(t.Context(), client))
// An updated Secret must produce a different configuration, keeping the same references.
...
assert.NotEqual(t, conf, updatedConf)
```

`git diff af6d927c5 evalon/evalon/traefik-se-<hash> -- '*_test.go' | grep '^+' | grep -i 'hash\|rebuild\|watcher'` on both branches matches only `user:hash`-style fixture strings; no added test calls `hashstructure.Hash`, drives `ConfigurationWatcher`, or rebuilds a middleware across a Secret change.

### Why that matters: the CRD provider gates reloads on `hashstructure.Hash`

`pkg/provider/kubernetes/crd/kubernetes.go` (unchanged on both branches):

```go
confHash, err := hashstructure.Hash(conf, nil)
...
case p.lastConfiguration.Get() == confHash:   // event skipped
```

### Mutation demonstration (`verify/repro/c3_hash_ignore_mutation.sh` + `verify/repro/c3_secret_rotation_hash_probe_test.go`)

Adding `hash:"ignore"` to `dynamic.Middleware.Secrets` keeps `reflect.DeepEqual` inequality (so the branch tests stay green) but removes the Secret from the provider's change-detection hash (`~/logs/c3_mut.log`):

```text
=== c89bfec6 BASELINE
--- PASS: TestLoadConfigurationFromCRD_secretValuesAreHidden (0.00s)
    verify_probe_c3_test.go:65: reflect.DeepEqual(before, after) = false
    verify_probe_c3_test.go:66: hashstructure.Hash before=14830162963999246289 after=9841003227046155029 changed=true
--- PASS: TestVerifyC3_SecretRotationHash (0.00s)
=== c89bfec6 MUTATED (diff):
> 	Secrets Secrets `json:"-" toml:"-" yaml:"-" label:"-" file:"-" kv:"-" hash:"ignore"`
--- PASS: TestLoadConfigurationFromCRD_secretValuesAreHidden (0.00s)
    verify_probe_c3_test.go:65: reflect.DeepEqual(before, after) = false
    verify_probe_c3_test.go:66: hashstructure.Hash before=18229885003307325636 after=18229885003307325636 changed=false
    verify_probe_c3_test.go:68: Secret rotation did NOT change the provider change-detection hash: Provider.Provide would skip the reload
--- FAIL: TestVerifyC3_SecretRotationHash (0.00s)
=== c89bfec6 restored: clean
=== 2839e93a BASELINE
--- PASS: TestLoadConfigurationFromCRD_SecretValues (0.06s)
    verify_probe_c3_test.go:66: hashstructure.Hash before=14830162963999246289 after=9841003227046155029 changed=true
=== 2839e93a MUTATED (diff):
> 	Secrets Secrets `json:"-" toml:"-" yaml:"-" label:"-" file:"-" kv:"-" hash:"ignore"`
--- PASS: TestLoadConfigurationFromCRD_SecretValues (0.07s)
    verify_probe_c3_test.go:66: hashstructure.Hash before=18229885003307325636 after=18229885003307325636 changed=false
--- FAIL: TestVerifyC3_SecretRotationHash (0.01s)
=== 2839e93a restored: clean
```

The same `hash:"ignore"` mutation run against the `TestLoadConfiguration_authSecretValuesAreHidden` ingress-nginx test on `c89bfec6` (included in the `-run` pattern) also stayed green (`ok ... ingress-nginx`).

Impact: a change that silently makes Secret rotation invisible to the CRD provider's reload gate — i.e. rotated credentials never reach the running BasicAuth/DigestAuth/ForwardAuth/RateLimit/plugin middleware until an unrelated CRD change happens — passes both branches' Secret-change test suites unchanged.

Verdict: **CONFIRMED** on `c89bfec6` and **CONFIRMED** on `2839e93a`.

## C4

Naming drift: the claim's `pkg/config/dynamic/urnrefs.go: RedactURNRefs` / `TestMiddleware_RedactURNRefs` do not exist on this branch; the nearest equivalent is `pkg/config/dynamic/secrets.go: (*Middleware).RedactSecrets`, exercised by `TestBuilder_buildConstructor_redactsPluginSecretValues`.

Claim (branch `0d33c7f2`): alternate single-quoted/unquoted plugin error redaction leaks a resolved Secret into stored diagnostics.

Production path: `pkg/server/middleware/middlewares.go` `buildConstructor` → `midInf.RedactSecrets(err)` → `BuildMiddlewareChain` → `b.configs[name].AddError(err, true)`. `pkg/config/dynamic/secrets.go` `RedactSecrets` builds a `strings.NewReplacer` with only `strconv.Quote(value)` and `"`+value+`"` as the old strings.

Probe `verify/repro/c4_alt_quote_redaction_test.go` (mock plugin builder returns `fmt.Errorf(format, config["token"])` for four formats):

```sh
cd ~/wt/0d33c7f2 && cp ~/repos/traefik/verify/repro/c4_alt_quote_redaction_test.go pkg/server/middleware/verify_probe_c4_test.go && go test -tags verify -count=1 -run '^TestVerifyC4_' -v ./pkg/server/middleware
```

```text
=== RUN   TestVerifyC4_PluginErrorQuoteStyles/double_quoted_%q
    verify_probe_c4_test.go:57: runtime error : plugin: invalid token "urn:k8s:secret:plugin:token"
    verify_probe_c4_test.go:58: stored API err: plugin: invalid token "urn:k8s:secret:plugin:token"
=== RUN   TestVerifyC4_PluginErrorQuoteStyles/single_quoted
    verify_probe_c4_test.go:57: runtime error : plugin: invalid token 's3cr3t-value-XYZ'
    verify_probe_c4_test.go:58: stored API err: plugin: invalid token 's3cr3t-value-XYZ'
    verify_probe_c4_test.go:59: status        : disabled
    verify_probe_c4_test.go:61: LEAK: stored diagnostic contains resolved Secret value "s3cr3t-value-XYZ"
    verify_probe_c4_test.go:64: stored diagnostic does not mention the reference "urn:k8s:secret:plugin:token"
=== RUN   TestVerifyC4_PluginErrorQuoteStyles/unquoted
    verify_probe_c4_test.go:58: stored API err: plugin: invalid token s3cr3t-value-XYZ
    verify_probe_c4_test.go:61: LEAK: stored diagnostic contains resolved Secret value "s3cr3t-value-XYZ"
=== RUN   TestVerifyC4_PluginErrorQuoteStyles/backtick_quoted
    verify_probe_c4_test.go:58: stored API err: plugin: invalid token `s3cr3t-value-XYZ`
    verify_probe_c4_test.go:61: LEAK: stored diagnostic contains resolved Secret value "s3cr3t-value-XYZ"
--- FAIL: TestVerifyC4_PluginErrorQuoteStyles (0.00s)
    --- PASS: TestVerifyC4_PluginErrorQuoteStyles/double_quoted_%q (0.00s)
    --- FAIL: TestVerifyC4_PluginErrorQuoteStyles/single_quoted (0.00s)
    --- FAIL: TestVerifyC4_PluginErrorQuoteStyles/unquoted (0.00s)
    --- FAIL: TestVerifyC4_PluginErrorQuoteStyles/backtick_quoted (0.00s)
```

Impact: `runtime.MiddlewareInfo.Err` is what the API (`/api/http/middlewares/...`) and dashboard display and what the log configuration path logs. Yaegi/mapstructure decoders and plugin authors commonly format with `'%s'`/`%v` (the branch's own `TestBuilder_PluginSecretReferenceError` on other branches uses exactly `value: '%s'`), so any plugin rejecting a Secret-backed value with such a message publishes the plaintext credential to every API/dashboard reader.

Verdict: **CONFIRMED**.

## C5

Naming drift: no `urnrefs.go`/`RedactURNRefs` on this branch; the redaction path is `pkg/server/middleware/secrets.go: redactError` + `pkg/config/dynamic/secrets.go: Secrets.Contains`.

Claim (branch `120cc25c`): RateLimit Redis CA-parse diagnostics change solely due to a password matching fixed error text.

Production path: `pkg/server/middleware/middlewares.go` → `ratelimiter.New(ctx, next, rateLimit, middlewareName)` → `redactError(err, config.Secrets, refs)` (`pkg/server/middleware/secrets.go`), which replaces the whole error with `hiddenError(refs)` whenever `secrets.Contains(err.Error(), refs...)` — a plain `strings.Contains` of each non-empty Secret value.

Probe `verify/repro/c5_redis_ca_password_collision_test.go` (Redis `TLS.CA: "not-a-pem-certificate"`, password Secret varied):

```sh
cd ~/wt/120cc25c && cp ~/repos/traefik/verify/repro/c5_redis_ca_password_collision_test.go pkg/server/middleware/verify_probe_c5_test.go && go test -tags verify -count=1 -run '^TestVerifyC5_' -v ./pkg/server/middleware
```

```text
    verify_probe_c5_test.go:46: password="qxUnrelatedPassword"                         stored ="creating redis limiter: creating TLS config: failed to parse CA"
    verify_probe_c5_test.go:46: password="CA"                                          stored ="invalid configuration using urn:k8s:secret:redis:password: the error is hidden as it could reveal a secret value"
    verify_probe_c5_test.go:46: password="config"                                      stored ="invalid configuration using urn:k8s:secret:redis:password: the error is hidden as it could reveal a secret value"
    verify_probe_c5_test.go:46: password="creating TLS config: failed to parse CA"     stored ="invalid configuration using urn:k8s:secret:redis:password: the error is hidden as it could reveal a secret value"
    verify_probe_c5_test.go:52: stored diagnostic changed solely because the password is "CA":
          baseline: creating redis limiter: creating TLS config: failed to parse CA
          got     : invalid configuration using urn:k8s:secret:redis:password: the error is hidden as it could reveal a secret value
--- FAIL: TestVerifyC5_RedisCAParseErrorDependsOnPassword (0.00s)
```

Impact: the same misconfiguration (`failed to parse CA`, which never includes the password) is reported differently depending on the password's bytes. Short or dictionary passwords (`CA`, `config`, `TLS`, `parse`) hide the actionable cause and misattribute the failure to the password reference; the redaction is also an oracle — an observer of the API diagnostic learns whether the password is a substring of a fixed, known string.

Verdict: **CONFIRMED**.

## C6

Naming drift: no `urnrefs.go`/`ResolveURNRefs`/`findPluginConfig` on this branch; the resolver is `pkg/server/middleware/secrets.go: resolvePluginConfig`/`resolvePluginValue`, called from `buildPlugin`.

Claim (branch `aad38ae0`): equal-value plugin Secret bindings share mutable containers with published configuration and production construction passes them without an intervening deep copy.

Production path: `pkg/server/middleware/secrets.go` `buildPlugin` → `resolvePluginConfig` collects `references` only when `resolved != v`; `if len(references) == 0 { return pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName) }` — the published `rawPluginConfig` itself, no `copyPluginConfig`.

Probe `verify/repro/c6_plugin_equal_value_binding_test.go` (mock plugin builder records the map it receives and mutates `token` and `nested.token`):

```sh
cd ~/wt/aad38ae0 && cp ~/repos/traefik/verify/repro/c6_plugin_equal_value_binding_test.go pkg/server/middleware/verify_probe_c6_test.go && go test -tags verify -count=1 -run '^TestVerifyC6_' -v ./pkg/server/middleware
```

```text
=== RUN   TestVerifyC6_PluginEqualValueBindingSharesPublishedConfig/secret_value_differs_from_reference
    verify_probe_c6_test.go:56: same top-level map passed to plugin builder: false
    verify_probe_c6_test.go:57: published after build: map[nested:map[token:urn:k8s:secret:plugin:token] token:urn:k8s:secret:plugin:token]
=== RUN   TestVerifyC6_PluginEqualValueBindingSharesPublishedConfig/secret_value_equals_reference
    verify_probe_c6_test.go:56: same top-level map passed to plugin builder: true
    verify_probe_c6_test.go:57: published after build: map[nested:map[token:MUTATED-NESTED] token:MUTATED-BY-PLUGIN]
    verify_probe_c6_test.go:60: plugin builder received the published configuration map itself (no deep copy)
    verify_probe_c6_test.go:63: published top-level token mutated: MUTATED-BY-PLUGIN
    verify_probe_c6_test.go:66: published nested token mutated: MUTATED-NESTED
--- FAIL: TestVerifyC6_PluginEqualValueBindingSharesPublishedConfig (0.00s)
    --- PASS: TestVerifyC6_PluginEqualValueBindingSharesPublishedConfig/secret_value_differs_from_reference (0.00s)
    --- FAIL: TestVerifyC6_PluginEqualValueBindingSharesPublishedConfig/secret_value_equals_reference (0.00s)
```

Impact: when a bound Secret's value equals its own URN string (equal-value binding), the plugin constructor runs directly on the map stored in `runtime.MiddlewareInfo.Middleware.Plugin` — the object published through the API, compared by the configuration watcher, and shared by concurrent chain builds — so an in-place normalising plugin corrupts the published configuration and races other builders; the branch's `TestBuilder_pluginSecretReferences` only covers the differing-value path.

Verdict: **CONFIRMED**.
