# Evidence for audit run v-3d0d5f5e

Observations only. Every command below was run on go1.26.3 linux/amd64. Claim-target branches were checked out in separate
worktrees (`~/wt/<hash>`); production code was never modified there, only `*_test.go` audit files were added. The probe
files are committed under `verify/probes/<hash>/`, the repros of the substantiated problems under `verify/repro/`.

Two words used throughout:

- *fixture-style build*: the eval fixtures build middlewares from `runtime.NewConfig(*conf)` on the raw provider
  configuration, so middleware names are unqualified (`default-basicauth`).
- *delivered path*: provider loader -> real `server.ConfigurationWatcher` (which qualifies names, `default-basicauth@kubernetescrd`)
  -> `runtime.NewConfig` -> `middleware.NewBuilder(...).BuildMiddlewareChain`. This is what a running Traefik does.

## C1

Target: [evalon/traefik-se-04dfad87](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-04dfad87) at `570fc08bfd2dfae3d32171351c89613389b49fca`. Verdict: **REFUTED** (execution: refuted; isolation: refuted).

### Setup and commands

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-04dfad87
git worktree add --detach ~/wt/04dfad87 FETCH_HEAD   # 570fc08bfd2dfae3d32171351c89613389b49fca
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/04dfad87/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/04dfad87/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/04dfad87/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/04dfad87/pkg/server/
cp verify/probes/04dfad87/verify_c1_probe_test.go ~/wt/04dfad87/pkg/provider/kubernetes/crd/
cd ~/wt/04dfad87
# 1. the fixture tests named by the claim, one at a time
for t in PluginSuccessExecution PluginTypedSecretField PluginNestedExecution PluginEscapedKeyExecution PluginInputMutationIsolation ConcurrentBuilds_RaceDetector; do
  go test -v -race -count=1 -timeout=5m -run "^TestEval_ProviderToBuilder_${t}\$" ./pkg/provider/kubernetes/crd
