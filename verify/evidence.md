Observations for audit run `v-6fba24be` (task `traefik-secret-references-in-middlewares`). Go `go1.26.3 linux/amd64`.

Setup shared by all sections (each section repeats what it needs):

```sh
cd ~/repos/traefik
git remote add ev https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
for b in 9c319c44 177cb630 e230a658 cc522816 7af818be 12ccb497 f176eba1 4000ae16; do
  git fetch ev evalon/traefik-se-$b && git worktree add --detach ~/repos/wt-$b FETCH_HEAD
done
unzip eval_tests.zip -d ~/fx      # fixtures, byte-exact, in ~/fx/tests
```

Commits audited: 9c319c44=`32be40e95`, 177cb630=`770d3ab38`, e230a658=`a2f4634a4`, cc522816=`9968fba15`, 7af818be=`27125adc5`, 12ccb497=`851a3cb3e`, f176eba1=`8edc40051`, 4000ae16=`b2de3a471`.
Full outputs of every run are in `verify/logs/`. Probes are build-tagged `verify`, are copied into the target package only for the run, and were removed afterwards; no production file was left modified (`git status --short` clean in every worktree after the runs, apart from installed fixtures).

## C1

Branch `evalon/traefik-se-9c319c44`. The provider keeps plugin Secret references in a custom carrier, `dynamic.Secret{Reference, Value}` (`pkg/provider/kubernetes/crd/kubernetes.go`: `return dynamic.Secret{Reference: iv, Value: value}, nil`), whose `MarshalJSON`/`MarshalText`/`MarshalYAML`/`String`/`GoString` all return the reference.

