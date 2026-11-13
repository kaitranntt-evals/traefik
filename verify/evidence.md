Observations for run `v-936b5f4d`. Setup shared by all sections (claim branches are fetched from the claim repository as remote `ev`, one detached worktree per branch; the byte-exact eval fixtures from `eval_tests.zip` are extracted to `~/eval/tests`):

```sh
cd ~/repos/traefik
git remote add ev https://git-manager.itsdev.in/proxy/github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch ev
git worktree add --detach ~/wt/<id> ev/evalon/traefik-se-<id>
# Only for the probes that say "eval fixtures provisioned": copy the fixtures, layout preserved, into the worktree.
cp -r ~/eval/tests/pkg ~/wt/<id>/
```

Go: `go1.26.3 linux/amd64`. All claim branches have merge base `af6d927c51191ae1df104aaf907793386b816c28`. Probes are `//go:build verify` test files copied into the target package; no production file is modified except by the two mutation scripts (C1, C8), which restore it on exit. Raw outputs are in `verify/logs/`.

## C1

Target: [evalon/traefik-se-a2edef14](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-a2edef14) at `9c989dca3098fb3ec89325981e34b6137ec6afc7`.

Assertion text, `pkg/provider/kubernetes/crd/kubernetes_test.go` (`TestLoadIngressRoutes`); the same loop is in `pkg/provider/kubernetes/ingress-nginx/kubernetes_test.go` (`TestLoadIngresses`):

```go
for name, actual := range conf.HTTP.Middlewares {
    if expected := test.expected.HTTP.Middlewares[name]; expected != nil {
        expected.SetSecretValues(actual.SecretValues())
    }
}
assert.Equal(t, test.expected, conf)
```

The expected objects of the plugin fixtures were changed from the credential to the reference (e.g. `"secret": "urn:k8s:secret:name:key"`), so the bound values in `expected` are whatever the provider produced.

Demonstration: `verify/repro/c1_deep_plugin_mutation.sh` prints the values the provider binds for the four plugin fixtures (`verify/repro/c1_probe_test.go`), then patches `loadSecretKeys` in `pkg/provider/kubernetes/crd/kubernetes.go` so that every Secret value bound for a nested plugin field (inside a map or list below the top level) becomes `CORRUPTED-BY-VERIFY`, and runs the branch's committed tests.

```sh
WT=~/wt/a2edef14 bash verify/repro/c1_deep_plugin_mutation.sh
```

```console
--- bound values, unmodified branch ---
BOUND with_plugin_read_secret.yml default-test-secret urn:k8s:secret:name:key = "this_is_the_secret"
BOUND with_plugin_deep_read_secret.yml default-test-secret urn:k8s:secret:name:key = "this_is_the_very_deep_secret"
BOUND with_plugin_read_array_of_secret.yml default-test-secret urn:k8s:secret:name:key1 = "secret_data1"
BOUND with_plugin_read_array_of_secret.yml default-test-secret urn:k8s:secret:name:key2 = "secret_data2"
BOUND with_plugin_read_array_of_map_contain_secret.yml default-test-secret urn:k8s:secret:name:key1 = "admin_password"
BOUND with_plugin_read_array_of_map_contain_secret.yml default-test-secret urn:k8s:secret:name:key2 = "user_password"
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.076s
 pkg/provider/kubernetes/crd/kubernetes.go | 9 +++++++++
 1 file changed, 9 insertions(+)
--- bound values, mutated ---
BOUND with_plugin_read_secret.yml default-test-secret urn:k8s:secret:name:key = "this_is_the_secret"
BOUND with_plugin_deep_read_secret.yml default-test-secret urn:k8s:secret:name:key = "CORRUPTED-BY-VERIFY"
BOUND with_plugin_read_array_of_secret.yml default-test-secret urn:k8s:secret:name:key1 = "CORRUPTED-BY-VERIFY"
BOUND with_plugin_read_array_of_secret.yml default-test-secret urn:k8s:secret:name:key2 = "CORRUPTED-BY-VERIFY"
BOUND with_plugin_read_array_of_map_contain_secret.yml default-test-secret urn:k8s:secret:name:key1 = "CORRUPTED-BY-VERIFY"
BOUND with_plugin_read_array_of_map_contain_secret.yml default-test-secret urn:k8s:secret:name:key2 = "CORRUPTED-BY-VERIFY"
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.091s
--- committed tests, mutated ---
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.015s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.030s
?   	github.com/traefik/traefik/v3/pkg/server/middleware/tcp	[no test files]
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.439s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.475s
--- targeted (mutated) ---
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_deep_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_secret_that_not_found (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.06s)
--- PASS: TestKubernetesSecretReferencesAndReload (0.01s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.083s
```

The committed-test step is `go test -count=1 ./pkg/config/dynamic/... ./pkg/server/middleware/... ./pkg/provider/kubernetes/crd/... ./pkg/provider/kubernetes/ingress-nginx/...`, the targeted step is `go test -race -count=1 -v -run 'TestLoadIngressRoutes/.*test_middleware_read_config|TestKubernetesSecretReferencesAndReload' ./pkg/provider/kubernetes/crd/`.

Observed: with the deep-map, list and list-of-maps fixtures bound to a wrong credential, every committed test of the four packages still passes. The flat fixture (`with_plugin_read_secret.yml`) is the only shape whose value the mutation leaves intact, and the only one with an independent check (`TestKubernetesSecretReferencesAndReload`).

Impact reasoning: a regression that resolves a nested plugin Secret reference to the wrong value (wrong key, wrong Secret, empty) would ship with a green suite; before the change these fixtures compared the resolved credential against a literal.

## C2

Target: [evalon/traefik-se-d5bacf39](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-d5bacf39) at `a8e8a35cc0d3dbf1d72afa426ef2eb03a1cad6d2`.