done
# 2. the same shapes through the delivered path with the real plugins.Builder (Yaegi)
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C1' ./pkg/provider/kubernetes/crd
# 3. where the fixture panic comes from
git show af6d927c5:pkg/server/middleware/middlewares.go | grep -n 'pluginBuilder).IsNil'
grep -n 'pluginBuilder).IsNil' pkg/server/middleware/middlewares.go
grep -n 'Build(pName string' pkg/server/middleware/plugins.go pkg/plugins/builder.go
```

### Fixture tests: all six exit 1 with the same panic, before any plugin is built

```console
--- FAIL: TestEval_ProviderToBuilder_PluginSuccessExecution (0.51s)
panic: reflect: call of reflect.Value.IsNil on zero Value [recovered, repanicked]
	/home/ubuntu/wt/04dfad87/pkg/server/middleware/middlewares.go:427 +0x4a31
	/home/ubuntu/wt/04dfad87/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:713 +0x2fa
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.603s
FAIL
```

The six logs contain this panic count (one or more per test):

```console
c1-fixture-ConcurrentBuilds_RaceDetector.log:2
c1-fixture-PluginEscapedKeyExecution.log:1
c1-fixture-PluginInputMutationIsolation.log:1
c1-fixture-PluginNestedExecution.log:1
c1-fixture-PluginSuccessExecution.log:1
c1-fixture-PluginTypedSecretField.log:1
```

### Why the fixture panics: the branch changed the plugin builder signature, the fixture mock no longer satisfies it

```console
$ git show af6d927c5:pkg/server/middleware/middlewares.go | grep -n ...
408:	if config.Plugin != nil && !reflect.ValueOf(b.pluginBuilder).IsNil() { // Using "reflect" because "b.pluginBuilder" is an interface.
$ grep -n ... pkg/server/middleware/middlewares.go
427:	if config.Plugin != nil && !reflect.ValueOf(b.pluginBuilder).IsNil() { // Using "reflect" because "b.pluginBuilder" is an interface.
$ grep -n ... plugins.go builder.go
pkg/server/middleware/plugins.go:17:	Build(pName string, config map[string]any, secrets dynamic.Secrets, middlewareName string) (plugins.Constructor, error)
pkg/plugins/builder.go:114:func (b Builder) Build(pName string, config map[string]any, secrets dynamic.Secrets, middlewareName string) (Constructor, error) {
```

The fixture mock implements `Build(pName string, config map[string]any, middlewareName string)` (three arguments). The fixture
helper `newEvalMiddlewareBuilder` passes the mock to `middleware.NewBuilder` only when it is assignable to the parameter
type; on this branch it is not, so the builder receives a nil interface and the `reflect.ValueOf(b.pluginBuilder).IsNil()`
guard panics. That guard is unchanged from the base commit `af6d927c5` (line 408 there), and `cmd/traefik` always passes a
`*plugins.Builder`. The fixture therefore never reaches the plugin path on this branch: its failure says nothing about
how Secret-backed plugin configuration is resolved.

### Delivered path with the real Yaegi plugin builder: exit 0, no race report

The probe writes two local Yaegi plugins to a temporary `plugins-local` directory and builds them with
`plugins.NewBuilder`: `verifygeneric` (`type Config map[string]interface{}`; it snapshots what it received, then overwrites
every string it can reach with `MUTATED`) and `verifytyped` (a struct with `int`, `float64`, `bool`, nested struct and
`[]string` fields). The CRD provider configuration is delivered through a real `ConfigurationWatcher`, built twice, then
built 20 times concurrently while 20 goroutines `json.Marshal` the shared runtime configuration.

```console
    verify_c1_probe_test.go:302: declared (provider) : {"secret":"urn:k8s:secret:name:key","user":"admin"}
    verify_c1_probe_test.go:303: received by plugin  : {"secret":"this_is_the_secret","user":"admin"}
    verify_c1_probe_test.go:340: shared after builds : {"secret":"urn:k8s:secret:name:key","user":"admin"}
    verify_c1_probe_test.go:341: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c1_probe_test.go:302: declared (provider) : {"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}
    verify_c1_probe_test.go:303: received by plugin  : {"secret_0":{"secret_1":{"secret_2":{"secret":"this_is_the_very_deep_secret","user":"admin"}}}}
    verify_c1_probe_test.go:340: shared after builds : {"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}
    verify_c1_probe_test.go:341: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c1_probe_test.go:302: declared (provider) : {"secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}
    verify_c1_probe_test.go:303: received by plugin  : {"secret":["secret_data1","secret_data2"]}
    verify_c1_probe_test.go:340: shared after builds : {"secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}
    verify_c1_probe_test.go:341: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c1_probe_test.go:302: declared (provider) : {"users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}
    verify_c1_probe_test.go:303: received by plugin  : {"users":[{"name":"admin","secret":"admin_password"},{"name":"user","secret":"user_password"}]}
    verify_c1_probe_test.go:340: shared after builds : {"users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}
    verify_c1_probe_test.go:341: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c1_probe_test.go:302: declared (provider) : {"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"urn:k8s:secret:evalkeys:slash","m~n":"urn:k8s:secret:evalkeys:tilde","x/y":["literal-0",{"~0":"urn:k8s:secret:evalkeys:listed"}],"~1":"urn:k8s:secret:evalkeys:escaped"}
    verify_c1_probe_test.go:303: received by plugin  : {"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"qx_slash_value","m~n":"qx_tilde_value","x/y":["literal-0",{"~0":"qx_listed_value"}],"~1":"qx_escaped_value"}
    verify_c1_probe_test.go:340: shared after builds : {"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"urn:k8s:secret:evalkeys:slash","m~n":"urn:k8s:secret:evalkeys:tilde","x/y":["literal-0",{"~0":"urn:k8s:secret:evalkeys:listed"}],"~1":"urn:k8s:secret:evalkeys:escaped"}
    verify_c1_probe_test.go:341: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c1_probe_test.go:302: declared (provider) : {"flag":"urn:k8s:secret:evalkeys:bool","hosts":["literal.example.com","urn:k8s:secret:evalkeys:host"],"inner":{"enabled":"urn:k8s:secret:evalkeys:bool","port":"urn:k8s:secret:evalkeys:int","token":"urn:k8s:secret:evalkeys:token"},"ratio":"urn:k8s:secret:evalkeys:float","secret":"urn:k8s:secret:evalkeys:int","user":"admin"}
    verify_c1_probe_test.go:303: received by plugin  : {"User":"admin","Secret":42,"Ratio":0.25,"Flag":true,"Inner":{"Token":"qx_token_value","Port":42,"Enabled":true},"Hosts":["literal.example.com","qx.example.com"]}
    verify_c1_probe_test.go:340: shared after builds : {"flag":"urn:k8s:secret:evalkeys:bool","hosts":["literal.example.com","urn:k8s:secret:evalkeys:host"],"inner":{"enabled":"urn:k8s:secret:evalkeys:bool","port":"urn:k8s:secret:evalkeys:int","token":"urn:k8s:secret:evalkeys:token"},"ratio":"urn:k8s:secret:evalkeys:float","secret":"urn:k8s:secret:evalkeys:int","user":"admin"}
    verify_c1_probe_test.go:341: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
--- PASS: TestVerify_C1_PluginExecutionAndIsolation (0.67s)
    --- PASS: TestVerify_C1_PluginExecutionAndIsolation/flat (0.53s)
    --- PASS: TestVerify_C1_PluginExecutionAndIsolation/nested_deep_map (0.02s)
    --- PASS: TestVerify_C1_PluginExecutionAndIsolation/nested_list (0.02s)
    --- PASS: TestVerify_C1_PluginExecutionAndIsolation/nested_list_of_maps (0.02s)
    --- PASS: TestVerify_C1_PluginExecutionAndIsolation/escaped_keys (0.02s)
    --- PASS: TestVerify_C1_PluginExecutionAndIsolation/typed (0.02s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	1.822s
```

```console
$ grep -c "DATA RACE" ~/logs/probe-04df-c1.log
0
```

- *execution*: for flat, typed, nested (deep map, list, list of maps) and escaped-key (`a/b`, `m~n`, `~1`, `/`, `x/y` with
  a nested `~0`) fields, the plugin constructor ran and received the Secret values (`received by plugin` lines); typed fields
  decoded to `42`, `0.25`, `true`.
- *isolation*: after the plugin overwrote its input, the shared provider and runtime configurations, and the bindings, were
  byte-identical to before (`shared config unchanged=true, bindings unchanged=true, contains MUTATED=false`); 20 concurrent
  builds all received the resolved values (`mismatches=0`) and the race detector reported nothing.

## C2

Verdict per branch: [evalon/traefik-se-fc2341ab](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-fc2341ab): **CONFIRMED**; [evalon/traefik-se-29c47f2b](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-29c47f2b): **CONFIRMED**.

### Branch evalon/traefik-se-fc2341ab (`62c77e7e82fd2402f651df8e84320fbe85b3c318`)

#### What the modified provider tests assert now

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-fc2341ab
git worktree add --detach ~/wt/fc2341ab FETCH_HEAD   # 62c77e7e82fd2402f651df8e84320fbe85b3c318
cd ~/wt/fc2341ab
git diff af6d927c5 HEAD -- pkg/provider/kubernetes/crd/kubernetes_test.go
grep -n 'pluginFixtures' pkg/provider/kubernetes/crd/secrets_test.go
grep -rn 'deep_read_secret\|array_of_secret\|array_of_map' pkg --include='*_test.go'
grep -rln 'this_is_the_very_deep_secret\|secret_data1\|admin_password' pkg --include='*_test.go'   # prints nothing
```

Hunks of `TestLoadIngressRoutes` for the nested, array and list-of-map fixtures (verbatim from the diff):

```diff
@@ -7464,12 +7469,13 @@ func TestLoadIngressRoutes(t *testing.T) {
 										"secret_1": map[string]any{
 											"secret_2": map[string]any{
 												"user":   "admin",
-												"secret": "this_is_the_very_deep_secret",
+												"secret": "urn:k8s:secret:name:key",
 											},
 										},
 									},
 								},
 							},
+							Secrets: &dynamic.MiddlewareSecrets{Namespace: "default"},
 						},
 					},
 					Services:          map[string]*dynamic.Service{},
@@ -7498,9 +7504,10 @@ func TestLoadIngressRoutes(t *testing.T) {
 						"default-test-secret": {
 							Plugin: map[string]dynamic.PluginConf{
 								"test-secret": map[string]any{
-									"secret": []any{"secret_data1", "secret_data2"},
+									"secret": []any{"urn:k8s:secret:name:key1", "urn:k8s:secret:name:key2"},
 								},
 							},
+							Secrets: &dynamic.MiddlewareSecrets{Namespace: "default"},
 						},
 					},
 					Services:          map[string]*dynamic.Service{},
@@ -7532,15 +7539,16 @@ func TestLoadIngressRoutes(t *testing.T) {
 									"users": []any{
 										map[string]any{
 											"name":   "admin",
-											"secret": "admin_password",
+											"secret": "urn:k8s:secret:name:key1",
 										},
 										map[string]any{
 											"name":   "user",
-											"secret": "user_password",
+											"secret": "urn:k8s:secret:name:key2",
 										},
 									},
 								},
 							},
+							Secrets: &dynamic.MiddlewareSecrets{Namespace: "default"},
 						},
 					},
 					Services:          map[string]*dynamic.Service{},
@@ -10212,11 +10220,25 @@ func TestLoadIngressRoutes(t *testing.T) {
 			require.NoError(t, p.Init())
 
 			conf := p.loadConfigurationFromCRD(t.Context(), client)
+			clearSecretsVersions(t, conf)
 			assert.Equal(t, test.expected, conf)
 		})
 	}
 }
 
+// clearSecretsVersions checks that the middlewares with secret references have a version,
+// and clears it because it is randomized for each run.
+func clearSecretsVersions(t *testing.T, conf *dynamic.Configuration) {
+	t.Helper()
+
+	for _, middleware := range conf.HTTP.Middlewares {
+		if middleware.Secrets != nil {
+			assert.NotEmpty(t, middleware.Secrets.Version)
+			middleware.Secrets.Version = ""
+		}
+	}
+}
+
 func TestLoadIngressRoutes_multipleEndpointAddresses(t *testing.T) {
 	wantConf := &dynamic.Configuration{
 		UDP: &dynamic.UDPConfiguration{
```

```console
$ grep -n pluginFixtures pkg/provider/kubernetes/crd/secrets_test.go
18:	pluginFixtures    = []string{"services.yml", "with_plugin_read_secret.yml"}
83:			paths:       pluginFixtures,
155:			paths:      pluginFixtures,
210:		{desc: "plugin token", paths: pluginFixtures, secret: "name", middleware: "default-test-secret"},
244:	p, _ := loadSecretsConfiguration(t, pluginFixtures, nil)
$ grep -rn 'deep_read_secret\|array_of_secret\|array_of_map' pkg --include='*_test.go'
pkg/provider/kubernetes/crd/kubernetes_test.go:7449:			paths: []string{"services.yml", "with_plugin_deep_read_secret.yml"},
pkg/provider/kubernetes/crd/kubernetes_test.go:7488:			paths: []string{"services.yml", "with_plugin_read_array_of_secret.yml"},
pkg/provider/kubernetes/crd/kubernetes_test.go:7520:			paths: []string{"services.yml", "with_plugin_read_array_of_map_contain_secret.yml"},
```

So for those three shapes the previous assertions on the resolved credentials (`this_is_the_very_deep_secret`,
`secret_data1`/`secret_data2`, `admin_password`/`user_password`) became assertions on the URN strings plus
`Secrets: &dynamic.MiddlewareSecrets{Namespace: "default"}` (whose `Version` is only checked to be non-empty, then cleared).
The branch's only provider-to-resolution test, `TestLoadConfigurationFromCRD_secretsResolution`, uses `pluginFixtures` =
`with_plugin_read_secret.yml`, the flat shape. The unit test `TestMiddleware_ResolveSecrets/plugin_values` resolves a
synthetic list of strings and a one-level map with a fake resolver; no test resolves a map inside a list or a map nested
more than one level.

#### Demonstration: breaking the resolution of those shapes fails no test of the branch

The mutations (committed as `verify/repro/C2_fc2341ab/*.patch`) change `secretsResolution.value` in
`pkg/config/dynamic/secrets.go` so that, when a middleware is built (`!r.collecting`), references are left unresolved for
one shape. They were applied in a scratch worktree, never to the claim worktree.

```sh
REMOTE=sol WT=~/scratch/c2a sh verify/repro/C2_fc2341ab_unasserted_plugin_resolution.sh
```

```console
From https://git-manager.itsdev.in/proxy/github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
 * branch                evalon/traefik-se-fc2341ab -> FETCH_HEAD
Preparing worktree (detached HEAD 62c77e7e8)
HEAD is now at 62c77e7e8 chore: apply eval changes
== the values the provider tests asserted before the change, in the branch's test files:
(none)
== unmutated
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.017s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.423s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
ok  	github.com/traefik/traefik/v3/pkg/plugins	0.136s
== M4-list-of-map-not-resolved-at-build
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.036s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.434s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.031s
ok  	github.com/traefik/traefik/v3/pkg/plugins	0.135s
M4-list-of-map-not-resolved-at-build: SURVIVED
== M5-deep-nested-not-resolved-at-build
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.018s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.404s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.047s
ok  	github.com/traefik/traefik/v3/pkg/plugins	0.166s
M5-deep-nested-not-resolved-at-build: SURVIVED
== M6-array-not-resolved-at-build
--- FAIL: TestMiddleware_ResolveSecrets (0.00s)
    --- FAIL: TestMiddleware_ResolveSecrets/plugin_values (0.00s)
        secrets_test.go:326: 
            	Error Trace:	/home/ubuntu/scratch/c2a/pkg/config/dynamic/secrets_test.go:326
            	Error:      	Not equal: 
            	            	expected: &dynamic.Middleware{AddPrefix:(*dynamic.AddPrefix)(nil), StripPrefix:(*dynamic.StripPrefix)(nil), StripPrefixRegex:(*dynamic.StripPrefixRegex)(nil), ReplacePath:(*dynamic.ReplacePath)(nil), ReplacePathRegex:(*dynamic.ReplacePathRegex)(nil), Chain:(*dynamic.Chain)(nil), IPWhiteList:(*dynamic.IPWhiteList)(nil), IPAllowList:(*dynamic.IPAllowList)(nil), Headers:(*dynamic.Headers)(nil), EncodedCharacters:(*dynamic.EncodedCharacters)(nil), Errors:(*dynamic.ErrorPage)(nil), RateLimit:(*dynamic.RateLimit)(nil), RedirectRegex:(*dynamic.RedirectRegex)(nil), RedirectScheme:(*dynamic.RedirectScheme)(nil), BasicAuth:(*dynamic.BasicAuth)(nil), DigestAuth:(*dynamic.DigestAuth)(nil), ForwardAuth:(*dynamic.ForwardAuth)(nil), InFlightReq:(*dynamic.InFlightReq)(nil), Buffering:(*dynamic.Buffering)(nil), CircuitBreaker:(*dynamic.CircuitBreaker)(nil), Compress:(*dynamic.Compress)(nil), PassTLSClientCert:(*dynamic.PassTLSClientCert)(nil), Retry:(*dynamic.Retry)(nil), ContentType:(*dynamic.ContentType)(nil), GrpcWeb:(*dynamic.GrpcWeb)(nil), Plugin:map[string]dynamic.PluginConf{"plug":dynamic.PluginConf{"list":[]interface {}{"other-token", "value"}, "literal":"value", "nested":map[string]interface {}{"deep":"plugin-token", "number":1}, "token":"plugin-token"}}, Secrets:(*dynamic.MiddlewareSecrets)(0x3d9f67aa0920), RequestHeaderModifier:(*dynamic.HeaderModifier)(nil), ResponseHeaderModifier:(*dynamic.HeaderModifier)(nil), RequestRedirect:(*dynamic.RequestRedirect)(nil), URLRewrite:(*dynamic.URLRewrite)(nil), AuthTLSPassCertificateToUpstream:(*dynamic.AuthTLSPassCertificateToUpstream)(nil), Snippet:(*dynamic.Snippet)(nil), RewriteTarget:(*dynamic.RewriteTarget)(nil), UpstreamVHost:(*dynamic.UpstreamVHost)(nil), AppRoot:(*dynamic.AppRoot)(nil)}
            	            	actual  : &dynamic.Middleware{AddPrefix:(*dynamic.AddPrefix)(nil), StripPrefix:(*dynamic.StripPrefix)(nil), StripPrefixRegex:(*dynamic.StripPrefixRegex)(nil), ReplacePath:(*dynamic.ReplacePath)(nil), ReplacePathRegex:(*dynamic.ReplacePathRegex)(nil), Chain:(*dynamic.Chain)(nil), IPWhiteList:(*dynamic.IPWhiteList)(nil), IPAllowList:(*dynamic.IPAllowList)(nil), Headers:(*dynamic.Headers)(nil), EncodedCharacters:(*dynamic.EncodedCharacters)(nil), Errors:(*dynamic.ErrorPage)(nil), RateLimit:(*dynamic.RateLimit)(nil), RedirectRegex:(*dynamic.RedirectRegex)(nil), RedirectScheme:(*dynamic.RedirectScheme)(nil), BasicAuth:(*dynamic.BasicAuth)(nil), DigestAuth:(*dynamic.DigestAuth)(nil), ForwardAuth:(*dynamic.ForwardAuth)(nil), InFlightReq:(*dynamic.InFlightReq)(nil), Buffering:(*dynamic.Buffering)(nil), CircuitBreaker:(*dynamic.CircuitBreaker)(nil), Compress:(*dynamic.Compress)(nil), PassTLSClientCert:(*dynamic.PassTLSClientCert)(nil), Retry:(*dynamic.Retry)(nil), ContentType:(*dynamic.ContentType)(nil), GrpcWeb:(*dynamic.GrpcWeb)(nil), Plugin:map[string]dynamic.PluginConf{"plug":dynamic.PluginConf{"list":[]interface {}{"urn:k8s:secret:plugin:other", "value"}, "literal":"value", "nested":map[string]interface {}{"deep":"plugin-token", "number":1}, "token":"plugin-token"}}, Secrets:(*dynamic.MiddlewareSecrets)(0x3d9f67aa0c40), RequestHeaderModifier:(*dynamic.HeaderModifier)(nil), ResponseHeaderModifier:(*dynamic.HeaderModifier)(nil), RequestRedirect:(*dynamic.RequestRedirect)(nil), URLRewrite:(*dynamic.URLRewrite)(nil), AuthTLSPassCertificateToUpstream:(*dynamic.AuthTLSPassCertificateToUpstream)(nil), Snippet:(*dynamic.Snippet)(nil), RewriteTarget:(*dynamic.RewriteTarget)(nil), UpstreamVHost:(*dynamic.UpstreamVHost)(nil), AppRoot:(*dynamic.AppRoot)(nil)}
            	            	
            	            	Diff:
            	            	--- Expected
            	            	+++ Actual
            	            	@@ -29,3 +29,3 @@
            	            	    (string) (len=4) "list": ([]interface {}) (len=2) {
            	            	-    (string) (len=11) "other-token",
            	            	+    (string) (len=27) "urn:k8s:secret:plugin:other",
            	            	     (string) (len=5) "value"
            	Test:       	TestMiddleware_ResolveSecrets/plugin_values
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/config/dynamic	0.035s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.470s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.026s
ok  	github.com/traefik/traefik/v3/pkg/plugins	0.143s
FAIL
M6-array-not-resolved-at-build: KILLED
exit=0
```

- M4 (maps inside lists, the list-of-map shape, are not resolved at build time): all four packages `ok`.
- M5 (maps nested deeper than one level, the deep-nested shape, are not resolved at build time): all four packages `ok`.
- M6 (control: lists are not resolved at build time): killed by `TestMiddleware_ResolveSecrets/plugin_values`, so the array
  shape does have a unit-level resolution assertion, and the procedure does detect asserted shapes.

Earlier variants M1-M3 (`verify/probes/fc2341ab/mutations/`), which skip the same shapes unconditionally, also break the
provider's reference collection (`Middleware.SecretReferences` shares the walker), and are caught by `TestLoadIngressRoutes`
only through `Secrets` becoming nil:

```console
$ go test -count=1 -timeout=10m -run 'TestLoadIngressRoutes$' ./pkg/provider/kubernetes/crd   # with M1 applied
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.00s)
            	            	-    Secrets: (*dynamic.MiddlewareSecrets)({
            	            	-     Namespace: (string) (len=7) "default",
            	            	-     Version: (string) ""
            	            	+    Secrets: (*dynamic.MiddlewareSecrets)(<nil>),
```

That is a check that a reference was found in the shape, not that it resolves to the Secret value.

**Impact reasoning.** Before the change, the provider tests proved that a plugin reference nested in a deep map or in a
list of maps ends up as the Secret value handed to the plugin. After the change the value is produced at build time, and
for these two shapes no test of the branch asserts it: a regression that leaves such references unresolved (the plugin
would receive the literal `urn:k8s:secret:...` string as its password or token) passes the whole suite. The array shape
kept an equivalent (unit-level) assertion. The gap is in test coverage, not in the current production behavior.

### Branch evalon/traefik-se-29c47f2b (`c3cfe565d99f305116af3bf9587315af250ef2d2`)

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-29c47f2b
git worktree add --detach ~/wt/29c47f2b FETCH_HEAD   # c3cfe565d99f305116af3bf9587315af250ef2d2
cp verify/probes/29c47f2b/verify_c2_probe_test.go ~/wt/29c47f2b/pkg/server/middleware/
cd ~/wt/29c47f2b
go test -v -count=1 -timeout=200s -run '^TestVerify_C2' ./pkg/server/middleware
go test -v -count=1 -run '^TestSecrets_RedactError$/empty_value' ./pkg/config/dynamic
sed -n 215,220p pkg/config/dynamic/secrets.go
```

```console
    verify_c2_probe_test.go:41: non-empty Secret: error="token \"urn:k8s:secret:plugin:token\" is revoked" recorded=["token \"urn:k8s:secret:plugin:token\" is revoked"]
    verify_c2_probe_test.go:48: RedactError(Secret value "s3cr3t-token"): original="token \"s3cr3t-token\" is revoked" diagnostic="token \"urn:k8s:secret:plugin:token\" is revoked" contains URN=true
    verify_c2_probe_test.go:48: RedactError(Secret value ""): original="token \"\" is revoked" diagnostic="token \"\" is revoked" contains URN=false
    verify_c2_probe_test.go:53: empty Secret    : error="plugin: token  is too short" recorded=["plugin: token  is too short"] contains URN=false
    verify_c2_probe_test.go:62: pkg/config/dynamic/secrets_test.go:180: 		{
    verify_c2_probe_test.go:62: pkg/config/dynamic/secrets_test.go:181: 			desc:     "empty value",
    verify_c2_probe_test.go:62: pkg/config/dynamic/secrets_test.go:182: 			secrets:  Secrets{"urn:k8s:secret:plugin:token": ""},
    verify_c2_probe_test.go:62: pkg/config/dynamic/secrets_test.go:183: 			err:      errors.New(`invalid value ""`),
    verify_c2_probe_test.go:62: pkg/config/dynamic/secrets_test.go:184: 			expected: `invalid value ""`,
    verify_c2_probe_test.go:62: pkg/config/dynamic/secrets_test.go:185: 		},
--- PASS: TestVerify_C2_EmptySecretPluginErrorDiagnostic (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.027s
```

```console
$ go test -v -count=1 -run '^TestSecrets_RedactError$/empty_value' ./pkg/config/dynamic
--- PASS: TestSecrets_RedactError (0.00s)
    --- PASS: TestSecrets_RedactError/empty_value (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.009s
$ sed -n 215,220p pkg/config/dynamic/secrets.go
func (s Secrets) redact(msg string) string {
	var replacements []replacement
	for ref, value := range s {
		if value == "" {
			continue
		}
```

The branch's regression test `TestSecrets_RedactError/empty_value` (`pkg/config/dynamic/secrets_test.go:180-185`) feeds an
error that quotes the resolved value of `urn:k8s:secret:plugin:token` (the empty string, `invalid value ""`) and requires
the diagnostic to stay `invalid value ""`, i.e. without the URN. The test passes because `Secrets.redact` skips empty
values. For a non-empty value the same error shape is required to carry the URN (`token "urn:k8s:secret:plugin:token" is
revoked`). Through the builder, an empty Secret produces `plugin: token  is too short`, recorded on the middleware, with no
reference either. No other test of the branch covers a plugin error with an empty Secret value
(`TestBuilder_secretValues_pluginErrors` uses non-empty values only; `TestSecrets_Revealed` holds an empty entry and never
expects it).

**Impact reasoning.** An operator whose plugin rejects an empty Secret value gets a diagnostic that does not say which
Secret key is at fault (`invalid value ""`, `token  is too short`), and the regression suite pins that outcome instead of
requiring the reference. Nothing confidential leaks (the value is empty); the cost is diagnosability, for a situation as
ordinary as a Secret key created empty.

## C3

Target: [evalon/traefik-se-fc2341ab](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-fc2341ab) at `62c77e7e82fd2402f651df8e84320fbe85b3c318`. Verdict: **CONFIRMED**.

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-fc2341ab
git worktree add --detach ~/wt/fc2341ab FETCH_HEAD   # 62c77e7e82fd2402f651df8e84320fbe85b3c318
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/fc2341ab/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/fc2341ab/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/fc2341ab/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/fc2341ab/pkg/server/
cp verify/probes/fc2341ab/verify_c3_probe_test.go ~/wt/fc2341ab/pkg/provider/kubernetes/ingress-nginx/
cd ~/wt/fc2341ab
go test -v -count=1 -timeout=200s -run '^TestVerify_C3' ./pkg/provider/kubernetes/ingress-nginx
sed -n 1080,1091p pkg/provider/kubernetes/ingress-nginx/build.go
sed -n 1292,1302p pkg/provider/kubernetes/ingress-nginx/build.go
# fixture-free variant, in a clean worktree
REMOTE=sol WT=~/scratch/c3 sh verify/repro/C3_auth_map_keys_selected_twice.sh
```

```console
    verify_c3_probe_test.go:31: basicAuthUsers returned 3 distinct orderings over 50 calls, e.g.:
    verify_c3_probe_test.go:33:   loader selection: mia:{SHA}mmm zoe:{SHA}zzz adam:{SHA}aaa
    verify_c3_probe_test.go:33:   loader selection: zoe:{SHA}zzz adam:{SHA}aaa mia:{SHA}mmm
    verify_c3_probe_test.go:33:   loader selection: adam:{SHA}aaa mia:{SHA}mmm zoe:{SHA}zzz
    verify_c3_probe_test.go:40: resolveBasicAuth users for default-ingress-with-basicauth-map-rule-0-path-0-tls-basic-auth: ["other:urn:k8s:secret:basic-auth-map:other" "user:urn:k8s:secret:basic-auth-map:user"]
    verify_c3_probe_test.go:40: resolveBasicAuth users for default-ingress-with-basicauth-map-rule-0-path-0-basic-auth: ["other:urn:k8s:secret:basic-auth-map:other" "user:urn:k8s:secret:basic-auth-map:user"]
    verify_c3_probe_test.go:56: build.go:1081: if _, err := basicAuthUsers(secret, authSecretType); err != nil {
    verify_c3_probe_test.go:57: build.go:1088: for _, user := range slices.Sorted(maps.Keys(secret.Data)) {
--- PASS: TestVerify_C3_AuthMapKeysSelectedTwice (0.08s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.116s
```

```console
$ sed -n 1080,1091p pkg/provider/kubernetes/ingress-nginx/build.go
	// The users are only built to check the Secret content, the configuration holds references to them.
	if _, err := basicAuthUsers(secret, authSecretType); err != nil {
		return nil, nil, nil, fmt.Errorf("getting users from secret: %w", err)
	}

	var users []string
	if authSecretType == "auth-map" {
		// The key is the user name, and its value is the user hash.
		for _, user := range slices.Sorted(maps.Keys(secret.Data)) {
			users = append(users, user+":"+dynamic.NewSecretReference(secretName, user))
		}
	} else {
$ sed -n 1292,1302p pkg/provider/kubernetes/ingress-nginx/build.go
func basicAuthUsers(secret *corev1.Secret, authSecretType string) ([]string, error) {
	var users []string
	if authSecretType == "auth-map" {
		if len(secret.Data) == 0 {
			return nil, fmt.Errorf("secret %s/%s contains no user credentials", secret.Namespace, secret.Name)
		}
		for user, pass := range secret.Data {
			users = append(users, user+":"+string(pass))
		}
		return users, nil
	}
```

```console
$ REMOTE=sol WT=~/scratch/c3 sh verify/repro/C3_auth_map_keys_selected_twice.sh
    repro_c3_test.go:31: basicAuthUsers returned 3 distinct orderings over 200 calls
    repro_c3_test.go:49: resolveBasicAuth users: adam:urn:k8s:secret:basic-auth:adam mia:urn:k8s:secret:basic-auth:mia zoe:urn:k8s:secret:basic-auth:zoe
    repro_c3_test.go:68: build.go:1081: if _, err := basicAuthUsers(secret, authSecretType); err != nil {
    repro_c3_test.go:69: build.go:1088: for _, user := range slices.Sorted(maps.Keys(secret.Data)) {
--- PASS: TestRepro_C3_AuthMapKeysSelectedTwice (0.05s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.068s
```

Tracing an auth-map Secret (`zoe`, `adam`, `mia`): `basicAuthUsers` selects the entries by ranging over `secret.Data`
(three different orders over repeated calls); `resolveBasicAuth` calls it, discards the result (`if _, err := ...`), and then
enumerates `secret.Data` a second time (`slices.Sorted(maps.Keys(secret.Data))`) for the sole purpose of building
`user:urn:k8s:secret:<name>:<user>` entries. The URNs do not consume the loader's selection and the two selections do not
share an implementation.

**Impact reasoning.** Today both enumerations cover every key, so the published users are complete and deterministic; no
runtime failure was observed. The cost is structural: the validation path and the reference path can drift. Any rule added
to one selection only (skipping a key, trimming, rejecting an entry) would make the provider validate one set of users and
publish references for another, without a test noticing, because the loader's result is thrown away.

## C4

Target: [evalon/traefik-se-39ae2e45](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-39ae2e45) at `bbf8db92312bfdd44f613ebc6251e16f88edf8db`. Verdict: **REFUTED**.

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-39ae2e45
git worktree add --detach ~/wt/39ae2e45 FETCH_HEAD   # bbf8db92312bfdd44f613ebc6251e16f88edf8db
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/39ae2e45/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/39ae2e45/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/39ae2e45/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/39ae2e45/pkg/server/
cp verify/probes/39ae2e45/verify_c4_probe_test.go ~/wt/39ae2e45/pkg/provider/kubernetes/crd/
cd ~/wt/39ae2e45
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C4' ./pkg/provider/kubernetes/crd
go test -v -race -count=1 -timeout=5m -run '^TestEval_CRD_(BasicAndDigestAuth_SecretRetentionAndSerialization|TypedBasicAuth_SecretRetention|ProviderConfigurationRedactorCompatibility)$' ./pkg/provider/kubernetes/crd
```

The probe loads `with_auth.yml` (CRD `basicauth` and `digestauth` middlewares) with an `authsecret` Secret holding
distinct values (`qxbasicuser:$apr1$QxBasicSalt$QxBasicHashFragment000`, `qxdigestuser:qxrealm:0f1e2d3c4b5a69788796a5b4c3d2e1f0`)
and a typed `kubernetes.io/basic-auth` Secret (`qxtypeduser` / `qxtypedpassword`). It first requires that each middleware's
Secret carrier is populated (so the absence checks are not vacuous), then searches eight outputs for ten fragments: both
usernames, both hashes, the hash salt and digest, the digest realm of the entry, the typed username, the typed password and
its base64 SHA-1.

```console
    verify_c4_probe_test.go:64: default-basicauth: users=["urn:k8s:secret:authsecret:users"] carrier holds 1 value(s) (values not printed)
    verify_c4_probe_test.go:64: default-digestauth: users=["urn:k8s:secret:authsecret:users"] carrier holds 1 value(s) (values not printed)
    verify_c4_probe_test.go:64: default-typed-basicauth: users=["urn:k8s:secret:typed-auth-sec:username:{SHA}urn:k8s:secret:typed-auth-sec:password"] carrier holds 2 value(s) (values not printed)
    verify_c4_probe_test.go:116: 1 json.Marshal(provider configuration)               bytes=614   urn refs=6  {SHA} labels=1 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 2 json.MarshalIndent(middlewares)                    bytes=695   urn refs=6  {SHA} labels=1 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 3 yaml.Marshal(provider configuration)               bytes=918   urn refs=6  {SHA} labels=1 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 4 toml.Encode(middlewares)                           bytes=713   urn refs=6  {SHA} labels=1 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 5 redactor.RemoveCredentials(provider configuration) bytes=454   urn refs=1  {SHA} labels=0 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 6 redactor.Anonymize(provider configuration)         bytes=372   urn refs=0  {SHA} labels=0 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 7 json.Marshal(runtime configuration)                bytes=654   urn refs=6  {SHA} labels=1 leaked credential fragments=[]
    verify_c4_probe_test.go:116: 8 redactor.RemoveCredentials(runtime middlewares)    bytes=478   urn refs=1  {SHA} labels=0 leaked credential fragments=[]
    verify_c4_probe_test.go:134: fmt %v   of the middleware structs (Go formatting, outside the claim's outputs): 9 credential fragment kind(s) present
    verify_c4_probe_test.go:134: fmt %+v  of the middleware structs (Go formatting, outside the claim's outputs): 9 credential fragment kind(s) present
    verify_c4_probe_test.go:134: fmt %#v  of the middleware structs (Go formatting, outside the claim's outputs): 9 credential fragment kind(s) present
--- PASS: TestVerify_C4_SerializationAndRedaction (0.74s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	1.842s
```

Serialized middlewares (`json.MarshalIndent`), verbatim:

```json
{
 "default-basicauth": {
  "basicAuth": {
   "users": [
    "urn:k8s:secret:authsecret:users"
   ]
  }
 },
 "default-digestauth": {
  "digestAuth": {
   "users": [
    "urn:k8s:secret:authsecret:users"
   ]
  }
 },
 "default-forwardauth": {
  "forwardAuth": {
   "address": "test.com",
   "tls": {
    "ca": "-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----",
    "cert": "urn:k8s:secret:tlssecret:tls.crt",
    "key": "urn:k8s:secret:tlssecret:tls.key"
   },
   "headerField": "X-Header-Field",
   "maxBodySize": -1
  }
 },
 "default-typed-basicauth": {
  "basicAuth": {
   "users": [
    "urn:k8s:secret:typed-auth-sec:username:{SHA}urn:k8s:secret:typed-auth-sec:password"
   ]
  }
 }
}
```

Redactor output (middlewares part), verbatim:

```console
        "middlewares":{"default-basicauth":{"basicAuth":{"users":["xxxx"]}},"default-digestauth":{"digestAuth":{"users":["xxxx"]}},"default-forwardauth":{"forwardAuth":{"address":"test.com","tls":{"ca":"-----BEGIN CERTIFICATE-----\n-----END CERTIFICATE-----","cert":"urn:k8s:secret:tlssecret:tls.crt","key":"xxxx"},"headerField":"X-Header-Field","maxBodySize":-1}},"default-typed-basicauth":{"basicAuth":{"users":["xxxx"]}}}},"tcp":{},"udp":{},"tls":{}}
        "middlewares":{"default-basicauth":{"basicAuth":{"users":["xxxx"]}},"default-digestauth":{"digestAuth":{"users":["xxxx"]}},"default-forwardauth":{"forwardAuth":{"address":"xxxx","tls":{"ca":"xxxx","cert":"xxxx","key":"xxxx"},"headerField":"X-Header-Field","maxBodySize":-1}},"default-typed-basicauth":{"basicAuth":{"users":["xxxx"]}}}},"tcp":{},"udp":{},"tls":{}}
```

The three fixture tests on this branch:

```console
--- PASS: TestEval_CRD_ProviderConfigurationRedactorCompatibility (0.63s)
        	Error:      	"{\"http\":{\"middlewares\":{\"default-typed-basicauth\":{\"basicAuth\":{\"users\":[\"urn:k8s:secret:typed-auth-sec:username:{SHA}urn:k8s:secret:typed-auth-sec:password\"]}}}},\"tcp\":{},\"udp\":{},\"tls\":{}}" should not contain "{SHA}"
        	Messages:   	JSON serialization must not leak password hash
--- FAIL: TestEval_CRD_TypedBasicAuth_SecretRetention (0.02s)
--- PASS: TestEval_CRD_BasicAndDigestAuth_SecretRetentionAndSerialization (0.03s)
```

- JSON, YAML and TOML serialization of the provider configuration, JSON of the runtime configuration, and both redactors
  (`RemoveCredentials`, `Anonymize`) contain no credential fragment (`leaked credential fragments=[]` on all eight lines).
  Users appear as `urn:k8s:secret:authsecret:users`; redactors replace them with `xxxx`.
- The typed basic-auth entry is `urn:k8s:secret:typed-auth-sec:username:{SHA}urn:k8s:secret:typed-auth-sec:password`: two
  references around the fixed `{SHA}` algorithm label. The only fixture failure among the three
  (`TestEval_CRD_TypedBasicAuth_SecretRetention`) is on that label, which the claim excludes as a credential fragment.
- Outside the claim's outputs: Go `fmt` formatting of the middleware struct (`%v`, `%+v`, `%#v`) does print the carrier
  values. The only production use found is the `%#v` fallback in `pkg/server/configurationwatcher.go:246`, reached only when
  `json.Marshal` of the configuration fails. It is neither serialization nor redactor output.

## C5

Target: [evalon/traefik-se-2af317a2](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-2af317a2) at `dac2f7eaaf92a5b6acf7ef0a6fd89970001a7f85`. Verdict: **CONFIRMED** (execution: confirmed for values nested in untyped containers; isolation: refuted).

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-2af317a2
git worktree add --detach ~/wt/2af317a2 FETCH_HEAD   # dac2f7eaaf92a5b6acf7ef0a6fd89970001a7f85
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/2af317a2/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/2af317a2/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/2af317a2/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/2af317a2/pkg/server/
cp verify/probes/2af317a2/verify_c5_probe_test.go ~/wt/2af317a2/pkg/provider/kubernetes/crd/
cd ~/wt/2af317a2
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C5' ./pkg/provider/kubernetes/crd
# fixture-free variant, in a clean worktree
REMOTE=sol WT=~/scratch/c5 sh verify/repro/C5_secretvalue_nested_plugin_input.sh
# the fixture tests named by the claim (part of the baseline run)
go test -v -race -count=1 -timeout=10m -run '^TestEval_' ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx ./pkg/middlewares/auth ./pkg/server
```

On this branch `Middleware.ResolveSecrets()` returns a copy in which every plugin string equal to a bound reference becomes a
`dynamic.SecretValue{Reference, Value}` struct (`String()` returns the reference, `MarshalJSON` the value). The production
Yaegi decoder converts it with a mapstructure hook (`secretValueHookFunc`). The probe uses the real `plugins.Builder` with
three local Yaegi plugins and the delivered path: `verifygeneric` (`type Config map[string]interface{}`, reports the Go type
of every leaf, then overwrites its input with `MUTATED`), `verifytyped` (struct fields), and `verifymixed` (a struct with a
few untyped containers, reporting how each credential can be used).

#### Probe output (exit 1: the usable-credential assertions fail for nested values)

```console
    verify_c5_probe_test.go:405: declared (provider) : {"secret":"urn:k8s:secret:name:key","user":"admin"}
    verify_c5_probe_test.go:407: received by plugin  : {"secret":"this_is_the_secret","user":"admin"}
    verify_c5_probe_test.go:408: Go types in plugin  : {".secret":"string (string assertion ok: true)",".user":"string (string assertion ok: true)"}
    verify_c5_probe_test.go:448: shared after builds : {"secret":"urn:k8s:secret:name:key","user":"admin"}
    verify_c5_probe_test.go:449: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c5_probe_test.go:405: declared (provider) : {"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}
    verify_c5_probe_test.go:407: received by plugin  : {"secret_0":{"secret_1":{"secret_2":{"secret":"this_is_the_very_deep_secret","user":"admin"}}}}
    verify_c5_probe_test.go:408: Go types in plugin  : {".secret_0.secret_1.secret_2.secret":"dynamic.SecretValue (string assertion ok: false)",".secret_0.secret_1.secret_2.user":"string (string assertion ok: true)"}
    verify_c5_probe_test.go:448: shared after builds : {"secret_0":{"secret_1":{"secret_2":{"secret":"urn:k8s:secret:name:key","user":"admin"}}}}
    verify_c5_probe_test.go:449: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c5_probe_test.go:405: declared (provider) : {"secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}
    verify_c5_probe_test.go:407: received by plugin  : {"secret":["secret_data1","secret_data2"]}
    verify_c5_probe_test.go:408: Go types in plugin  : {".secret[0]":"dynamic.SecretValue (string assertion ok: false)",".secret[1]":"dynamic.SecretValue (string assertion ok: false)"}
    verify_c5_probe_test.go:448: shared after builds : {"secret":["urn:k8s:secret:name:key1","urn:k8s:secret:name:key2"]}
    verify_c5_probe_test.go:449: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c5_probe_test.go:405: declared (provider) : {"users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}
    verify_c5_probe_test.go:407: received by plugin  : {"users":[{"name":"admin","secret":"admin_password"},{"name":"user","secret":"user_password"}]}
    verify_c5_probe_test.go:408: Go types in plugin  : {".users[0].name":"string (string assertion ok: true)",".users[0].secret":"dynamic.SecretValue (string assertion ok: false)",".users[1].name":"string (string assertion ok: true)",".users[1].secret":"dynamic.SecretValue (string assertion ok: false)"}
    verify_c5_probe_test.go:448: shared after builds : {"users":[{"name":"admin","secret":"urn:k8s:secret:name:key1"},{"name":"user","secret":"urn:k8s:secret:name:key2"}]}
    verify_c5_probe_test.go:449: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c5_probe_test.go:405: declared (provider) : {"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"urn:k8s:secret:evalkeys:slash","m~n":"urn:k8s:secret:evalkeys:tilde","x/y":["literal-0",{"~0":"urn:k8s:secret:evalkeys:listed"}],"~1":"urn:k8s:secret:evalkeys:escaped"}
    verify_c5_probe_test.go:407: received by plugin  : {"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"qx_slash_value","m~n":"qx_tilde_value","x/y":["literal-0",{"~0":"qx_listed_value"}],"~1":"qx_escaped_value"}
    verify_c5_probe_test.go:408: Go types in plugin  : {"./":"string (string assertion ok: true)",".a.b":"string (string assertion ok: true)",".a/b":"string (string assertion ok: true)",".m~n":"string (string assertion ok: true)",".x/y[0]":"string (string assertion ok: true)",".x/y[1].~0":"dynamic.SecretValue (string assertion ok: false)",".~1":"string (string assertion ok: true)"}
    verify_c5_probe_test.go:448: shared after builds : {"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"urn:k8s:secret:evalkeys:slash","m~n":"urn:k8s:secret:evalkeys:tilde","x/y":["literal-0",{"~0":"urn:k8s:secret:evalkeys:listed"}],"~1":"urn:k8s:secret:evalkeys:escaped"}
    verify_c5_probe_test.go:449: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
    verify_c5_probe_test.go:405: declared (provider) : {"flag":"urn:k8s:secret:evalkeys:bool","hosts":["literal.example.com","urn:k8s:secret:evalkeys:host"],"inner":{"enabled":"urn:k8s:secret:evalkeys:bool","port":"urn:k8s:secret:evalkeys:int","token":"urn:k8s:secret:evalkeys:token"},"ratio":"urn:k8s:secret:evalkeys:float","secret":"urn:k8s:secret:evalkeys:int","user":"admin"}
    verify_c5_probe_test.go:407: received by plugin  : {"User":"admin","Secret":42,"Ratio":0.25,"Flag":true,"Inner":{"Token":"qx_token_value","Port":42,"Enabled":true},"Hosts":["literal.example.com","qx.example.com"]}
    verify_c5_probe_test.go:408: Go types in plugin  : n/a (typed plugin struct)
    verify_c5_probe_test.go:448: shared after builds : {"flag":"urn:k8s:secret:evalkeys:bool","hosts":["literal.example.com","urn:k8s:secret:evalkeys:host"],"inner":{"enabled":"urn:k8s:secret:evalkeys:bool","port":"urn:k8s:secret:evalkeys:int","token":"urn:k8s:secret:evalkeys:token"},"ratio":"urn:k8s:secret:evalkeys:float","secret":"urn:k8s:secret:evalkeys:int","user":"admin"}
    verify_c5_probe_test.go:449: shared config unchanged=true, bindings unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0, MiddlewareInfo.Err=[]
--- FAIL: TestVerify_C5_PluginExecutionAndIsolation (0.65s)
    --- PASS: TestVerify_C5_PluginExecutionAndIsolation/flat (0.52s)
    --- FAIL: TestVerify_C5_PluginExecutionAndIsolation/nested_deep_map (0.02s)
    --- FAIL: TestVerify_C5_PluginExecutionAndIsolation/nested_list (0.02s)
    --- FAIL: TestVerify_C5_PluginExecutionAndIsolation/nested_list_of_maps (0.02s)
    --- FAIL: TestVerify_C5_PluginExecutionAndIsolation/escaped_keys (0.02s)
    --- PASS: TestVerify_C5_PluginExecutionAndIsolation/typed (0.02s)
    verify_c5_probe_test.go:487: 1 token (string field)                                           type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 10 rules[0].password (map inside []interface{})                  type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:evalkeys:token"
    verify_c5_probe_test.go:487: 2 any (interface{} field)                                        type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 3 labels.k (map[string]string)                                   type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 4 creds[0].password (struct in slice)                            type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 5 options.password (map[string]interface{})                      type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 6 users[0].secret ([]map[string]interface{})                     type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 7 list[0] ([]interface{})                                        type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    verify_c5_probe_test.go:487: 8 options.nested.password (map inside map[string]interface{})    type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:evalkeys:token"
    verify_c5_probe_test.go:487: 9 deep.password (map inside interface{} field)                   type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:evalkeys:token"
--- FAIL: TestVerify_C5_UntypedContainersKeepSecretValue (0.03s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.775s
FAIL
```

```console
$ grep -c "DATA RACE" ~/logs/probe-2af3-c5.log
0
```

#### Fixture-free repro in a clean worktree (PASS means reproduced)

```console
    repro_c5_test.go:146: 1 token (string field)                                           type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    repro_c5_test.go:146: 2 options.password (map[string]interface{} entry)                type=string string-assertion=true asserted="qx_token_value" fmt.Sprint="qx_token_value"
    repro_c5_test.go:146: 3 options.nested.password (map inside map[string]interface{})    type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:creds:token"
    repro_c5_test.go:146: 4 rules[0].password (map inside []interface{})                   type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:creds:token"
--- PASS: TestRepro_C5_NestedSecretValueReachesPlugin (0.54s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	1.675s
```

#### Fixture tests on this branch (mock plugin builder, no production decoding hook)

```console
--- FAIL: TestEval_ProviderToBuilder_PluginTypedSecretField (0.54s)
--- FAIL: TestEval_ProviderToBuilder_PluginSuccessExecution (0.02s)
--- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution (0.02s)
--- PASS: TestEval_ProviderToBuilder_ConcurrentBuilds_RaceDetector (0.03s)
--- PASS: TestEval_ProviderToBuilder_PluginNestedExecution (0.00s)
--- FAIL: TestEval_ProviderToBuilder_PluginInputMutationIsolation (0.00s)
        	            	* 'secret' expected type 'int', got unconvertible type 'dynamic.SecretValue', value: 'urn:k8s:secret:name:key'
```

- *execution*, flat and typed fields: usable. A top-level string, `int`/`float64`/`bool` fields, nested **struct** fields,
  `[]string`, `map[string]string`, and first-level entries of `map[string]interface{}` / `[]interface{}` /
  `interface{}` fields all reach the plugin as plain Go values (`type=string ... asserted="qx_token_value"`, `Secret:42`).
  The fixture failures for flat and typed fields come from the fixture's mock builder, which decodes without the
  production hook (`got unconvertible type 'dynamic.SecretValue'`); they do not occur with the real plugin builder.
- *execution*, values nested inside an untyped container: **not usable**. With `type Config map[string]interface{}` every
  credential below the first level, and with a struct configuration every credential inside a map held by a
  `map[string]interface{}`, `interface{}` or `[]interface{}` field, reaches the plugin as Traefik's `dynamic.SecretValue`
  struct: `type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:evalkeys:token"`.
  The task's own nested fixtures (`with_plugin_deep_read_secret.yml`, `..._array_of_secret.yml`,
  `..._array_of_map_contain_secret.yml`) and the list entry of the escaped-key shape are in this case. The mapstructure
  hook only runs when the decoder visits the value itself; containers decoded into an `interface{}` target are copied as
  they are.
- *isolation*: refuted. For every shape, after the plugin overwrote its input: `shared config unchanged=true, bindings
  unchanged=true, contains MUTATED=false, concurrent builds=20 mismatches=0`, and no race report.

**Impact reasoning.** A Yaegi plugin cannot import Traefik's `dynamic` package, so a `dynamic.SecretValue` inside its
configuration is opaque: `v.(string)` fails, and formatting it (`fmt.Sprint`, `%s`, `%v`) yields the reference text, not the
credential. A plugin reading `config.Options["nested"]["password"]` therefore either errors or silently authenticates
upstream with the literal string `urn:k8s:secret:...`. The plugin still builds and the API still shows only references, so
nothing signals the problem. Before the change the same configuration delivered the plain value. Plugins with free-form
nested options (maps of maps, lists of objects) are the affected group; fully struct-typed plugins are not. The only
workaround is to declare typed structs in the plugin, which a user of a third-party plugin cannot do.

## C6

Target: [evalon/traefik-se-9fafd9a4](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-9fafd9a4) at `7bdfcee363c373765c726a9bac7a02138ef87514`. Verdict: **REFUTED**.

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-9fafd9a4
git worktree add --detach ~/wt/9fafd9a4 FETCH_HEAD   # 7bdfcee363c373765c726a9bac7a02138ef87514
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/9fafd9a4/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/9fafd9a4/pkg/server/
cp verify/probes/9fafd9a4/verify_ingress_probe_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/ingress-nginx/
cd ~/wt/9fafd9a4
go test -v -race -count=1 -timeout=200s -run '^TestVerify_' ./pkg/provider/kubernetes/ingress-nginx
go test -v -race -count=1 -timeout=5m -run '^TestEval_IngressNGINX_(BasicAndDigestAuth_SecretRetentionAndSerialization|AuthFileRuntimeNormalization)$' ./pkg/provider/kubernetes/ingress-nginx
```

The probe creates Ingress-NGINX `auth-type: basic` and `digest` Ingresses whose `auth-file` Secret content is, in turn,
normalized, padded with spaces (with and without a trailing newline), and wrapped in tabs, CRLF, blank and comment lines.
Each configuration goes through the delivered path and is exercised with valid and invalid credentials; the same
configuration is also built fixture-style.

```console
    verify_ingress_probe_test.go:146: basic normalized content="test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/" users=["urn:k8s:secret:basic-auth:auth"] qualified=default-ingress-with-basicauth-rule-0-path-0-tls-basic-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing BasicAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: basic padded-no-trailing-newline content="  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  " users=["urn:k8s:secret:basic-auth:auth"] qualified=default-ingress-with-basicauth-rule-0-path-0-tls-basic-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing BasicAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: basic padded-trailing-newline content="  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  \n" users=["urn:k8s:secret:basic-auth:auth"] qualified=default-ingress-with-basicauth-rule-0-path-0-basic-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing BasicAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: basic tabs-crlf-blank-and-comment-lines content="# comment\r\n\r\n\ttest:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/ \t\r\n\r\n" users=["urn:k8s:secret:basic-auth:auth"] qualified=default-ingress-with-basicauth-rule-0-path-0-tls-basic-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing BasicAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: digest normalized content="user:Authentication Required:0a132d8fa4138c56c1c2b50e9c9d1a1f" users=["urn:k8s:secret:digest-auth:auth"] qualified=default-ingress-with-digestauth-rule-0-path-0-tls-digest-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing digestAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: digest padded-no-trailing-newline content="  user:Authentication Required:0a132d8fa4138c56c1c2b50e9c9d1a1f  " users=["urn:k8s:secret:digest-auth:auth"] qualified=default-ingress-with-digestauth-rule-0-path-0-digest-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing digestAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: digest padded-trailing-newline content="  user:Authentication Required:0a132d8fa4138c56c1c2b50e9c9d1a1f  \n" users=["urn:k8s:secret:digest-auth:auth"] qualified=default-ingress-with-digestauth-rule-0-path-0-digest-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing digestAuth: invalid user entry at position 0
    verify_ingress_probe_test.go:146: digest tabs-crlf-blank-and-comment-lines content="# comment\r\n\r\n\tuser:Authentication Required:0a132d8fa4138c56c1c2b50e9c9d1a1f \t\r\n\r\n" users=["urn:k8s:secret:digest-auth:auth"] qualified=default-ingress-with-digestauth-rule-0-path-0-tls-digest-auth@kubernetesingressnginx build_err=<nil> valid=200 invalid=401 | unqualified fixture-style build err=error parsing digestAuth: invalid user entry at position 0
--- PASS: TestVerify_C6_AuthFileWhitespace (0.60s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/basic/normalized (0.50s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/basic/padded-no-trailing-newline (0.03s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/basic/padded-trailing-newline (0.01s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/basic/tabs-crlf-blank-and-comment-lines (0.01s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/digest/normalized (0.01s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/digest/padded-no-trailing-newline (0.01s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/digest/padded-trailing-newline (0.02s)
    --- PASS: TestVerify_C6_AuthFileWhitespace/digest/tabs-crlf-blank-and-comment-lines (0.01s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	1.723s
```

```console
$ grep -c "DATA RACE" ~/logs/probe-9faf-ingress.log
0
```

Fixture test on this branch:

```console
        	Messages:   	valid credential controls must build: error parsing BasicAuth: invalid user entry at position 0
        	Messages:   	valid credential controls must build: error parsing BasicAuth: invalid user entry at position 0
--- FAIL: TestEval_IngressNGINX_AuthFileRuntimeNormalization (0.00s)
--- PASS: TestEval_IngressNGINX_BasicAndDigestAuth_SecretRetentionAndSerialization (0.51s)
```

Through the delivered path every whitespace-bearing entry builds (`build_err=<nil>`) and authenticates exactly like its
normalized counterpart (`valid=200 invalid=401`), for BasicAuth and DigestAuth. The fixture test fails on this branch, but
it fails on the *normalized* control too (`valid credential controls must build: error parsing BasicAuth: invalid user entry
at position 0`): the branch keeps Secret values in a store keyed by `<middleware>@<provider>`, and the fixture builds with
unqualified names, which a running Traefik never does. The failure is independent of whitespace.

## C7

Target: [evalon/traefik-se-9fafd9a4](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-9fafd9a4) at `7bdfcee363c373765c726a9bac7a02138ef87514`. Verdict: **REFUTED**.

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-9fafd9a4
git worktree add --detach ~/wt/9fafd9a4 FETCH_HEAD   # 7bdfcee363c373765c726a9bac7a02138ef87514
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/9fafd9a4/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/9fafd9a4/pkg/server/
cp verify/probes/9fafd9a4/verify_harness_test.go verify/probes/9fafd9a4/verify_probe_test.go verify/probes/9fafd9a4/verify_c10_probe_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cd ~/wt/9fafd9a4
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C(7|8|9)' ./pkg/provider/kubernetes/crd
go test -v -race -count=1 -timeout=5m -run '^TestEval_ProviderToBuilder_DigestAuthLateBinding$' ./pkg/provider/kubernetes/crd
```

```console
    verify_probe_test.go:81: provider users: ["urn:k8s:secret:authsecret:users"]
    verify_probe_test.go:85: unqualified (fixture-style) build error: error parsing digestAuth: invalid user entry at position 0
    verify_probe_test.go:90: delivered middleware name: default-forwardauth@kubernetescrd
    verify_probe_test.go:90: delivered middleware name: default-basicauth@kubernetescrd
    verify_probe_test.go:90: delivered middleware name: default-digestauth@kubernetescrd
    verify_probe_test.go:97: qualified build: err=<nil> matching credentials=200 wrong password=401
--- PASS: TestVerify_C7_DigestAuthProductionPath (0.49s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	2.681s
```

Fixture test on this branch:

```console
        	            	error parsing digestAuth: invalid user entry at position 0
--- FAIL: TestEval_ProviderToBuilder_DigestAuthLateBinding (0.05s)
```

The CRD `digestauth` middleware backed by a Secret (`users` = `urn:k8s:secret:authsecret:users` in the published
configuration) is delivered as `default-digestauth@kubernetescrd`; building it succeeds, a completed Digest challenge with
matching credentials returns 200, a wrong password 401, and the shared configuration still holds the reference afterwards.
Late binding does supply the credential entry. The fixture failure (`error parsing digestAuth: invalid user entry at position
0`) is reproduced by the probe only for the unqualified, fixture-style name, for which the branch's store has no binding.

## C8

Target: [evalon/traefik-se-9fafd9a4](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-9fafd9a4) at `7bdfcee363c373765c726a9bac7a02138ef87514`. Verdict: **REFUTED**.

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-9fafd9a4
git worktree add --detach ~/wt/9fafd9a4 FETCH_HEAD   # 7bdfcee363c373765c726a9bac7a02138ef87514
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/9fafd9a4/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/9fafd9a4/pkg/server/
cp verify/probes/9fafd9a4/verify_harness_test.go verify/probes/9fafd9a4/verify_probe_test.go verify/probes/9fafd9a4/verify_c10_probe_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cd ~/wt/9fafd9a4
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C(7|8|9)' ./pkg/provider/kubernetes/crd
go test -v -race -count=1 -timeout=200s -run '^TestVerifyControl_C8' ./pkg/provider/kubernetes/crd
go test -v -race -count=1 -timeout=5m -run '^TestEval_ProviderToBuilder_ConcurrentBuilds_RaceDetector$' ./pkg/provider/kubernetes/crd
```

Workload: valid Secret-backed `basicauth` and `digestauth` CRD middlewares, delivered path; 40 goroutines each build the
middleware 20 times on one shared runtime configuration, while other goroutines `json.Marshal` that configuration and the
provider keeps reloading (which rewrites the Secret bindings of the store).

```console
    verify_probe_test.go:188: basic: 40 concurrent builders x 20 builds, 55 provider reloads, build failures=0, MiddlewareInfo.Err=[]
    verify_probe_test.go:188: digest: 40 concurrent builders x 20 builds, 84 provider reloads, build failures=0, MiddlewareInfo.Err=[]
--- PASS: TestVerify_C8_ConcurrentValidAuthBuilds (0.12s)
    --- PASS: TestVerify_C8_ConcurrentValidAuthBuilds/basic (0.03s)
    --- PASS: TestVerify_C8_ConcurrentValidAuthBuilds/digest (0.03s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	2.681s
```

```console
$ grep -c "DATA RACE" ~/logs/probe-9faf-crd.log
0
```

Control: 20 concurrent builds of an inline, malformed BasicAuth middleware (`inline@file`, no Secret involved):

```console
=== RUN   TestVerifyControl_C8_FailingBuildsRaceWithoutSecrets
==================
WARNING: DATA RACE
Read at 0x00c0004b4d78 by goroutine 34:
  github.com/traefik/traefik/v3/pkg/config/runtime.(*MiddlewareInfo).AddError()
      /home/ubuntu/wt/9fafd9a4/pkg/config/runtime/runtime_http.go:125 +0x84
  github.com/traefik/traefik/v3/pkg/server/middleware.(*Builder).BuildMiddlewareChain.func1()
      /home/ubuntu/wt/9fafd9a4/pkg/server/middleware/middlewares.go:92 +0x2f4
  github.com/containous/alice.Chain.Then()
      /home/ubuntu/go/pkg/mod/github.com/containous/alice@v0.0.0-20181107144136-d83ebdd94cbd/chain.go:51 +0xe2
  github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd_test.TestVerifyControl_C8_FailingBuildsRaceWithoutSecrets.func1()
      /home/ubuntu/wt/9fafd9a4/pkg/provider/kubernetes/crd/verify_probe_test.go:241 +0x179

Previous write at 0x00c0004b4d78 by goroutine 37:
  github.com/traefik/traefik/v3/pkg/config/runtime.(*MiddlewareInfo).AddError()
      /home/ubuntu/wt/9fafd9a4/pkg/config/runtime/runtime_http.go:129 +0x225
  github.com/traefik/traefik/v3/pkg/server/middleware.(*Builder).BuildMiddlewareChain.func1()
      /home/ubuntu/wt/9fafd9a4/pkg/server/middleware/middlewares.go:92 +0x2f4
  github.com/containous/alice.Chain.Then()
      /home/ubuntu/go/pkg/mod/github.com/containous/alice@v0.0.0-20181107144136-d83ebdd94cbd/chain.go:51 +0xe2
  github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd_test.TestVerifyControl_C8_FailingBuildsRaceWithoutSecrets.func1()
      /home/ubuntu/wt/9fafd9a4/pkg/provider/kubernetes/crd/verify_probe_test.go:241 +0x179
...
    testing.go:1712: race detected during execution of test
--- FAIL: TestVerifyControl_C8_FailingBuildsRaceWithoutSecrets (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.090s
```

Fixture test on this branch:

```console
        	Messages:   	concurrent middleware builds must not return errors
--- FAIL: TestEval_ProviderToBuilder_ConcurrentBuilds_RaceDetector (0.52s)
WARNING: DATA RACE
Read at 0x00c001135458 by goroutine 1617:
  github.com/traefik/traefik/v3/pkg/config/runtime.(*MiddlewareInfo).AddError()
      /home/ubuntu/wt/9fafd9a4/pkg/config/runtime/runtime_http.go:125 +0x84
  github.com/traefik/traefik/v3/pkg/server/middleware.(*Builder).BuildMiddlewareChain.func1()
      /home/ubuntu/wt/9fafd9a4/pkg/server/middleware/middlewares.go:92 +0x2f4
  github.com/containous/alice.Chain.Then()
```

- Concurrent construction of *valid* Secret-backed authentication middlewares: 800 builds per middleware type, 0 failures,
  `MiddlewareInfo.Err=[]`, and no race report, with concurrent serialization and concurrent binding replacement. Binding
  reads go through the store's lock and resolution works on a copy of the configuration.
- The race the fixture reports is in `runtime.(*MiddlewareInfo).AddError` (unsynchronized append to `Err`). It is reached
  only when builds *fail*: in the fixture they fail because of the unqualified names (`concurrent middleware builds must not
  return errors: ... invalid user entry at position 0`). The control reproduces the same race report with no Secret at
  all, so it is a property of concurrent failing builds of any middleware, in code the branch did not change, not of
  Secret-backed construction.

## C9

Target: [evalon/traefik-se-9fafd9a4](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-9fafd9a4) at `7bdfcee363c373765c726a9bac7a02138ef87514`. Verdict: **REFUTED**.

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-9fafd9a4
git worktree add --detach ~/wt/9fafd9a4 FETCH_HEAD   # 7bdfcee363c373765c726a9bac7a02138ef87514
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/9fafd9a4/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/9fafd9a4/pkg/server/
cp verify/probes/9fafd9a4/verify_harness_test.go verify/probes/9fafd9a4/verify_probe_test.go verify/probes/9fafd9a4/verify_c10_probe_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cd ~/wt/9fafd9a4
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C(7|8|9)' ./pkg/provider/kubernetes/crd
go test -v -race -count=1 -timeout=5m -run '^TestEval_(GetUsers_ErrorReportsPositionWithoutLeakingCredentials|ProviderToBuilder_BasicAuthSecretEntryPosition|ProviderToBuilder_DigestAuthSecretEntryPosition)$' ./pkg/middlewares/auth ./pkg/provider/kubernetes/crd
```

The probe puts one malformed entry in the `authsecret` Secret after 0, 1, 2 and 3 valid entries, for BasicAuth and
DigestAuth and three kinds of malformed text, and builds through the delivered path. `expanded_index` is the 0-based
position of the malformed entry in the expanded credential list.

```console
    verify_probe_test.go:215: default-basicauth malformed="badsentinel" preceding_valid=0 expanded_index=0 -> returned="error parsing BasicAuth: invalid user entry at line 1 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing BasicAuth: invalid user entry at line 1 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-basicauth malformed="badsentinel" preceding_valid=1 expanded_index=1 -> returned="error parsing BasicAuth: invalid user entry at line 2 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing BasicAuth: invalid user entry at line 2 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-basicauth malformed="badsentinel" preceding_valid=2 expanded_index=2 -> returned="error parsing BasicAuth: invalid user entry at line 3 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing BasicAuth: invalid user entry at line 3 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-basicauth malformed="badsentinel" preceding_valid=3 expanded_index=3 -> returned="error parsing BasicAuth: invalid user entry at line 4 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing BasicAuth: invalid user entry at line 4 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-digestauth malformed="badsentinel" preceding_valid=0 expanded_index=0 -> returned="error parsing digestAuth: invalid user entry at line 1 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing digestAuth: invalid user entry at line 1 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-digestauth malformed="badsentinel" preceding_valid=1 expanded_index=1 -> returned="error parsing digestAuth: invalid user entry at line 2 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing digestAuth: invalid user entry at line 2 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-digestauth malformed="badsentinel" preceding_valid=2 expanded_index=2 -> returned="error parsing digestAuth: invalid user entry at line 3 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing digestAuth: invalid user entry at line 3 of \"urn:k8s:secret:authsecret:users\""]
    verify_probe_test.go:215: default-digestauth malformed="badsentinel" preceding_valid=3 expanded_index=3 -> returned="error parsing digestAuth: invalid user entry at line 4 of \"urn:k8s:secret:authsecret:users\"" stored=["error parsing digestAuth: invalid user entry at line 4 of \"urn:k8s:secret:authsecret:users\""]
--- PASS: TestVerify_C9_SecretEntryPosition (0.96s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	2.681s
```

The other two malformed kinds (`user`, `format`) give the same positions:

```console
24 lines in total; distinct (expanded_index -> reported line) pairs:
      6 expanded_index=0 -> line 1
      6 expanded_index=1 -> line 2
      6 expanded_index=2 -> line 3
      6 expanded_index=3 -> line 4
```

Fixture tests on this branch:

```console
--- FAIL: TestEval_ProviderToBuilder_BasicAuthSecretEntryPosition (0.87s)
--- FAIL: TestEval_ProviderToBuilder_DigestAuthSecretEntryPosition (0.89s)
--- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials (0.00s)
        	Messages:   	moving a malformed entry from first to third must increase its reported position by two
```

Each reported position follows the malformed entry: expanded index 0, 1, 2, 3 is reported as line 1, 2, 3, 4 of
`"urn:k8s:secret:authsecret:users"` (1-based), both in the returned error and in the error stored on the middleware, and the
malformed text itself never appears. The fixture tests fail because, fixture-style, the reference is never expanded (no
binding for the unqualified name), so the only entry seen is the reference itself, always at `position 0`.

## C10

Target: [evalon/traefik-se-9fafd9a4](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-9fafd9a4) at `7bdfcee363c373765c726a9bac7a02138ef87514`. Verdict: **CONFIRMED** (binding lifetime: confirmed; queued construction: confirmed).

```sh
git fetch https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares evalon/traefik-se-9fafd9a4
git worktree add --detach ~/wt/9fafd9a4 FETCH_HEAD   # 7bdfcee363c373765c726a9bac7a02138ef87514
# byte-exact eval fixtures (eval_tests.zip extracted to ~/eval)
cp ~/eval/tests/eval_crd_behavior_test.go ~/eval/tests/export_eval_test.go ~/eval/tests/eval_provider_to_builder_runtime_test.go ~/eval/tests/eval_deepcopy_isolation_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cp ~/eval/tests/eval_ingress_nginx_test.go ~/eval/tests/export_eval_ingress_nginx_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/ingress-nginx/
cp ~/eval/tests/eval_auth_sanitation_test.go ~/wt/9fafd9a4/pkg/middlewares/auth/
cp ~/eval/tests/eval_logconfiguration_test.go ~/wt/9fafd9a4/pkg/server/
cp verify/probes/9fafd9a4/verify_harness_test.go verify/probes/9fafd9a4/verify_probe_test.go verify/probes/9fafd9a4/verify_c10_probe_test.go ~/wt/9fafd9a4/pkg/provider/kubernetes/crd/
cd ~/wt/9fafd9a4
go test -v -race -count=1 -timeout=200s -run '^TestVerify_C10' ./pkg/provider/kubernetes/crd
grep -n 'secretStore().Set(\|func (s \*Store) Set(' pkg/provider/kubernetes/crd/kubernetes.go pkg/config/dynamic/secrets/store.go
grep -n 'b.secrets' pkg/server/middleware/middlewares.go
# fixture-free variant, in a clean worktree
REMOTE=sol WT=~/scratch/c10 sh verify/repro/C10_replaced_bindings.sh
```

Configuration A: CRD `basicauth` (Secret `authsecret`) and a plugin middleware referencing `urn:k8s:secret:name:key`.
Replacement B: both middlewares are pointed to new Secrets (`authsecret2`, `name2`) holding the *same* credentials, a routine
migration. The loader is the production `loadConfigurationFromCRD`, which publishes the provider's bindings to the store
before the configuration is sent.

```console
$ grep -n 'secretStore().Set(\|func (s \*Store) Set(' pkg/provider/kubernetes/crd/kubernetes.go pkg/config/dynamic/secrets/store.go
pkg/provider/kubernetes/crd/kubernetes.go:402:	p.secretStore().Set(ProviderName, secretValues)
pkg/config/dynamic/secrets/store.go:48:func (s *Store) Set(providerName string, middlewares map[string]Values) {
$ grep -n 'b.secrets' pkg/server/middleware/middlewares.go
111:	config := b.secrets.Resolve(middlewareName, info.Middleware)
112:	secretValues := b.secrets.Get(middlewareName)
```

#### Part "binding lifetime"

```console
    verify_c10_probe_test.go:98: A built while A's bindings are published : basicAuth users=["urn:k8s:secret:authsecret:users"] revision=633bd67bcea72477341dc7f376d8d437 build_err=<nil> test:test=200 | plugin declared=urn:k8s:secret:name:key received=["this_is_the_secret"] build_err=<nil>
    verify_c10_probe_test.go:102: A built after replacement B was loaded   : basicAuth users=["urn:k8s:secret:authsecret:users"] revision=633bd67bcea72477341dc7f376d8d437 build_err=error parsing BasicAuth: invalid user entry at position 0 test:test=0 | plugin declared=urn:k8s:secret:name:key received=["urn:k8s:secret:name:key"] build_err=<nil>
    verify_c10_probe_test.go:103: B built after replacement B was loaded   : basicAuth users=["urn:k8s:secret:authsecret2:users"] revision=3d2c00262833815bc029b66749c2df9a build_err=<nil> test:test=200 | plugin declared=urn:k8s:secret:name2:key received=["this_is_the_secret"] build_err=<nil>
    verify_c10_probe_test.go:104: SecretsRevision A=633bd67bcea72477341dc7f376d8d437 B=3d2c00262833815bc029b66749c2df9a (A's revision is still on the retained configuration, but is not used to find A's values)
--- PASS: TestVerify_C10_BindingLifetime (0.55s)
```

The retained configuration A is unchanged (same users reference, same `SecretsRevision`), but once B is loaded,
`Store.Set` has replaced the `kubernetescrd` bindings: A's BasicAuth no longer builds (`invalid user entry at position 0`,
the reference itself is parsed as a user), and A's plugin is built **without error** with the literal
`urn:k8s:secret:name:key` as its secret. `buildConstructor` resolves by middleware name against the current bindings
(`b.secrets.Resolve(middlewareName, ...)`); `SecretsRevision` is carried on A but not used to find A's values.

#### Part "queued construction", ordering 1: A pending in the watcher while the provider loads B

```console
    verify_c10_probe_test.go:192: watcher applied configuration A (B already loaded by provider: true): basicAuth users=["urn:k8s:secret:authsecret:users"] revision=633bd67bcea72477341dc7f376d8d437 build_err=error parsing BasicAuth: invalid user entry at position 0 test:test=0 | plugin declared=urn:k8s:secret:name:key received=["urn:k8s:secret:name:key"] build_err=<nil>
    verify_c10_probe_test.go:192: watcher applied configuration B (B already loaded by provider: true): basicAuth users=["urn:k8s:secret:authsecret2:users"] revision=3d2c00262833815bc029b66749c2df9a build_err=<nil> test:test=200 | plugin declared=urn:k8s:secret:name2:key received=["this_is_the_secret"] build_err=<nil>
--- PASS: TestVerify_C10_WatcherBuildsPendingA (0.05s)
```

A real `server.ConfigurationWatcher`: its applier is busy with another provider's configuration, A becomes the pending
configuration, the provider loads B (bindings replaced), the applier becomes free and applies A before B arrives. A is
built against B's bindings and fails as above; nothing in the ordering, cancellation or lifetime management prevents it.

#### Part "queued construction", ordering 2: the default aggregator throttle

```console
    verify_c10_probe_test.go:221: crd.Provider implements ThrottleDuration(): false (false => aggregator applies providersThrottleDuration)
    verify_c10_probe_test.go:290: provider loaded replacement B (bindings replaced) at t=25ms
    verify_c10_probe_test.go:292: t=   1ms watcher applied kubernetescrd configuration A (other provider update included: false): basicAuth users=["urn:k8s:secret:authsecret:users"] revision=633bd67bcea72477341dc7f376d8d437 build_err=<nil> test:test=200 | plugin declared=urn:k8s:secret:name:key received=["this_is_the_secret"] build_err=<nil>
    verify_c10_probe_test.go:292: t= 227ms watcher applied kubernetescrd configuration A (other provider update included: true): basicAuth users=["urn:k8s:secret:authsecret:users"] revision=633bd67bcea72477341dc7f376d8d437 build_err=error parsing BasicAuth: invalid user entry at position 0 test:test=0 | plugin declared=urn:k8s:secret:name:key received=["urn:k8s:secret:name:key"] build_err=<nil>
    verify_c10_probe_test.go:292: t=2006ms watcher applied kubernetescrd configuration B (other provider update included: true): basicAuth users=["urn:k8s:secret:authsecret2:users"] revision=3d2c00262833815bc029b66749c2df9a build_err=<nil> test:test=200 | plugin declared=urn:k8s:secret:name2:key received=["this_is_the_secret"] build_err=<nil>
--- PASS: TestVerify_C10_ThrottledReplacement (4.04s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.737s
```

```console
$ grep -c "DATA RACE" ~/logs/probe-9faf-c10.log
0
```

With the real `aggregator.ProviderAggregator` and the default `providersThrottleDuration` of 2s (the CRD provider has no
`ThrottleDuration` of its own): B's bindings are published when the provider loads it (t=25ms), but B itself is held by the
throttle until t=2006ms. An unrelated update from another provider at t=227ms makes the watcher re-apply the last known
`kubernetescrd` configuration, A, which is rebuilt against B's bindings and breaks.

#### Fixture-free repro in a clean worktree (PASS means reproduced)

```console
--- PASS: TestRepro_C10_BindingLifetime (0.64s)
--- PASS: TestRepro_C10_WatcherBuildsPendingA (0.04s)
--- PASS: TestRepro_C10_ThrottledReplacement (4.04s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	5.837s
```

**Impact reasoning.** The published configuration and the Secret bindings travel separately and are not versioned together:
the bindings change at load time, the configuration is applied later (throttled by default for 2s, or queued behind a busy
applier). In that window, any rebuild of the still-current older configuration uses the newer bindings. Observed
consequences: a BasicAuth/DigestAuth middleware whose Secret was renamed or removed fails to build (its routers stop
serving until the new configuration is applied), and, worse, a plugin middleware is built successfully with the literal
`urn:k8s:secret:...` string in place of its token, i.e. state that looks healthy but is not, with no error recorded. The
trigger is ordinary: pointing a middleware to another Secret, removing a middleware, or any change that drops a binding,
combined with any other provider event within the throttle window (in a cluster, Kubernetes events are continuous). The
window closes when B is applied; there is no configuration-side workaround short of setting the throttle to zero, which
narrows but does not remove the pending-configuration ordering.