Byte-exact fixture (fails on the carrier's Go type only):

```sh
verify/repro/install_fixtures.sh ~/fx/tests ~/repos/wt-9c319c44
cd ~/repos/wt-9c319c44 && go test -count=1 -v -run '^TestEval_CRD_Plugin_SecretRetentionAndSerialization$' ./pkg/provider/kubernetes/crd
```

```console
        	Error Trace:	/home/ubuntu/repos/wt-9c319c44/pkg/provider/kubernetes/crd/eval_crd_behavior_test.go:137
        	Error:      	Should be true
--- FAIL: TestEval_CRD_Plugin_SecretRetentionAndSerialization (0.05s)
```

Line 136-137 of the fixture is `secretVal, ok := pluginConf["secret"].(string); require.True(t, ok)`: a Go type assertion to `string`, which a carrier type cannot satisfy. The claim's refute rule says retention holds "regardless of their Go carrier type", so the behavior was probed directly.

Direct probe (`verify/repro/c1_plugin_retention_probe_test.go`): loads four plugin fixtures (scalar, deep map, list, list of maps) through the production CRD loader, then checks that every original URN is still represented and that no resolved value appears in JSON, YAML, TOML, `%v`, `%+v`, `%#v`, a `DeepCopy`, and both redactor outputs.

```sh
cp verify/repro/c1_plugin_retention_probe_test.go ~/repos/wt-9c319c44/pkg/provider/kubernetes/crd/
cd ~/repos/wt-9c319c44 && go test -tags verify -count=1 -v -run '^TestVerify_C1' ./pkg/provider/kubernetes/crd
```

```console
    c1_plugin_retention_probe_test.go:34: retained Go value (%#v): map[string]dynamic.PluginConf{"test-secret":dynamic.PluginConf{"secret":urn:k8s:secret:name:key, "user":"admin"}}
    c1_plugin_retention_probe_test.go:63: redactor.RemoveCredentials: {"http":{"middlewares":{"default-test-secret":{"plugin":{"test-secret":{"secret":{},"user":{}}}}}},"tcp":{},"udp":{},"tls":{}}
    c1_plugin_retention_probe_test.go:72: json plugin: {"test-secret":{"secret":"urn:k8s:secret:name:key","user":"admin"}}
    c1_plugin_retention_probe_test.go:72: json plugin: {"test-secret":{"secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}}
    c1_plugin_retention_probe_test.go:72: json plugin: {"test-secret":{"users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}}
--- PASS: TestVerify_C1_PluginRetentionAndSerialization (0.07s)
    --- PASS: TestVerify_C1_PluginRetentionAndSerialization/with_plugin_read_secret.yml (0.06s)
    --- PASS: TestVerify_C1_PluginRetentionAndSerialization/with_plugin_deep_read_secret.yml (0.00s)
    --- PASS: TestVerify_C1_PluginRetentionAndSerialization/with_plugin_read_array_of_secret.yml (0.00s)
    --- PASS: TestVerify_C1_PluginRetentionAndSerialization/with_plugin_read_array_of_map_contain_secret.yml (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.096s
```

- *Reference retention:* every original URN is still represented (as a `dynamic.Secret` carrier) in all four shapes.
- *Serialization safety:* no serialization printed a resolved value (`this_is_the_secret` etc.); JSON/YAML/TOML/fmt print the URN. The redactor blanks every plugin value, literals included (`"user":{}`), so it shows no reference but also no value.

Logs: `verify/logs/c1_fixture_byte_exact.txt`, `verify/logs/c1_probe.txt`.

## C2

Branch `evalon/traefik-se-9c319c44`. Fixture tests exercised: scalar, typed, nested (deep map, list, list of maps), escaped key, WASM, input-mutation isolation, retained-`CreateConfig` isolation; they build real Yaegi/WASM plugins through the production `plugins.Builder` and `middleware.Builder`.

```sh
verify/repro/install_fixtures.sh ~/fx/tests ~/repos/wt-9c319c44
cd ~/repos/wt-9c319c44
RX='^(TestEval_ProviderToBuilder_Plugin(SuccessExecution|TypedSecretField|NestedExecution|EscapedKeyExecution|InputMutationIsolation|RetainedCreateConfigIsolation|WasmExecution))$'
go test -count=1 -v -run "$RX" ./pkg/provider/kubernetes/crd
```

Byte-exact result: 3 pass, the others fail only where the fixture compares a retained value with a Go `string` or walks the configuration with a helper that only recognises `string`:

```console
--- PASS: TestEval_ProviderToBuilder_PluginWasmExecution (0.14s)
        	Error Trace:	.../eval_provider_to_builder_runtime_test.go:1179
        	            	expected: "urn:k8s:secret:name:key"
        	            	actual  : ""
--- FAIL: TestEval_ProviderToBuilder_PluginSuccessExecution (0.01s)
        	Error Trace:	.../eval_provider_to_builder_runtime_test.go:2044
        	            	expected: string("urn:k8s:secret:name:key")
        	            	actual  : dynamic.Secret(urn:k8s:secret:name:key)
--- FAIL: TestEval_ProviderToBuilder_PluginTypedSecretField (0.04s)
--- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution (0.04s)
        	Error Trace:	.../eval_provider_to_builder_runtime_test.go:1287
        	Error:      	Should NOT be empty, but was []
--- PASS: TestEval_ProviderToBuilder_PluginRetainedCreateConfigIsolation (0.07s)
--- FAIL: TestEval_ProviderToBuilder_PluginInputMutationIsolation (0.04s)
```

`actual: dynamic.Secret(urn:k8s:secret:name:key)` shows the reference is retained; only its Go type differs. Line 1287 is the fixture's own precondition (`evalURNReferences` found no `string` URN), not an isolation failure.

Carrier-neutral rerun: `verify/repro/c2_carrier_tolerant_fixture.py` rewrites the installed fixture so that its two helpers and its direct equality assertions read a `dynamic.Secret` as the URN it carries (`verifyNorm`). Plugin execution, value checks, mutation scenarios and JSON comparisons are untouched.

```sh
python3 verify/repro/c2_carrier_tolerant_fixture.py ~/repos/wt-9c319c44
go test -count=1 -v -run "$RX" ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_ProviderToBuilder_PluginWasmExecution (0.14s)
--- PASS: TestEval_ProviderToBuilder_PluginSuccessExecution (0.01s)
--- PASS: TestEval_ProviderToBuilder_PluginTypedSecretField (0.03s)
--- PASS: TestEval_ProviderToBuilder_PluginRetainedCreateConfigIsolation (0.06s)
--- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution (0.06s)
--- PASS: TestEval_ProviderToBuilder_PluginNestedExecution (0.06s)
    --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution/deep_map (0.00s)
    --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution/list_of_maps (0.00s)
    --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution/list (0.01s)
--- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation (0.06s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/deep_map/success (0.01s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/list/success (0.02s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/list_of_maps/success (0.02s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/payload_equals_reference/success (0.01s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.253s
```

(all 12 `InputMutationIsolation` subtests and all 5 `RetainedCreateConfigIsolation` subtests pass; full list in the log.)

- *Reference retention:* retained after plugin construction and execution (carrier type `dynamic.Secret`).
- *Resolved execution:* scalar, typed, nested, escaped-key and WASM plugins received and used the resolved values.
- *Configuration isolation:* plugin mutations of the supplied configuration and of retained `CreateConfig` objects left the retained configuration and other instances unchanged.

Logs: `verify/logs/c2_fixture_byte_exact.txt`, `verify/logs/c2_fixture_carrier_neutral.txt`.

## C3

A claim about what tests assert: assertion text plus demonstration runs (the branch's own tests, then the same tests with production code mutated by `verify/repro/c3_mutations.sh`, which restores each file).

```sh
verify/repro/c3_mutations.sh ~/repos/wt-177cb630 ~/repos/wt-e230a658
```

### Branch evalon/traefik-se-177cb630

Assertions in `pkg/server/middleware/secrets_test.go` (added by the branch):

```go
// TestResolveSecretUserEntries — shared auth resolver
users := dynamic.Users{"inline:hash", reference}
secrets := map[string]dynamic.Secret{reference: {"alice:hash\n", "bob:hash"}}
resolved := resolveSecretUsers(users, secrets)
assert.Equal(t, dynamic.Users{"inline:hash", "alice:hash\n", "bob:hash"}, resolved)

// TestBuilderSecretForwardAuthTLS — production builder, real TLS auth server requiring a client certificate
ForwardAuth: &dynamic.ForwardAuth{
    Address: authServer.URL,
    TLS:     &dynamic.ClientTLS{Cert: certReference, Key: keyReference, InsecureSkipVerify: true},
handler.ServeHTTP(rw, httptest.NewRequest(http.MethodGet, "https://example.com", nil))
assert.Equal(t, http.StatusNoContent, rw.Code)
```

Production use of the shared resolver, `pkg/server/middleware/secrets.go`:

```go
24:		resolved.BasicAuth.Users = resolveSecretUsers(config.BasicAuth.Users, config.Secrets)
27:		resolved.DigestAuth.Users = resolveSecretUsers(config.DigestAuth.Users, config.Secrets)
```

and `pkg/server/middleware/middlewares.go:108: resolved := resolveSecrets(config.Middleware)`.

```console
##### 177cb630 baseline
--- PASS: TestBuilderSecretAuthentication (0.00s)
--- PASS: TestResolveSecretsCopy (0.00s)
--- PASS: TestResolveSecretUserEntries (0.00s)
--- PASS: TestBuilderSecretForwardAuthTLS (0.00s)
--- PASS: TestBuilderSecretAuthErrors (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.027s
=== [177 M1 resolveSecretUsers drops literal (unbound) siblings]
--- FAIL: TestResolveSecretUserEntries (0.00s)
=== [177 M2 resolveSecretUsers leaves references unresolved]
--- FAIL: TestBuilderSecretAuthentication (0.00s)
--- FAIL: TestResolveSecretUserEntries (0.00s)
--- FAIL: TestBuilderSecretAuthErrors (0.00s)
=== [177 M3 resolveSecrets blanks the non-secret ForwardAuth.Address]
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
=== [177 M4 nested ForwardAuth.TLS secrets left unresolved]
--- FAIL: TestResolveSecretsCopy (0.00s)
--- FAIL: TestBuilderSecretForwardAuthTLS (0.00s)
```

Observed: (a) the shared resolver that production uses for basicAuth and digestAuth has one assertion holding both resolved Secret entries and the content-bearing literal sibling `inline:hash`; M1 and M2 each fail it. (b) forwardAuth: the 204 is only produced when the auth endpoint answered 2xx over a handshake that required the resolved nested `TLS.Cert`/`TLS.Key`; blanking the non-secret `Address` (M3) or leaving the nested TLS references unresolved (M4) fails the test, which shows the endpoint executed in the passing run. So at least two eligible types (basicAuth/digestAuth through the shared helper, forwardAuth) have qualifying assertions, one of them nested.

### Branch evalon/traefik-se-e230a658

Assertions in `pkg/config/dynamic/secrets_test.go` (added by the branch):

```go
// TestSecretValues_ResolveUsers
secrets := SecretValues{
    "urn:k8s:secret:auth:users":       "user1:hash1\nuser2:hash2",
    "user3:urn:k8s:secret:auth:user3": "user3:hash3",
}
users := Users{"urn:k8s:secret:auth:users", "inline:hash", "user3:urn:k8s:secret:auth:user3"}
resolved := secrets.ResolveUsers(users)
assert.Equal(t, Users{"user1:hash1", "user2:hash2", "inline:hash", "user3:hash3"}, resolved)

// TestSecretValues_ResolvePluginConf
conf := map[string]any{
    "token": "urn:k8s:secret:name:key1",
    "plain": "urn:k8s:secret:name:unknown",
    "count": float64(42),
    "deep":  map[string]any{"list": []any{"urn:k8s:secret:name:key2", true}},
}
assert.Equal(t, map[string]any{
    "token": "value1",
    "plain": "urn:k8s:secret:name:unknown",
    "count": float64(42),
    "deep":  map[string]any{"list": []any{"value2", true}},
}, resolved)
```

Production callers: `pkg/server/middleware/secrets.go:12` (`resolveBasicAuth`) and `:17` (`resolveDigestAuth`) call `secrets.ResolveUsers(config.Users)`; `pkg/server/middleware/plugins.go:47` calls `secrets.ResolvePluginConf(rawPluginConfig)`.

```console
##### e230a658 baseline
--- PASS: TestSecretValues_ResolveUsers (0.00s)
--- PASS: TestSecretValues_ResolvePluginConf (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.009s
--- PASS: TestBuilder_BasicAuthSecretReference (0.00s)
--- PASS: TestBuilder_PluginSecretReference (0.00s)
--- PASS: TestBuilder_AuthErrorDoesNotRevealSecretValues (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
=== [e23 M1 ResolveUsers drops literal (unbound) siblings]
--- FAIL: TestSecretValues_ResolveUsers (0.00s)
=== [e23 M2 ResolveUsers leaves references unresolved]
--- FAIL: TestSecretValues_ResolveUsers (0.00s)
--- FAIL: TestBuilder_BasicAuthSecretReference (0.00s)
--- FAIL: TestBuilder_AuthErrorDoesNotRevealSecretValues (0.00s)
=== [e23 M3 ResolvePluginConf drops non-string siblings]
--- FAIL: TestSecretValues_ResolvePluginConf (0.00s)
=== [e23 M4 ResolvePluginConf does not resolve inside nested lists]
--- FAIL: TestSecretValues_ResolvePluginConf (0.00s)
--- FAIL: TestBuilder_PluginSecretReference (0.00s)
```

Observed: the Users-list test asserts a resolved Secret-backed entry and the literal sibling `inline:hash` in the output of the resolver production uses for basicAuth and digestAuth (M1, M2 fail it); the plugin test asserts resolved values with non-secret fields (`count`, unbound `plain`) and a nested Secret at `deep.list[0]` (M3, M4 fail it).

Log: `verify/logs/c3_mutations.txt`.

## C4

Branch `evalon/traefik-se-cc522816`. A claim about what tests assert. `pkg/server/middleware/secrets_test.go` (added by the branch), `TestBuilder_PluginSecretReference`:

```go
"nested": map[string]any{"list": []any{"urn:k8s:secret:creds:other", "urn:k8s:secret:unknown:key"}},
...
Secrets: dynamic.Secrets{
    "urn:k8s:secret:creds:token": "s3cr3t-token",
    "urn:k8s:secret:creds:other": "s3cr3t-other",
},
...
_, info, err := buildMiddleware(t, config, pluginBuilder)
assert.Equal(t, map[string]any{
    "token":  "s3cr3t-token",
    "plain":  "altered",
    "nested": map[string]any{"list": []any{"s3cr3t-other", "urn:k8s:secret:unknown:key"}},
}, pluginBuilder.configs[0])
```

`urn:k8s:secret:unknown:key` has no binding in `Secrets`, so it is a URN-shaped unbound literal sibling; `buildMiddleware` calls the production `NewBuilder(...).BuildMiddlewareChain(...)`.

```sh
verify/repro/c4_mutations.sh ~/repos/wt-cc522816
```

```console
=== baseline
--- PASS: TestBuilder_PluginSecretReference (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
=== M1: references inside lists are not resolved
        	            	expected: map[string]interface {}{"nested":map[string]interface {}{"list":[]interface {}{"s3cr3t-other", "urn:k8s:secret:unknown:key"}}, "plain":"altered", "token":"s3cr3t-token"}
        	            	actual  : map[string]interface {}{"nested":map[string]interface {}{"list":[]interface {}{"urn:k8s:secret:creds:other", "urn:k8s:secret:unknown:key"}}, "plain":"altered", "token":"s3cr3t-token"}
--- FAIL: TestBuilder_PluginSecretReference (0.00s)
=== M2: unbound (literal) list siblings are dropped to empty
        	            	expected: map[string]interface {}{"nested":map[string]interface {}{"list":[]interface {}{"s3cr3t-other", "urn:k8s:secret:unknown:key"}}, "plain":"altered", "token":"s3cr3t-token"}
        	            	actual  : map[string]interface {}{"nested":map[string]interface {}{"list":[]interface {}{"s3cr3t-other", ""}}, "plain":"altered", "token":"s3cr3t-token"}
--- FAIL: TestBuilder_PluginSecretReference (0.00s)
```

Observed: the test asserts a resolved list reference next to a preserved content-bearing unbound sibling through the production builder, and fails when production stops doing either. No special-character map-key assertion was looked for further, since the claim needs only one of the two.

Log: `verify/logs/c4_mutations.txt`.

## C5

Branch `evalon/traefik-se-7af818be`. A claim about what tests demonstrate. Branch tests that rotate a Secret:

```go
// pkg/provider/kubernetes/crd/secrets_test.go — TestLoadConfigurationFromCRD_secretUpdate
updated := loadConfigurationWithSecrets(t, paths, func(secret *corev1.Secret) {
    secret.Data["key"] = []byte("rotated")
})
assert.NotEqual(t, initial, updated)
assert.Equal(t, initial.HTTP.Middlewares["default-test-secret"].Plugin, updated.HTTP.Middlewares["default-test-secret"].Plugin)
value, ok := dynamic.Secrets.Lookup("default-test-secret@"+ProviderName, "urn:k8s:secret:name:key")
assert.Equal(t, "rotated", value)
```

`loadConfigurationWithSecrets` runs the production `Provider.loadConfigurationFromCRD`. The same shape exists for ingress-nginx (`TestLoadConfiguration_authSecretUpdate`) and for the digest itself (`TestSecretStore_Digest`). Production reload decisions: `pkg/provider/kubernetes/crd/kubernetes.go:159` (`hashstructure.Hash(conf, nil)` compared with the last hash) and `pkg/server/configurationwatcher.go:136` (`reflect.DeepEqual` on the provider's configuration). `dynamic.Secrets.Lookup` is what the builder reads (`pkg/server/middleware/secrets.go:17`, `pkg/middlewares/auth/auth.go:107`).

`verify/repro/c5_mutation.sh` runs those tests plus `verify/repro/c5_reload_probe_test.go` (prints both production comparisons for the same two loaded configurations), then repeats with `SecretStore.Digest` made independent of the values, then restores:

```sh
verify/repro/c5_mutation.sh ~/repos/wt-7af818be
```

```console
=== baseline
--- PASS: TestSecretStore_Digest (0.00s)
    c5_reload_probe_test.go:25: unchanged Secret: provider hash equal=true, watcher DeepEqual=true
    c5_reload_probe_test.go:26: rotated Secret:   provider hash equal=false, watcher DeepEqual=false
--- PASS: TestLoadConfigurationFromCRD_secretUpdate (0.01s)
--- PASS: TestLoadConfigurationFromCRD_authSecretUpdate (0.00s)
--- PASS: TestLoadConfiguration_authSecretUpdate (0.05s)
=== mutation: digest no longer depends on the Secret values
--- FAIL: TestSecretStore_Digest (0.00s)
    c5_reload_probe_test.go:25: unchanged Secret: provider hash equal=true, watcher DeepEqual=true
    c5_reload_probe_test.go:26: rotated Secret:   provider hash equal=true, watcher DeepEqual=true
--- FAIL: TestLoadConfigurationFromCRD_secretUpdate (0.01s)
--- FAIL: TestLoadConfigurationFromCRD_authSecretUpdate (0.00s)
--- FAIL: TestLoadConfiguration_authSecretUpdate (0.05s)
```

Observed: the branch's tests change a referenced Secret and assert that the loaded configuration differs (the comparison the watcher makes); on the same pair the provider's hash gate also differs; and the tests fail exactly when the production fingerprint stops tracking the value. They also assert the rotated value is what the builder's lookup returns.

Log: `verify/logs/c5_mutation.txt`.

## C6

Branch `evalon/traefik-se-12ccb497`. Reference construction in `pkg/provider/kubernetes/crd/kubernetes.go`:

```go
ref := dynamic.SecretURN(basicAuth.Secret, "username") + ":" + dynamic.SecretURN(basicAuth.Secret, "password")
secretValues[ref] = credentials
...
key, credentials, err := loadAuthCredentials(secret)   // returns the single Secret.Data key it read
Users: dynamic.Users{addSecretValue(secretValues, basicAuth.Secret, key, strings.Join(credentials, "\n"))},
```

Probes (`verify/repro/c6_crd_auth_reference_probe_test.go`, `verify/repro/c6_ingress_nginx_auth_reference_probe_test.go`) run the production loaders, split each retained reference into the Secret keys it names, check each key exists in `Secret.Data`, then change only that key's value and compare the credential the resolver produces.

```sh
cp verify/repro/c6_crd_auth_reference_probe_test.go ~/repos/wt-12ccb497/pkg/provider/kubernetes/crd/
cp verify/repro/c6_ingress_nginx_auth_reference_probe_test.go ~/repos/wt-12ccb497/pkg/provider/kubernetes/ingress-nginx/
cd ~/repos/wt-12ccb497 && go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx
```

```console
    c6_crd_auth_reference_probe_test.go:63: Secret.Data keys=username,password
    c6_crd_auth_reference_probe_test.go:64: retained users=["urn:k8s:secret:creds:username:urn:k8s:secret:creds:password"]
    c6_crd_auth_reference_probe_test.go:65: resolved users=["alice:{SHA}JauGvtFJymypwcDV23yakTiN3qs="]
    c6_crd_auth_reference_probe_test.go:83: named key "username": exists in Secret.Data=true, value contributes (resolved users change when only it changes)=true
    c6_crd_auth_reference_probe_test.go:83: named key "password": exists in Secret.Data=true, value contributes (resolved users change when only it changes)=true
    c6_crd_auth_reference_probe_test.go:63: Secret.Data keys=my.users
    c6_crd_auth_reference_probe_test.go:64: retained users=["urn:k8s:secret:creds:my.users"]
    c6_crd_auth_reference_probe_test.go:83: named key "my.users": exists in Secret.Data=true, value contributes (resolved users change when only it changes)=true
    c6_crd_auth_reference_probe_test.go:63: Secret.Data keys=htdigest
    c6_crd_auth_reference_probe_test.go:64: retained users=["urn:k8s:secret:creds:htdigest"]
    c6_crd_auth_reference_probe_test.go:83: named key "htdigest": exists in Secret.Data=true, value contributes (resolved users change when only it changes)=true
--- PASS: TestVerify_C6_ComposedReferencesNameContributingKeys (0.05s)
    c6_ingress_nginx_auth_reference_probe_test.go:51: Secret.Data keys=[auth unrelated] retained users=["urn:k8s:secret:auth:auth"] resolved users=["user:{SHA}W6ph5Mm5Pz8GgiULbPgzG37mj9g="]
    c6_ingress_nginx_auth_reference_probe_test.go:61: named key "auth": exists in Secret.Data=true, value contributes=true
    c6_ingress_nginx_auth_reference_probe_test.go:51: Secret.Data keys=[user user2] retained users=["urn:k8s:secret:auth:user" "urn:k8s:secret:auth:user2"] resolved users=["user:{SHA}W6ph5Mm5Pz8GgiULbPgzG37mj9g=" "user2:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0"]
    c6_ingress_nginx_auth_reference_probe_test.go:61: named key "user": exists in Secret.Data=true, value contributes=true
    c6_ingress_nginx_auth_reference_probe_test.go:61: named key "user2": exists in Secret.Data=true, value contributes=true
    c6_ingress_nginx_auth_reference_probe_test.go:61: named key "auth": exists in Secret.Data=true, value contributes=true
--- PASS: TestVerify_C6_IngressNginxReferencesNameContributingKeys (0.05s)
```

Observed: every key named by a reference exists and was read; each named value changes the derived credential, including the composed `username`+`password` reference; the unread sibling key `unrelated` is not named.

Logs: `verify/logs/c6_crd.txt`, `verify/logs/c6_ingress_nginx.txt`.

## C7

Branch `evalon/traefik-se-f176eba1`. `dynamic.Middleware` gained an unexported pointer as its first field:

```go
type Middleware struct {
	// Secret snapshots are immutable after assignment, so generated copies can share them.
	secretValues     *SecretValues
	AddPrefix        *AddPrefix        `json:"addPrefix,omitempty" ...
```

`extractType` (`pkg/api/handler.go:181`, unchanged by the branch) returns the name of the first non-nil pointer-to-struct field, and checks fields in order, so it also returns before reaching the `Plugin` map. The CRD provider sets the carrier on the middleware whenever it bound a Secret for plugin, forwardAuth or rateLimit (`kubernetes.go:375`: `if len(secretValues.References()) > 0 { dynamicMiddleware.SetSecretValues(secretValues) }`); basicAuth/digestAuth keep their carrier inside `BasicAuth`/`DigestAuth` instead.

Probe: `verify/repro/c7_api_type_probe_test.go` (package `api`) with `verify/repro/c7_crd_export_verify.go` (build-tagged instrumentation that exposes the unmodified `Provider.loadConfigurationFromCRD` to another package). Path of the second test: CRD fixtures `with_ratelimit.yml`, `with_auth.yml`, `with_plugin_read_secret.yml` -> production loader -> `DeepCopy` (as the watcher does) -> `@kubernetescrd` names (as the aggregator does) -> `runtime.NewConfig` -> `api.New(...).createRouter()` -> `GET /api/http/middlewares`.

```sh
cp verify/repro/c7_crd_export_verify.go ~/repos/wt-f176eba1/pkg/provider/kubernetes/crd/
cp verify/repro/c7_api_type_probe_test.go ~/repos/wt-f176eba1/pkg/api/
cd ~/repos/wt-f176eba1 && go test -tags verify -count=1 -v -run '^TestVerify_C7' ./pkg/api
```

```console
    c7_api_type_probe_test.go:32: extractType(rateLimit, no carrier)        = "RateLimit"
    c7_api_type_probe_test.go:33: extractType(rateLimit, populated carrier) = "secretValues"
    c7_api_type_probe_test.go:34: extractType(plugin demo, populated carrier) = "secretValues"
--- FAIL: TestVerify_C7_ExtractTypeSelection (0.00s)
    c7_api_type_probe_test.go:65: GET /api/http/middlewares: name=default-basicauth@kubernetescrd type="basicauth"
    c7_api_type_probe_test.go:65: GET /api/http/middlewares: name=default-digestauth@kubernetescrd type="digestauth"
    c7_api_type_probe_test.go:65: GET /api/http/middlewares: name=default-forwardauth@kubernetescrd type="secretvalues"
    c7_api_type_probe_test.go:65: GET /api/http/middlewares: name=default-ratelimit@kubernetescrd type="secretvalues"
    c7_api_type_probe_test.go:65: GET /api/http/middlewares: name=default-test-secret@kubernetescrd type="secretvalues"
        	            	expected: "ratelimit"
        	            	actual  : "secretvalues"
        	            	expected: "forwardauth"
        	            	actual  : "secretvalues"
        	            	expected: "test-secret"
        	            	actual  : "secretvalues"
--- FAIL: TestVerify_C7_APIResponseType (0.06s)
FAIL	github.com/traefik/traefik/v3/pkg/api	0.082s
```

- *Type selection:* observed — `extractType` returns `secretValues` for a middleware holding both its real configuration and a populated carrier (rateLimit and plugin).
- *API-path reachability:* observed — the HTTP response of the real API router reports `type="secretvalues"` for the Secret-backed forwardAuth, rateLimit and plugin middlewares produced by the CRD loader; the populated carrier survives `DeepCopy` (`*out = *in`) and reaches `newMiddlewareRepresentation` (`pkg/api/handler_http.go:70: Type: strings.ToLower(extractType(mi.Middleware))`). basicAuth and digestAuth report the right type.

Impact reasoning: `type` is the field of the API's middleware representation that says what a middleware is; any consumer of `/api/http/middlewares` (the dashboard reads this endpoint) gets it from there. On this branch every Kubernetes CRD forwardAuth with a TLS Secret, every rateLimit with a Redis Secret and every plugin with a Secret reference is reported as `secretvalues`: the middleware works, the answer about it is wrong, and nothing errors. The plugin case also loses the plugin name, which is otherwise the reported type. Using a Secret with these middlewares is the ordinary way to configure them on Kubernetes; the only way to avoid the wrong type is not to use a Secret. No secret value is exposed by this defect. Not exercised here: the dashboard UI itself and TCP middlewares.

Log: `verify/logs/c7_api.txt`.

## C8

Branch `evalon/traefik-se-4000ae16`. `SecretValues.Redact` (`pkg/config/dynamic/secrets.go`) replaces each quoted occurrence (`"v"`, `'v'`, `` `v` ``, `strconv.Quote`) of a bound value in the error text with the quoted reference; `BuildMiddlewareChain` applies it to every constructor error before `AddError` and before returning (`pkg/server/middleware/middlewares.go:88` and `:95`: `err = secrets.Redact(err)`).

Probe: `verify/repro/c8_plugin_diagnostic_probe_test.go` (package `middleware`) with `verify/repro/c8_crd_export_verify.go` (build-tagged instrumentation exposing the unmodified CRD loader with one Secret value overridden). The second test writes a real Yaegi local plugin whose `New` ignores its configuration and always returns `errors.New("invalid 'token'")`, loads `with_plugin_read_secret.yml` through the production CRD loader with the Secret value set to `token` and then to `other`, and builds the middleware with the production `plugins.Builder` and `middleware.Builder`.

```sh
cp verify/repro/c8_crd_export_verify.go ~/repos/wt-4000ae16/pkg/provider/kubernetes/crd/
cp verify/repro/c8_plugin_diagnostic_probe_test.go ~/repos/wt-4000ae16/pkg/server/middleware/
cd ~/repos/wt-4000ae16 && go test -tags verify -count=1 -v -run '^TestVerify_C8' ./pkg/server/middleware
```

```console
    c8_plugin_diagnostic_probe_test.go:25: Redact("invalid 'token'") with Secret value "token" -> "invalid 'urn:k8s:secret:name:key'"
    c8_plugin_diagnostic_probe_test.go:26: Redact("invalid 'token'") with Secret value "other" -> "invalid 'token'"
--- FAIL: TestVerify_C8_RedactFixedDiagnostic (0.00s)
    c8_plugin_diagnostic_probe_test.go:79: Secret value "token": returned error="invalid 'urn:k8s:secret:name:key'", MiddlewareInfo.Err (exposed by the API)=["invalid 'urn:k8s:secret:name:key'"]
    c8_plugin_diagnostic_probe_test.go:80: Secret value "other": returned error="invalid 'token'", MiddlewareInfo.Err (exposed by the API)=["invalid 'token'"]
--- FAIL: TestVerify_C8_PluginDiagnosticPath (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.080s
```

- *Value-dependent replacement:* observed — the same fixed error gives different output depending only on the bound Secret value.
- *Plugin diagnostic reachability:* observed — a fixed plugin error passes through the value-matching redaction before it is returned and stored in `MiddlewareInfo.Err`, which the API serializes as the middleware's `error`.

Impact reasoning: the text a plugin emits did not contain the Secret, yet what the operator sees depends on the Secret's value. Two consequences were observed directly. First, the diagnostic is rewritten into something the plugin never said (`invalid 'urn:k8s:secret:name:key'` instead of `invalid 'token'`), pointing at the Secret reference when the quoted word was a fixed part of the message. Second, the output is an equality test on the Secret: anyone who can read the middleware error (API `error` field, dashboard, logs) learns whether the Secret value equals a quoted word of the message — here, that the Secret is literally `token` — which is the disclosure the feature exists to prevent, limited to values that coincide with quoted text in an error. The trigger needs a failing plugin whose message quotes a word equal to the Secret value, so it is uncommon, but short or dictionary-like values (`token`, `string`, `invalid`, `true`) are where it happens, and there is no configuration workaround. The branch's own comment on `Redact` acknowledges the hazard for unquoted occurrences only. Not exercised here: non-plugin middlewares, though lines 88/95 apply the same redaction to every middleware type.

Log: `verify/logs/c8_plugin.txt`.