Probe `verify/repro/c2_plugin_alias_probe_test.go` (eval fixtures provisioned). It loads each plugin fixture through the CRD provider, publishes it as the runtime (API) configuration and builds the middleware through the production `middleware.Builder`, twice, with:

- a recording plugin builder that captures the `config` argument, counts the maps/slices it shares (by pointer) with the published configuration, then mutates every nested container; success and failure variants;
- the real Yaegi builder with the eval local plugin, whose `CreateConfig` result and constructor config are mutated by the plugin; success, constructor-error and decode-error variants (alias count not observable there, printed as `-1`);
- a control where the Secret bindings are removed from the published middleware before building.

```sh
cp verify/repro/c2_plugin_alias_probe_test.go ~/wt/d5bacf39/pkg/provider/kubernetes/crd/zz_verify_c2_test.go
cd ~/wt/d5bacf39 && go test -tags verify -count=1 -run TestVerifyC2 -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(C2|---|ok|FAIL)'
```

```console
C2 with_plugin_deep_read_secret.yml                 recorder-success                   build#1 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 recorder-success                   build#2 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 recorder-failure                   build#1 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 recorder-failure                   build#2 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 yaegi-success                      build#1 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 yaegi-success                      build#2 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 yaegi-constructor-error            build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 yaegi-constructor-error            build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 yaegi-decode-error                 build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 yaegi-decode-error                 build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_deep_read_secret.yml                 CONTROL-recorder-secrets-stripped  build#1 err=false aliasedContainers=7 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_deep_read_secret.yml                 CONTROL-recorder-secrets-stripped  build#2 err=false aliasedContainers=7 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_read_array_of_secret.yml             recorder-success                   build#1 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             recorder-success                   build#2 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             recorder-failure                   build#1 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             recorder-failure                   build#2 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             yaegi-success                      build#1 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             yaegi-success                      build#2 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             yaegi-constructor-error            build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             yaegi-constructor-error            build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             yaegi-decode-error                 build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             yaegi-decode-error                 build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_secret.yml             CONTROL-recorder-secrets-stripped  build#1 err=false aliasedContainers=5 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_read_array_of_secret.yml             CONTROL-recorder-secrets-stripped  build#2 err=false aliasedContainers=5 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_read_array_of_map_contain_secret.yml recorder-success                   build#1 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml recorder-success                   build#2 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml recorder-failure                   build#1 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml recorder-failure                   build#2 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml yaegi-success                      build#1 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml yaegi-success                      build#2 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml yaegi-constructor-error            build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml yaegi-constructor-error            build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml yaegi-decode-error                 build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml yaegi-decode-error                 build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_array_of_map_contain_secret.yml CONTROL-recorder-secrets-stripped  build#1 err=false aliasedContainers=7 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_read_array_of_map_contain_secret.yml CONTROL-recorder-secrets-stripped  build#2 err=false aliasedContainers=7 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_read_secret.yml                      recorder-success                   build#1 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      recorder-success                   build#2 err=false aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      recorder-failure                   build#1 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      recorder-failure                   build#2 err=true aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      yaegi-success                      build#1 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      yaegi-success                      build#2 err=false aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      yaegi-constructor-error            build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      yaegi-constructor-error            build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      yaegi-decode-error                 build#1 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      yaegi-decode-error                 build#2 err=true aliasedContainers=-1 publishedUnchanged=true runtimeUnchanged=true
C2 with_plugin_read_secret.yml                      CONTROL-recorder-secrets-stripped  build#1 err=false aliasedContainers=4 publishedUnchanged=false runtimeUnchanged=false
C2 with_plugin_read_secret.yml                      CONTROL-recorder-secrets-stripped  build#2 err=false aliasedContainers=4 publishedUnchanged=false runtimeUnchanged=false
--- PASS: TestVerifyC2 (0.11s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.146s
```

Observed, per part:

- *configuration_alias*: an alias exists only in the control (bindings stripped): `aliasedContainers=4..7`, and the mutation changes the published and runtime configuration. So the builder does pass API-shared containers to plugins when the middleware has no Secret binding.
- *provider_bound_path*: with provider-loaded bindings, on success, failure, constructor-error and decode-error paths, first and second build: `aliasedContainers=0 publishedUnchanged=true runtimeUnchanged=true`. `Middleware.ResolveSecrets` → `Secrets.ResolvePluginConf` allocates fresh maps and slices before the plugin call, so the aliasing assignment is not reached for provider-bound configuration.

The claim requires both parts on the provider-bound path; the second does not hold.

## C3

Target: [evalon/traefik-se-70a4758f](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-70a4758f) at `8a0d10231d324dc2363e1972ddb3d83bda530046`.

Probe `verify/repro/c3_redis_mixed_snapshot_test.go`. It loads `with_ratelimit.yml` through `Provider.loadConfigurationFromCRD` with a client wrapper whose `GetSecret` returns snapshot A (the fixture values) for the first N reads and snapshot B (`user-B` / `password-B`, certificate B / key B, each individually valid) afterwards, logs every read and prints the bindings published on the `default-ratelimit` middleware.

```sh
cp verify/repro/c3_redis_mixed_snapshot_test.go ~/wt/70a4758f/pkg/provider/kubernetes/crd/zz_verify_c3_test.go
cd ~/wt/70a4758f && go test -tags verify -count=1 -run TestVerifyC3 -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(C3|---|ok|FAIL)'
```

```console
C3 rotationAfterRead=0 reads=[tlssecret#1=B redissecret#1=B redissecret#2=B redissecret#3=B tlssecret#2=B tlssecret#3=B]
C3   published urn:k8s:secret:redissecret:username="user-B" urn:k8s:secret:redissecret:password="password-B"
C3   published urn:k8s:secret:tlssecret:tls.crt="-----BEGIN CERTIFICATE-----\nB\n-----END CERTIFICATE-----" urn:k8s:secret:tlssecret:tls.key="-----BEGIN PRIVATE KEY-----\nB\n-----END PRIVATE KEY-----"
C3 rotationAfterRead=1 reads=[tlssecret#1=A redissecret#1=A redissecret#2=B redissecret#3=B tlssecret#2=B tlssecret#3=B]
C3   published urn:k8s:secret:redissecret:username="user-B" urn:k8s:secret:redissecret:password="password-B"
C3   published urn:k8s:secret:tlssecret:tls.crt="-----BEGIN CERTIFICATE-----\nB\n-----END CERTIFICATE-----" urn:k8s:secret:tlssecret:tls.key="-----BEGIN PRIVATE KEY-----\nB\n-----END PRIVATE KEY-----"
C3 rotationAfterRead=2 reads=[tlssecret#1=A redissecret#1=A redissecret#2=A redissecret#3=B tlssecret#2=A tlssecret#3=B]
C3   published urn:k8s:secret:redissecret:username="user" urn:k8s:secret:redissecret:password="password-B"
C3   published urn:k8s:secret:tlssecret:tls.crt="-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----" urn:k8s:secret:tlssecret:tls.key="-----BEGIN PRIVATE KEY-----\nB\n-----END PRIVATE KEY-----"
C3 rotationAfterRead=3 reads=[tlssecret#1=A redissecret#1=A redissecret#2=A redissecret#3=A tlssecret#2=A tlssecret#3=A]
C3   published urn:k8s:secret:redissecret:username="user" urn:k8s:secret:redissecret:password="password"
C3   published urn:k8s:secret:tlssecret:tls.crt="-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----" urn:k8s:secret:tlssecret:tls.key="-----BEGIN PRIVATE KEY-----\n-----END PRIVATE KEY-----"
--- PASS: TestVerifyC3 (0.06s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.084s
```

Observed, per part:

- *pair_snapshot_selection*: one middleware load reads `redissecret` three times and `tlssecret` three times (`createRateLimitMiddleware` validates with one read, `protectMiddlewareSecrets` then reads once per field). Username and password, and certificate and key, come from separate reads.
- *rotation_exposure*: with the rotation between reads 2 and 3 of each Secret (`rotationAfterRead=2`), the published bindings are username `user` (A) with password `password-B` (B), and certificate A with private key B. No error, no retry; the configuration is published.

Impact reasoning: a Secret update landing between two reads of one provider load publishes a credential pair that never existed. Redis rejects the mismatched user/password, or the TLS key does not match the certificate, so the rate limiter cannot be built or connect until another Kubernetes event triggers a reload that reads a consistent pair. The window is one load, but credential rotation is an ordinary operation and nothing signals the mixed state.

## C4

Target: [evalon/traefik-se-5b5c5895](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-5b5c5895) at `62e9e840f8fed82165266f282af20e080fb9bb87`.

Probe `verify/repro/c4_retained_createconfig_test.go` with the local Yaegi plugin `verify/repro/verifyretain/` (eval fixtures provisioned). The plugin keeps every object returned by its `CreateConfig` and, on each later `CreateConfig` call, mutates the nested containers of the retained objects. The middleware is loaded through the CRD provider, published as the runtime (API) configuration and built twice through the production builder; `eval_reject: not-an-integer` makes decoding fail.

```sh
mkdir -p ~/wt/5b5c5895/pkg/provider/kubernetes/crd/plugins-local/src/verifyretain
cp verify/repro/verifyretain/verifyretain.go verify/repro/verifyretain/.traefik.yml ~/wt/5b5c5895/pkg/provider/kubernetes/crd/plugins-local/src/verifyretain/
cp verify/repro/c4_retained_createconfig_test.go ~/wt/5b5c5895/pkg/provider/kubernetes/crd/zz_verify_c4_test.go
cd ~/wt/5b5c5895 && go test -tags verify -count=1 -run TestVerifyC4 -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(C4|---|ok|FAIL)'
```

```console
C4 [failed decoding, deep map]
C4   published before : {"test-secret":{"eval_reject":"not-an-integer","secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   build#1 err=plugin: failed to decode configuration: 1 error(s) decoding:
C4   published after  : {"test-secret":{"eval_reject":"not-an-integer","secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   runtime (API)     : {"test-secret":{"eval_reject":"not-an-integer","secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   unchanged=true
C4   build#2 err=plugin: failed to decode configuration: 1 error(s) decoding:
C4   published after  : {"test-secret":{"eval_reject":"not-an-integer","secret_0":{"secret_1":{"secret_2":{"secret":"MUTATED-BY-RETAINED-CONFIG","user":"MUTATED-BY-RETAINED-CONFIG","verify_added":"MUTATED-BY-RETAINED-CONFIG"},"verify_added":"MUTATED-BY-RETAINED-CONFIG"},"verify_added":"MUTATED-BY-RETAINED-CONFIG"}}}
C4   runtime (API)     : {"test-secret":{"eval_reject":"not-an-integer","secret_0":{"secret_1":{"secret_2":{"secret":"MUTATED-BY-RETAINED-CONFIG","user":"MUTATED-BY-RETAINED-CONFIG","verify_added":"MUTATED-BY-RETAINED-CONFIG"},"verify_added":"MUTATED-BY-RETAINED-CONFIG"},"verify_added":"MUTATED-BY-RETAINED-CONFIG"}}}
C4   unchanged=false
C4 [failed decoding, list of maps]
C4   published before : {"test-secret":{"eval_reject":"not-an-integer","users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}}
C4   build#1 err=plugin: failed to decode configuration: 1 error(s) decoding:
C4   published after  : {"test-secret":{"eval_reject":"not-an-integer","users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}}
C4   runtime (API)     : {"test-secret":{"eval_reject":"not-an-integer","users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}}
C4   unchanged=true
C4   build#2 err=plugin: failed to decode configuration: 1 error(s) decoding:
C4   published after  : {"test-secret":{"eval_reject":"not-an-integer","users":[{"name":"MUTATED-BY-RETAINED-CONFIG","secret":"MUTATED-BY-RETAINED-CONFIG","verify_added":"MUTATED-BY-RETAINED-CONFIG"},{"name":"MUTATED-BY-RETAINED-CONFIG","secret":"MUTATED-BY-RETAINED-CONFIG","verify_added":"MUTATED-BY-RETAINED-CONFIG"}]}}
C4   runtime (API)     : {"test-secret":{"eval_reject":"not-an-integer","users":[{"name":"MUTATED-BY-RETAINED-CONFIG","secret":"MUTATED-BY-RETAINED-CONFIG","verify_added":"MUTATED-BY-RETAINED-CONFIG"},{"name":"MUTATED-BY-RETAINED-CONFIG","secret":"MUTATED-BY-RETAINED-CONFIG","verify_added":"MUTATED-BY-RETAINED-CONFIG"}]}}
C4   unchanged=false
C4 [failed decoding, list]
C4   published before : {"test-secret":{"eval_reject":"not-an-integer","secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}}
C4   build#1 err=plugin: failed to decode configuration: 1 error(s) decoding:
C4   published after  : {"test-secret":{"eval_reject":"not-an-integer","secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}}
C4   runtime (API)     : {"test-secret":{"eval_reject":"not-an-integer","secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}}
C4   unchanged=true
C4   build#2 err=plugin: failed to decode configuration: 1 error(s) decoding:
C4   published after  : {"test-secret":{"eval_reject":"not-an-integer","secret":["MUTATED-BY-RETAINED-CONFIG","MUTATED-BY-RETAINED-CONFIG"]}}
C4   runtime (API)     : {"test-secret":{"eval_reject":"not-an-integer","secret":["MUTATED-BY-RETAINED-CONFIG","MUTATED-BY-RETAINED-CONFIG"]}}
C4   unchanged=false
C4 [CONTROL successful decoding, deep map]
C4   published before : {"test-secret":{"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   build#1 err=<nil>
C4   published after  : {"test-secret":{"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   runtime (API)     : {"test-secret":{"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   unchanged=true
C4   build#2 err=<nil>
C4   published after  : {"test-secret":{"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   runtime (API)     : {"test-secret":{"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}}
C4   unchanged=true
--- PASS: TestVerifyC4 (0.07s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.101s
```

Observed, per part:

- *retained_object_alias*: after the first failed build the published configuration is unchanged; when the plugin mutates the object retained from that build (during build #2), the published and runtime (API) configuration change (`"secret":"MUTATED-BY-RETAINED-CONFIG"`, added keys). The retained object therefore shares nested maps/slices with the API-shared configuration.
- *failed_decode_path*: this happens only when decoding fails (deep map, list of maps, list). In the control with successful decoding both builds leave the configuration unchanged, so the alias is created by the failed-decoding error-reporting path, which decodes the unresolved (reference-only) API-shared plugin configuration into a `CreateConfig` result.

Impact reasoning: a plugin whose configuration fails to decode ends up holding live pointers into the configuration served by `/api/rawdata`; any later write through them changes what the API reports (and races with readers). Requires a plugin that keeps its `CreateConfig` result and a configuration that fails decoding.

## C5

Target: [evalon/traefik-se-6b2a0d3d](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-6b2a0d3d) at `2b40e41f22c3bdaa01452918c038470d5dcb745e`.

Probe `verify/repro/c5_c6_plugin_error_probe_test.go` with the local Yaegi plugin `verify/repro/verifyerr/` (eval fixtures provisioned). For each error shape (`mode`) it:

1. builds the plugin directly with the production `plugins.Builder` and the plain credential `verify_s3cret_VALUE_zq7`, and walks the returned error with host-Go `errors.Unwrap` (single and multi) — the *original* error;
2. loads a Middleware referencing `urn:k8s:secret:name:key` through the CRD provider (Secret value `verify_s3cret_VALUE_zq7`), builds it twice through the production middleware builder (`evalBuildHandler`), and prints the returned error, its host-reachable chain and the stored `MiddlewareInfo.Err`.

Modes: `native-wrap` returns `fmt.Errorf("verifyerr: internal error%.0w", cause)` (a host `*fmt.wrapError` whose message hides the credential-bearing cause); `native-join` returns `errors.Join(...)`; `interpreted-unwrap` returns a plugin-defined type with `Unwrap`; `cached` stores the first credential-bearing error and returns that same error on every later invocation (including a reference-only replay); `cached-always` caches the error of the first invocation whatever it received.

```sh
mkdir -p ~/wt/6b2a0d3d/pkg/provider/kubernetes/crd/plugins-local/src/verifyerr
cp verify/repro/verifyerr/verifyerr.go verify/repro/verifyerr/.traefik.yml ~/wt/6b2a0d3d/pkg/provider/kubernetes/crd/plugins-local/src/verifyerr/
cp verify/repro/c5_c6_plugin_error_probe_test.go ~/wt/6b2a0d3d/pkg/provider/kubernetes/crd/zz_verify_c5c6_test.go
cd ~/wt/6b2a0d3d && go test -tags verify -count=1 -run TestVerifyPluginErrors -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(===|  |---|ok|FAIL|#)|Error' | grep -v '=== RUN'
```

Output (modes `interpreted-unwrap`, `native-wrap`, `native-join`; full output in `verify/logs/c5c6_probe_6b2a0d3d.log`):

```console

=== mode=interpreted-unwrap
  original error: top="verifyerr: internal error"
    host chain[0] interp._error: "verifyerr: internal error"
  original: credentialInTopMessage=false credentialReachableViaHostUnwrap=false
  production build#1 returned="verifyerr: internal error"
    host chain[0] interp._error: "verifyerr: internal error"
    stored MiddlewareInfo.Err=["verifyerr: internal error"]
    LEAK returnedMessage=false reachableCause=false storedDiagnostic=false
  production build#2 returned="verifyerr: internal error"
    host chain[0] interp._error: "verifyerr: internal error"
    stored MiddlewareInfo.Err=["verifyerr: internal error"]
    LEAK returnedMessage=false reachableCause=false storedDiagnostic=false
=== mode=native-wrap
  original error: top="verifyerr: internal error"
    host chain[0] *fmt.wrapError: "verifyerr: internal error"
    host chain[1] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
  original: credentialInTopMessage=false credentialReachableViaHostUnwrap=true
  production build#1 returned="verifyerr: internal error"
    host chain[0] *fmt.wrapError: "verifyerr: internal error"
    host chain[1] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
    stored MiddlewareInfo.Err=["verifyerr: internal error"]
    LEAK returnedMessage=false reachableCause=true storedDiagnostic=false
  production build#2 returned="verifyerr: internal error"
    host chain[0] *fmt.wrapError: "verifyerr: internal error"
    host chain[1] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
    stored MiddlewareInfo.Err=["verifyerr: internal error"]
    LEAK returnedMessage=false reachableCause=true storedDiagnostic=false
=== mode=native-join
  original error: top="verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[0] *errors.joinError: "verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[1] *errors.errorString: "verifyerr: internal error"
    host chain[2] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
  original: credentialInTopMessage=true credentialReachableViaHostUnwrap=true
  production build#1 returned="verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[0] *errors.joinError: "verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[1] *errors.errorString: "verifyerr: internal error"
    host chain[2] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
    stored MiddlewareInfo.Err=["verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"]
    LEAK returnedMessage=true reachableCause=true storedDiagnostic=true
  production build#2 returned="verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[0] *errors.joinError: "verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[1] *errors.errorString: "verifyerr: internal error"
    host chain[2] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
    stored MiddlewareInfo.Err=["verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"]
    LEAK returnedMessage=true reachableCause=true storedDiagnostic=true
```

Observed, per part:

- *cause_preservation*: the original `native-wrap` error has a clean message and a host-reachable cause holding the credential (`host chain[1] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"`). After provider-bound construction the returned error is the same chain: `LEAK ... reachableCause=true`. (A plugin-defined `Unwrap` is not host-reachable through Yaegi: `interp._error`, chain length 1 — so that shape is not an exposure.)
- *diagnostic_exposure*: the preserved cause is in the error returned by middleware construction on both builds. For `native-join`, whose message includes the cause, the credential is also in the returned message and in the stored `MiddlewareInfo.Err` (`storedDiagnostic=true`).

On this branch `pluginErrorWithoutSecrets` only handles errors of `pluginBuilder.Build` (decoding); errors returned by the plugin constructor through `newTraceablePlugin` are propagated untouched. Consistent with that, the byte-exact eval tests fail on this branch for the constructor paths:

```sh
cd ~/wt/6b2a0d3d && go test -count=1 -run 'TestEval_ProviderToBuilder_Plugin(Cached|Build)ErrorRedaction' -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^\s*--- '
```

```console
--- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (0.66s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/decode_error (0.06s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor_error (0.00s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/decode (0.01s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor (0.18s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constant (0.17s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/reject_resolved_value (0.03s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/wrapped_cause (0.05s)
--- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction (0.76s)
    --- PASS: TestEval_ProviderToBuilder_PluginCachedErrorRedaction/decode_error (0.00s)
    --- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction/constructor_error (0.05s)
    --- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction/wrapped_cause (0.05s)
```

(Third-level subtests omitted; full list in `verify/logs/c5_eval_fixture.log`.)

Impact reasoning: any plugin whose constructor rejects its configuration with an error that carries a resolved Secret value (directly, joined, or as a wrapped cause) puts that value in the middleware construction error: logged with the router error, stored in `MiddlewareInfo.Err` and served by `/api/http/middlewares` when the message carries it, and reachable by any host code using `errors.Unwrap`/`errors.As` when only the cause carries it. Quoting the rejected value in an error is common plugin behavior.

## C6

Target: [evalon/traefik-se-64dd1e37](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-64dd1e37) at `6f18e8665f329b2134431e0eb95b38f35ac534d0`.

Probe `verify/repro/c5_c6_plugin_error_probe_test.go` with the local Yaegi plugin `verify/repro/verifyerr/` (eval fixtures provisioned). For each error shape (`mode`) it:

1. builds the plugin directly with the production `plugins.Builder` and the plain credential `verify_s3cret_VALUE_zq7`, and walks the returned error with host-Go `errors.Unwrap` (single and multi) — the *original* error;
2. loads a Middleware referencing `urn:k8s:secret:name:key` through the CRD provider (Secret value `verify_s3cret_VALUE_zq7`), builds it twice through the production middleware builder (`evalBuildHandler`), and prints the returned error, its host-reachable chain and the stored `MiddlewareInfo.Err`.

Modes: `native-wrap` returns `fmt.Errorf("verifyerr: internal error%.0w", cause)` (a host `*fmt.wrapError` whose message hides the credential-bearing cause); `native-join` returns `errors.Join(...)`; `interpreted-unwrap` returns a plugin-defined type with `Unwrap`; `cached` stores the first credential-bearing error and returns that same error on every later invocation (including a reference-only replay); `cached-always` caches the error of the first invocation whatever it received.

```sh
mkdir -p ~/wt/64dd1e37/pkg/provider/kubernetes/crd/plugins-local/src/verifyerr
cp verify/repro/verifyerr/verifyerr.go verify/repro/verifyerr/.traefik.yml ~/wt/64dd1e37/pkg/provider/kubernetes/crd/plugins-local/src/verifyerr/
cp verify/repro/c5_c6_plugin_error_probe_test.go ~/wt/64dd1e37/pkg/provider/kubernetes/crd/zz_verify_c5c6_test.go
cd ~/wt/64dd1e37 && go test -tags verify -count=1 -run TestVerifyPluginErrors -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(===|  |---|ok|FAIL|#)|Error' | grep -v '=== RUN'
```

Output (modes `native-wrap`, `native-join`, `cached`, `cached-always`; full output in `verify/logs/c5c6_probe_64dd1e37.log`):

```console

=== mode=native-wrap
  original error: top="verifyerr: internal error"
    host chain[0] *fmt.wrapError: "verifyerr: internal error"
    host chain[1] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
  original: credentialInTopMessage=false credentialReachableViaHostUnwrap=true
  production build#1 returned="verifyerr: internal error"
    host chain[0] *fmt.wrapError: "verifyerr: internal error"
    host chain[1] *errors.errorString: "underlying failure with secret: urn:k8s:secret:name:key"
    stored MiddlewareInfo.Err=["verifyerr: internal error"]
    LEAK returnedMessage=false reachableCause=false storedDiagnostic=false
  production build#2 returned="verifyerr: internal error"
    host chain[0] *fmt.wrapError: "verifyerr: internal error"
    host chain[1] *errors.errorString: "underlying failure with secret: urn:k8s:secret:name:key"
    stored MiddlewareInfo.Err=["verifyerr: internal error"]
    LEAK returnedMessage=false reachableCause=false storedDiagnostic=false
=== mode=native-join
  original error: top="verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[0] *errors.joinError: "verifyerr: internal error\nunderlying failure with secret: verify_s3cret_VALUE_zq7"
    host chain[1] *errors.errorString: "verifyerr: internal error"
    host chain[2] *errors.errorString: "underlying failure with secret: verify_s3cret_VALUE_zq7"
  original: credentialInTopMessage=true credentialReachableViaHostUnwrap=true
  production build#1 returned="verifyerr: internal error\nunderlying failure with secret: urn:k8s:secret:name:key"
    host chain[0] *errors.joinError: "verifyerr: internal error\nunderlying failure with secret: urn:k8s:secret:name:key"
    host chain[1] *errors.errorString: "verifyerr: internal error"
    host chain[2] *errors.errorString: "underlying failure with secret: urn:k8s:secret:name:key"
    stored MiddlewareInfo.Err=["verifyerr: internal error\nunderlying failure with secret: urn:k8s:secret:name:key"]
    LEAK returnedMessage=false reachableCause=false storedDiagnostic=false
  production build#2 returned="verifyerr: internal error\nunderlying failure with secret: urn:k8s:secret:name:key"
    host chain[0] *errors.joinError: "verifyerr: internal error\nunderlying failure with secret: urn:k8s:secret:name:key"
    host chain[1] *errors.errorString: "verifyerr: internal error"
    host chain[2] *errors.errorString: "underlying failure with secret: urn:k8s:secret:name:key"
    stored MiddlewareInfo.Err=["verifyerr: internal error\nunderlying failure with secret: urn:k8s:secret:name:key"]
    LEAK returnedMessage=false reachableCause=false storedDiagnostic=false
=== mode=cached
  original error: top="verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    host chain[0] *errors.errorString: "verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
  original: credentialInTopMessage=true credentialReachableViaHostUnwrap=false
  production build#1 returned="verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    host chain[0] *errors.errorString: "verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    stored MiddlewareInfo.Err=["verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""]
    LEAK returnedMessage=true reachableCause=false storedDiagnostic=true
  production build#2 returned="verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    host chain[0] *errors.errorString: "verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    stored MiddlewareInfo.Err=["verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""]
    LEAK returnedMessage=true reachableCause=false storedDiagnostic=true
=== mode=cached-always
  original error: top="verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    host chain[0] *errors.errorString: "verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
  original: credentialInTopMessage=true credentialReachableViaHostUnwrap=false
  production build#1 returned="verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    host chain[0] *errors.errorString: "verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    stored MiddlewareInfo.Err=["verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""]
    LEAK returnedMessage=true reachableCause=false storedDiagnostic=true
  production build#2 returned="verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    host chain[0] *errors.errorString: "verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""
    stored MiddlewareInfo.Err=["verifyerr: rejected credential \"verify_s3cret_VALUE_zq7\""]
    LEAK returnedMessage=true reachableCause=false storedDiagnostic=true
--- PASS: TestVerifyPluginErrors (0.09s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.119s
```

Observed, per part:

- *cached_error_handling*: this branch's `secretPluginError` replaces a constructor error by the error of a replay built from the reference-only configuration. That works for stateless errors (`native-wrap`: cause gone; `native-join`: message now shows `urn:k8s:secret:name:key`). For the stateful plugin the replay calls the constructor again, which returns the cached credential-bearing error unchanged, and that error is used as the "safe" one.
- *cached_error_path*: `cached` and `cached-always`, both builds: returned error `verifyerr: rejected credential "verify_s3cret_VALUE_zq7"`, stored `MiddlewareInfo.Err` identical, `LEAK returnedMessage=true ... storedDiagnostic=true`.

Impact reasoning: a plugin that memoizes its validation/initialization error (package-level `sync.Once`, cached client error, …) exposes the resolved Secret value in the construction error that is logged and in the `MiddlewareInfo.Err` served by the API, on every build. The replay design assumes the plugin constructor is a pure function of its configuration.

## C7

Target: [evalon/traefik-se-22e29d38](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-22e29d38) at `7c12f1b2bfff6c4329394cf520a6aaaefc932799`.

Code under test, `pkg/provider/kubernetes/crd/kubernetes.go` (`loadConfigurationFromCRD`): the middleware is registered with `addToConfig(..., &dynamic.Middleware{...})`, which warns when `!reflect.DeepEqual(existing, obj)`, and only afterwards `conf.HTTP.Middlewares[id].Secrets = secrets` is assigned.

Probe `verify/repro/c7_collision_warning_test.go`. With the legacy naming (SafeNaming unset), Middleware `c` in namespace `a-b` and Middleware `b-c` in namespace `a` both get the identifier `a-b-c`. Three pairs are loaded through `Provider.loadConfigurationFromCRD` with a captured zerolog logger: a basicAuth pair whose `authsecret` Secrets hold the same users, a control pair without Secret (`addPrefix`), and a control basicAuth pair whose Secrets differ. Each resource is also loaded alone to print the complete object (configuration and number of bindings) and compare the two complete objects.

```sh
cp verify/repro/c7_collision_warning_test.go ~/wt/22e29d38/pkg/provider/kubernetes/crd/zz_verify_c7_test.go
cd ~/wt/22e29d38 && go test -tags verify -count=1 -run TestVerifyC7 -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(C7|---|ok|FAIL|#)|rror'
```

```console
C7 === secret-backed equivalent pair (same users in both Secrets)
C7   loaded alone: a-b/c -> id a-b-c basicAuth=&{Users:[urn:k8s:secret:authsecret:users] UsersFile: Realm: RemoveHeader:false HeaderField:} addPrefix=<nil> bindings=1
C7   loaded alone: a/b-c -> id a-b-c basicAuth=&{Users:[urn:k8s:secret:authsecret:users] UsersFile: Realm: RemoveHeader:false HeaderField:} addPrefix=<nil> bindings=1
C7   complete objects DeepEqual=true
C7   loaded together: middlewares=1 collisionWarnings=1
C7   LOG {"level":"warn","middlewareName":"a-b-c","message":"Name collision: middleware \"a-b-c\" is defined more than once, the previous definition will be overridden"}
C7 === CONTROL equivalent pair without Secret
C7   loaded alone: a-b/c -> id a-b-c basicAuth=<nil> addPrefix=&{Prefix:/x} bindings=0
C7   loaded alone: a/b-c -> id a-b-c basicAuth=<nil> addPrefix=&{Prefix:/x} bindings=0
C7   complete objects DeepEqual=true
C7   loaded together: middlewares=1 collisionWarnings=0
C7 === CONTROL secret-backed pair with different Secret values
C7   loaded alone: a-b/c -> id a-b-c basicAuth=&{Users:[urn:k8s:secret:authsecret:users] UsersFile: Realm: RemoveHeader:false HeaderField:} addPrefix=<nil> bindings=1
C7   loaded alone: a/b-c -> id a-b-c basicAuth=&{Users:[urn:k8s:secret:authsecret:users] UsersFile: Realm: RemoveHeader:false HeaderField:} addPrefix=<nil> bindings=1
C7   complete objects DeepEqual=false
C7   loaded together: middlewares=1 collisionWarnings=1
C7   LOG {"level":"warn","middlewareName":"a-b-c","message":"Name collision: middleware \"a-b-c\" is defined more than once, the previous definition will be overridden"}
--- PASS: TestVerifyC7 (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.102s
```

Observed: the two Secret-backed resources produce complete objects that are `reflect.DeepEqual` (same references, same bindings), yet loading them together logs `Name collision: middleware "a-b-c" is defined more than once, the previous definition will be overridden`. The equivalent pair without Secret logs nothing, so the warning is not caused by the duplicate identifier alone: the existing object (bindings already assigned) is compared with the new one (bindings not yet assigned). The same comparison order also means the warning is emitted whether or not the Secret values differ.

Impact reasoning: with legacy naming, two equivalent Secret-backed Middlewares that flatten to the same name now warn on every configuration load that an existing definition "will be overridden", although nothing changes; equivalent definitions without Secrets stay silent. Noise only: the resulting configuration is correct.

## C8

Target: [evalon/traefik-se-01f93250](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-01f93250) at `fd68913525a9b6baf8fe2e0d3c1cda5504ccc937`.

Naming drift: `pkg/provider/kubernetes/ingress-nginx/build_test.go` and `Test_basicAuthUsers` do not exist on this branch.

```console
$ ls ~/wt/01f93250/pkg/provider/kubernetes/ingress-nginx/build_test.go
ls: cannot access '/home/ubuntu/wt/01f93250/pkg/provider/kubernetes/ingress-nginx/build_test.go': No such file or directory
$ grep -rn "func Test_basicAuthUsers" ~/wt/01f93250/pkg        # no output
$ grep -rln "auth-map" ~/wt/01f93250/pkg/provider/kubernetes/ingress-nginx/
pkg/provider/kubernetes/ingress-nginx/secrets_test.go
pkg/provider/kubernetes/ingress-nginx/build.go
```

The only committed auth-map test is `TestAuthSecretReferences` (`secrets_test.go`); its auth-map Secret has a single key, and its reload assertion compares two loads of that single-user map:

```go
if secretType == "auth-map" {
    key = "alice"
    value = "secret-hash"
}
secret := &corev1.Secret{..., Data: map[string][]byte{key: []byte(value)}}
...
first := load()
assert.Equal(t, first, load())
```

The generation under test, `resolveBasicAuth` in `build.go`, sorts the Secret keys (`slices.Sort(keys)`) before emitting one user reference per key.

Demonstration: `verify/repro/c8_order_mutation.sh` prints the user order generated for a three-key Secret over 200 loads (`verify/repro/c8_probe_test.go`), then applies two order-only mutations — reversed order, and no sort (map iteration order, which changes between loads) — and runs the committed tests against each.

```sh
verify/repro/c8_order_mutation.sh ~/wt/01f93250
```

```console
##### UNMODIFIED
C8 distinct user orders over 200 loads: 1
C8   200x [urn:k8s:secret:credentials:alice urn:k8s:secret:credentials:bob urn:k8s:secret:credentials:carol]
--- PASS: TestVerifyC8 (0.10s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.113s
--- committed auth tests: go test -race -count=1 -run 'TestAuthSecretReferences|TestLoadIngresses|Test_basicAuthUsers' -v ./pkg/provider/kubernetes/ingress-nginx/
--- PASS: TestLoadIngresses (0.03s)
--- PASS: TestAuthSecretReferences (0.06s)
    --- PASS: TestAuthSecretReferences/auth-file/basic (0.01s)
    --- PASS: TestAuthSecretReferences/auth-file/digest (0.02s)
    --- PASS: TestAuthSecretReferences/auth-map/basic (0.02s)
    --- PASS: TestAuthSecretReferences/auth-map/digest (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	2.326s
--- whole package: go test -count=1 ./pkg/provider/kubernetes/ingress-nginx/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.512s
diff --git a/pkg/provider/kubernetes/ingress-nginx/build.go b/pkg/provider/kubernetes/ingress-nginx/build.go
index 7aa935f4c..47aa54aee 100644
--- a/pkg/provider/kubernetes/ingress-nginx/build.go
+++ b/pkg/provider/kubernetes/ingress-nginx/build.go
@@ -1084,6 +1084,7 @@ func (p *Provider) resolveBasicAuth(namespace string, cfg IngressConfig) (*dynam
 			keys = append(keys, key)
 		}
 		slices.Sort(keys)
+		slices.Reverse(keys)
 		for _, key := range keys {
 			value := string(secret.Data[key])
 			ref := secrets.Add(secretName, key, value)
##### MUTATION 1: reversed order
 pkg/provider/kubernetes/ingress-nginx/build.go | 1 +
 1 file changed, 1 insertion(+)
C8 distinct user orders over 200 loads: 1
C8   200x [urn:k8s:secret:credentials:carol urn:k8s:secret:credentials:bob urn:k8s:secret:credentials:alice]
--- PASS: TestVerifyC8 (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.103s
--- committed auth tests: go test -race -count=1 -run 'TestAuthSecretReferences|TestLoadIngresses|Test_basicAuthUsers' -v ./pkg/provider/kubernetes/ingress-nginx/
--- PASS: TestLoadIngresses (0.06s)
--- PASS: TestAuthSecretReferences (0.09s)
    --- PASS: TestAuthSecretReferences/auth-file/basic (0.02s)
    --- PASS: TestAuthSecretReferences/auth-file/digest (0.03s)
    --- PASS: TestAuthSecretReferences/auth-map/basic (0.02s)
    --- PASS: TestAuthSecretReferences/auth-map/digest (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	2.434s
--- whole package: go test -count=1 ./pkg/provider/kubernetes/ingress-nginx/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.542s
diff --git a/pkg/provider/kubernetes/ingress-nginx/build.go b/pkg/provider/kubernetes/ingress-nginx/build.go
index 7aa935f4c..2ace2b676 100644
--- a/pkg/provider/kubernetes/ingress-nginx/build.go
+++ b/pkg/provider/kubernetes/ingress-nginx/build.go
@@ -1083,7 +1083,7 @@ func (p *Provider) resolveBasicAuth(namespace string, cfg IngressConfig) (*dynam
 		for key := range secret.Data {
 			keys = append(keys, key)
 		}
-		slices.Sort(keys)
+		_ = slices.Sort[[]string]
 		for _, key := range keys {
 			value := string(secret.Data[key])
 			ref := secrets.Add(secretName, key, value)
##### MUTATION 2: unsorted (map iteration order, changes between loads)
 pkg/provider/kubernetes/ingress-nginx/build.go | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)
C8 distinct user orders over 200 loads: 3
C8    28x [urn:k8s:secret:credentials:bob urn:k8s:secret:credentials:carol urn:k8s:secret:credentials:alice]
C8    23x [urn:k8s:secret:credentials:alice urn:k8s:secret:credentials:bob urn:k8s:secret:credentials:carol]
C8   149x [urn:k8s:secret:credentials:carol urn:k8s:secret:credentials:alice urn:k8s:secret:credentials:bob]
--- PASS: TestVerifyC8 (0.09s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.106s
--- committed auth tests: go test -race -count=1 -run 'TestAuthSecretReferences|TestLoadIngresses|Test_basicAuthUsers' -v ./pkg/provider/kubernetes/ingress-nginx/
--- PASS: TestLoadIngresses (0.03s)
--- PASS: TestAuthSecretReferences (0.06s)
    --- PASS: TestAuthSecretReferences/auth-file/basic (0.01s)
    --- PASS: TestAuthSecretReferences/auth-file/digest (0.02s)
    --- PASS: TestAuthSecretReferences/auth-map/basic (0.02s)
    --- PASS: TestAuthSecretReferences/auth-map/digest (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	2.259s
--- whole package: go test -count=1 ./pkg/provider/kubernetes/ingress-nginx/...
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.534s
```

Observed: both mutations keep the same users and change their sequence (reversed; three distinct orders over 200 loads), and the committed tests — `TestAuthSecretReferences`, `TestLoadIngresses`, and the whole `ingress-nginx` package — still pass.

Impact reasoning: the branch currently sorts, so behavior is correct today; but a change that loses the sort makes the generated middleware configuration differ between otherwise identical loads (observed under mutation 2), and nothing in the committed tests would catch it.
