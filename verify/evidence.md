Observations for run `v-8dac7b35`. Raw outputs are under `verify/logs/` (shell-wrapper xtrace lines from this VM's `git` shim and `go: downloading` lines were filtered out of the logs; nothing else was edited). Environment: `go version go1.26.3 linux/amd64`, Linux, 8 CPUs.

Worktrees used:

```sh
cd ~/repos/traefik
git fetch origin traefik-secret-references-in-middlewares-perfect
git checkout -b verify/traefik-secret-references-in-middlewares-v-8dac7b35 origin/traefik-secret-references-in-middlewares-perfect   # 84614aa0c, C3 and C4
git remote add evalrepo https://git-manager.itsdev.in/proxy/github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch evalrepo evalon/traefik-se-19e992fb evalon/traefik-se-5aec87b4
git worktree add --detach ../wt-c1 evalrepo/evalon/traefik-se-19e992fb   # 732b2ed60, C1
git worktree add --detach ../wt-c2 evalrepo/evalon/traefik-se-5aec87b4   # 9664df113, C2
```

All three branches have merge-base `af6d927c51191ae1df104aaf907793386b816c28` (the eval base).

## C1

Target: `evalon/traefik-se-19e992fb` (`732b2ed60`), worktree `../wt-c1`.

### Which tests are in scope

`verify/logs/c1-test-inventory.log` lists every test function added or modified against the base, and every added test line that mentions a plugin together with an error:

```console
$ git diff af6d927c5..HEAD -U0 -- '*_test.go' | grep -E '^\+\+\+ |^@@.*func Test|^\+func Test' | sed -E 's/^@@[^@]*@@ ?//' | uniq
+++ b/pkg/config/dynamic/secrets_test.go
+func TestMiddleware_ResolveSecrets(t *testing.T) {
+func TestMiddleware_ResolveSecrets_noSecrets(t *testing.T) {
+func TestMiddleware_SecretsNotSerialized(t *testing.T) {
+func TestUsers_SecretSource(t *testing.T) {
+++ b/pkg/middlewares/auth/basic_auth_test.go
func TestBasicAuthUsersFromFile(t *testing.T) {
+func TestBasicAuthMalformedUserError(t *testing.T) {
+++ b/pkg/middlewares/auth/digest_auth_test.go
func TestDigestAuthUserHeaderCanonical(t *testing.T) {
+func TestDigestAuthMalformedUserError(t *testing.T) {
+++ b/pkg/provider/kubernetes/crd/kubernetes_test.go
func TestLoadIngressRoutes(t *testing.T) {
func TestCreateBasicAuthCredentials(t *testing.T) {
func TestGlobalNativeLB(t *testing.T) {
+func TestLoadMiddlewaresSecretReferences(t *testing.T) {
+++ b/pkg/provider/kubernetes/ingress-nginx/kubernetes_test.go
func TestLoadIngresses(t *testing.T) {
+func TestResolveBasicAuthSecretReferences(t *testing.T) {
+++ b/pkg/server/middleware/secrets_test.go
+func TestBuilder_BasicAuthSecretReference(t *testing.T) {
+func TestBuilder_MalformedUserSecretReference(t *testing.T) {
+func TestBuilder_PluginSecretReference(t *testing.T) {
+func TestBuilder_PluginSecretReferenceError(t *testing.T) {
```

The only test that asserts a plugin error diagnostic is `TestBuilder_PluginSecretReferenceError` in `pkg/server/middleware/secrets_test.go`. Its three cases (assertion text, verbatim from the branch):

```go
{
	desc:  "decoding error quoting the value",
	token: "s3cr3t",
	build: func(token string) error {
		return fmt.Errorf("cannot decode token %q", token)
	},
	expectedError: fmt.Sprintf("plugin: cannot decode token %q", tokenRef),
},
{
	desc:  "value equal to a word of the error",
	token: "invalid",
	build: func(string) error {
		return errors.New("invalid token")
	},
	expectedError: "plugin: invalid token",
},
{
	desc:  "error only with the value",
	token: "s3cr3t",
	build: func(token string) error {
		if token == tokenRef {
			return nil
		}
		return fmt.Errorf("token %q is rejected", token)
	},
	expectedError: "plugin: invalid configuration referencing " + tokenRef + ": error details are hidden as they may contain Secret values",
},
```

with `tokenRef = "urn:k8s:secret:plugin:token"` and the assertion `require.EqualError(t, err, test.expectedError)` on the error returned by `BuildMiddlewareChain(...).Then(next)`.

Only the second case asserts text that overlaps the configured Secret (`"invalid"` inside `"plugin: invalid token"`). Its callback ignores its argument and returns the constant `errors.New("invalid token")`.

### Demonstration run of the branch's tests

```console
$ cd ../wt-c1 && go test -count=1 -v -run '^TestBuilder_PluginSecretReference(Error)?$' ./pkg/server/middleware
--- PASS: TestBuilder_PluginSecretReference (0.00s)
--- PASS: TestBuilder_PluginSecretReferenceError (0.00s)
    --- PASS: TestBuilder_PluginSecretReferenceError/decoding_error_quoting_the_value (0.00s)
    --- PASS: TestBuilder_PluginSecretReferenceError/error_only_with_the_value (0.00s)
    --- PASS: TestBuilder_PluginSecretReferenceError/value_equal_to_a_word_of_the_error (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.040s
exit=0
```

(`verify/logs/c1-branch-tests.log`)

### Mutation: change only the configured Secret values in the branch's own test

If an assertion required Secret data, changing the Secret while leaving the expected text alone would turn the test red.

```console
$ sed -i -E 's/^(\t\t\ttoken: +)"s3cr3t",/\1"Zq7#kP2x-9fLm",/; s/^(\t\t\ttoken: +)"invalid",/\1"hunter2",/' pkg/server/middleware/secrets_test.go
$ git diff -- pkg/server/middleware/secrets_test.go
-			token: "s3cr3t",
+			token: "Zq7#kP2x-9fLm",
-			token: "invalid",
+			token: "hunter2",
-			token: "s3cr3t",
+			token: "Zq7#kP2x-9fLm",
$ go test -count=1 -v -run '^TestBuilder_PluginSecretReferenceError$' ./pkg/server/middleware
--- PASS: TestBuilder_PluginSecretReferenceError (0.00s)
    --- PASS: TestBuilder_PluginSecretReferenceError/decoding_error_quoting_the_value (0.00s)
    --- PASS: TestBuilder_PluginSecretReferenceError/error_only_with_the_value (0.00s)
    --- PASS: TestBuilder_PluginSecretReferenceError/value_equal_to_a_word_of_the_error (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.029s
exit=0
```

(`verify/logs/c1-mutated-secret-values.log`; the test file was restored afterwards.)

### Probe: the asserted diagnostics for five Secret values, including `MiddlewareInfo.Err`

`verify/probes/c1_plugin_error_secret_independence_test.go` copies the three callbacks and expected strings verbatim, builds each with the branch's Secret value and four unrelated ones, and prints what the plugin received, the error from `Then()`, and `MiddlewareInfo.Err`.

```console
$ cp verify/probes/c1_plugin_error_secret_independence_test.go ../wt-c1/pkg/server/middleware/
$ cd ../wt-c1 && go test -tags verify -count=1 -v -run '^TestVerifyC1_' ./pkg/server/middleware
=== RUN   TestVerifyC1_AssertedDiagnosticsAreSecretIndependent/decoding_error_quoting_the_value
asserted diagnostic        : "plugin: cannot decode token \"urn:k8s:secret:plugin:token\""
configured Secret value    : "s3cr3t"
asserted text minus the ref: "plugin: cannot decode token \"\"" (contains the configured value: false)
Secret="s3cr3t"                             plugin saw ["s3cr3t" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: cannot decode token \"urn:k8s:secret:plugin:token\"" ; MiddlewareInfo.Err ["plugin: cannot decode token \"urn:k8s:secret:plugin:token\""]
Secret="Zq7#kP2x-9fLm"                      plugin saw ["Zq7#kP2x-9fLm" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: cannot decode token \"urn:k8s:secret:plugin:token\"" ; MiddlewareInfo.Err ["plugin: cannot decode token \"urn:k8s:secret:plugin:token\""]
...
=== RUN   TestVerifyC1_AssertedDiagnosticsAreSecretIndependent/value_equal_to_a_word_of_the_error
asserted diagnostic        : "plugin: invalid token"
configured Secret value    : "invalid"
asserted text minus the ref: "plugin: invalid token" (contains the configured value: true)
Secret="invalid"                            plugin saw ["invalid" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: invalid token" ; MiddlewareInfo.Err ["plugin: invalid token"]
Secret="Zq7#kP2x-9fLm"                      plugin saw ["Zq7#kP2x-9fLm" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: invalid token" ; MiddlewareInfo.Err ["plugin: invalid token"]
Secret="hunter2"                            plugin saw ["hunter2" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: invalid token" ; MiddlewareInfo.Err ["plugin: invalid token"]
Secret="correct horse battery staple"       plugin saw ["correct horse battery staple" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: invalid token" ; MiddlewareInfo.Err ["plugin: invalid token"]
Secret="0123456789abcdef0123456789abcdef"   plugin saw ["0123456789abcdef0123456789abcdef" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: invalid token" ; MiddlewareInfo.Err ["plugin: invalid token"]
=== RUN   TestVerifyC1_AssertedDiagnosticsAreSecretIndependent/error_only_with_the_value
asserted diagnostic        : "plugin: invalid configuration referencing urn:k8s:secret:plugin:token: error details are hidden as they may contain Secret values"
configured Secret value    : "s3cr3t"
asserted text minus the ref: "plugin: invalid configuration referencing : error details are hidden as they may contain Secret values" (contains the configured value: false)
Secret="s3cr3t"                             plugin saw ["s3cr3t" "urn:k8s:secret:plugin:token"] -> Then() err "plugin: invalid configuration referencing urn:k8s:secret:plugin:token: error details are hidden as they may contain Secret values" ; MiddlewareInfo.Err [...same string...]
...
--- PASS: TestVerifyC1_AssertedDiagnosticsAreSecretIndependent (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.028s
exit=0
```

(full, unabridged lines in `verify/logs/c1-probe.log`)

What was observed:

- Cases 1 and 3: the asserted text with the reference removed does not contain the configured Secret; the only Secret-related text asserted is the `urn:k8s:secret:plugin:token` reference (name and key, already public in the routing configuration).
- Case 2: the asserted text contains the word `invalid`, which equals the configured Secret, but the diagnostic is byte-identical for five different Secret values, in both the returned error and `MiddlewareInfo.Err`. The word comes from the plugin's constant message, not from the Secret.
- No assertion in the three cases changed outcome when the Secret changed.

## C2

Target: `evalon/traefik-se-5aec87b4` (`9664df113`), worktree `../wt-c2`.

Naming drift: `pkg/config/dynamic/urnrefs.go` and `RedactURNRefs` do not exist on this branch (`ls pkg/config/dynamic | grep -i urn` prints nothing). The reference formatting lives in `pkg/server/middleware/secrets.go` (`secretResolver.references`, `resolvePluginConfig`) and `pkg/server/middleware/plugins.go` (`pluginBuildError`).

Code path on the branch (for orientation; the verdict rests on the runs below):

```go
// pkg/server/middleware/secrets.go
func (r *secretResolver) references() string {
	return strings.Join(r.used, ", ")
}
...
	for key, value := range resolved {            // resolvePluginConfig: map[string]any
		resolved[key] = r.resolvePluginValue(value)
	}
```

```go
// pkg/server/middleware/middlewares.go
		references := secrets.references()
		middleware = func(next http.Handler) (http.Handler, error) {
			handler, err := newTraceablePlugin(ctx, middlewareName, plug, next)
			if err != nil && references != "" {
				return nil, fmt.Errorf("plugin: creating middleware with the values of %s failed, the error is not reported as it may disclose them", references)
			}
```

### Repro 1: repeated identical failing builds (`verify/repro/c2_plugin_error_reference_order_test.go`)

Configuration: one plugin middleware, four options, each a distinct `urn:k8s:secret:` reference, `Secrets` map with the four values. Every iteration builds a fresh, identical configuration through `BuildMiddlewareChain(...).Then(...)`.

```console
$ cp verify/repro/c2_plugin_error_reference_order_test.go ../wt-c2/pkg/server/middleware/
$ cd ../wt-c2 && go test -tags verify -count=1 -v -run '^TestVerifyC2_' ./pkg/server/middleware
=== RUN   TestVerifyC2_ReferencesKeepCollectionOrder
    references() = "urn:k8s:secret:z:z, urn:k8s:secret:a:a, urn:k8s:secret:m:m"
--- PASS: TestVerifyC2_ReferencesKeepCollectionOrder (0.00s)
=== RUN   TestVerifyC2_ResolvePluginConfigCollectionOrder
    resolvePluginConfig used: 4 distinct diagnostics over 200 identical builds
       83x [urn:k8s:secret:plugin-creds:apiKey urn:k8s:secret:plugin-creds:clientSecret urn:k8s:secret:redis-creds:password urn:k8s:secret:plugin-creds:token]
       37x [urn:k8s:secret:plugin-creds:clientSecret urn:k8s:secret:redis-creds:password urn:k8s:secret:plugin-creds:token urn:k8s:secret:plugin-creds:apiKey]
       47x [urn:k8s:secret:plugin-creds:token urn:k8s:secret:plugin-creds:apiKey urn:k8s:secret:plugin-creds:clientSecret urn:k8s:secret:redis-creds:password]
       33x [urn:k8s:secret:redis-creds:password urn:k8s:secret:plugin-creds:token urn:k8s:secret:plugin-creds:apiKey urn:k8s:secret:plugin-creds:clientSecret]
--- FAIL: TestVerifyC2_ResolvePluginConfigCollectionOrder (0.00s)
=== RUN   TestVerifyC2_ConstructorErrorOrdering
    constructor error: 4 distinct diagnostics over 200 identical builds
       90x plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token failed, the error is not reported as it may disclose them
       31x plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey failed, the error is not reported as it may disclose them
       45x plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password failed, the error is not reported as it may disclose them
       34x plugin: creating middleware with the values of urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret failed, the error is not reported as it may disclose them
--- FAIL: TestVerifyC2_ConstructorErrorOrdering (0.00s)
=== RUN   TestVerifyC2_BuildErrorOrdering
    build error: 4 distinct diagnostics over 200 identical builds
       97x plugin: invalid configuration with the values of urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token
       42x plugin: invalid configuration with the values of urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey
       26x plugin: invalid configuration with the values of urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password
       35x plugin: invalid configuration with the values of urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret
--- FAIL: TestVerifyC2_BuildErrorOrdering (0.00s)
=== RUN   TestVerifyC2_MiddlewareInfoErrAccumulates
    MiddlewareInfo.Err holds 4 entries after 20 routers built the same middleware
--- FAIL: TestVerifyC2_MiddlewareInfoErrAccumulates (0.00s)
=== RUN   TestVerifyC2_ControlSingleReference
    single reference: 1 distinct diagnostics over 200 identical builds
      200x plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:token failed, the error is not reported as it may disclose them
--- PASS: TestVerifyC2_ControlSingleReference (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.038s
exit=1
```

(`verify/logs/c2-repro.log`. The per-ordering counts vary from run to run; the number of distinct orderings for this configuration was 4 in every run. The repro is written so that FAIL means the problem reproduces.)

Per part:

- **Reference formatting.** `TestVerifyC2_ReferencesKeepCollectionOrder`: references resolved in the order z, a, m are reported as `z, a, m`. The formatter keeps collection order and does not sort. The branch's own `TestSecretResolver_doesNotMutateConfiguration` asserts the same unsorted collection order (`"urn:k8s:secret:tls:tls.crt, urn:k8s:secret:tls:tls.key, urn:k8s:secret:redis:username, urn:k8s:secret:redis:password"`).
- **Failing-build trigger.** `TestVerifyC2_ResolvePluginConfigCollectionOrder`: `resolvePluginConfig` on an identical configuration collects `used` in 4 different orders over 200 calls. `TestVerifyC2_ConstructorErrorOrdering`: the constructor-failure diagnostic returned by `Then()` (and stored in `MiddlewareInfo.Err`, asserted equal inside the loop) appears with 4 different orderings over 200 identical builds. The sibling path (`Build` rejects the values, accepts the references) shows the same.

### Control mutation: sort inside `references()` (`verify/patches/c2-sorted-references.diff`)

```console
$ cd ../wt-c2 && git apply ../traefik/verify/patches/c2-sorted-references.diff
$ go test -tags verify -count=1 -v -run "^TestVerifyC2_" ./pkg/server/middleware
    references() = "urn:k8s:secret:a:a, urn:k8s:secret:m:m, urn:k8s:secret:z:z"
--- FAIL: TestVerifyC2_ReferencesKeepCollectionOrder (0.00s)
    resolvePluginConfig used: 4 distinct diagnostics over 200 identical builds
--- FAIL: TestVerifyC2_ResolvePluginConfigCollectionOrder (0.00s)
    constructor error: 1 distinct diagnostics over 200 identical builds
--- PASS: TestVerifyC2_ConstructorErrorOrdering (0.00s)
    build error: 1 distinct diagnostics over 200 identical builds
--- PASS: TestVerifyC2_BuildErrorOrdering (0.00s)
    MiddlewareInfo.Err holds 1 entries after 20 routers built the same middleware
--- PASS: TestVerifyC2_MiddlewareInfoErrAccumulates (0.00s)
    single reference: 1 distinct diagnostics over 200 identical builds
--- PASS: TestVerifyC2_ControlSingleReference (0.00s)
$ go test -tags verify -count=1 -v -run "^TestVerifyC2_RouterManager" ./pkg/server/router
    distinct router error strings for the one failure: 1
--- PASS: TestVerifyC2_RouterManagerAPIErrors (0.00s)
$ git apply -R ../traefik/verify/patches/c2-sorted-references.diff
```

(`verify/logs/c2-control-sorted.log`.) With the one-line sort the collection order still varies (second test), but the emitted diagnostics no longer do. The two tests that still fail under the mutation are the ones that measure the unsorted formatter and the map-ordered collection directly.

### Repro 2: what the API-facing runtime configuration holds (`verify/repro/c2_router_manager_api_errors_test.go`)

One configuration load through `router.NewManager(...).BuildHandlers(...)`: eight routers share the one failing plugin middleware.

```console
$ cp verify/repro/c2_router_manager_api_errors_test.go ../wt-c2/pkg/server/router/
$ cd ../wt-c2 && go test -tags verify -count=1 -v -run '^TestVerifyC2_RouterManager' ./pkg/server/router
    middleware plugin@kubernetescrd as the API reports it:
        {
          "status": "disabled",
          "error": [
            "plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token failed, the error is not reported as it may disclose them",
            "plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey failed, the error is not reported as it may disclose them",
            "plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password failed, the error is not reported as it may disclose them",
            "plugin: creating middleware with the values of urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret failed, the error is not reported as it may disclose them"
          ]
        }
    router app0@kubernetescrd status=disabled error=["plugin: creating middleware with the values of urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret, urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token failed, ..."]
    router app1@kubernetescrd status=disabled error=["plugin: creating middleware with the values of urn:k8s:secret:redis-creds:password, urn:k8s:secret:plugin-creds:token, urn:k8s:secret:plugin-creds:apiKey, urn:k8s:secret:plugin-creds:clientSecret failed, ..."]
    ...
    distinct router error strings for the one failure: 4
--- FAIL: TestVerifyC2_RouterManagerAPIErrors (0.00s)
exit=1
```

(`verify/logs/c2-router-manager.log`, unabridged.) A first attempt at this probe panicked inside the pre-existing `reflect.ValueOf(b.pluginBuilder).IsNil()` check because the probe passed a struct value as the plugin builder; that was a probe bug (base code, line 408 at `af6d927c5`), fixed by passing a pointer, and is not a finding.

### Scale probe (`verify/probes/c2_ordering_scale_test.go`)

This probe was written after the repro above showed exactly 4 orderings for 4 references, to see how the count moves with the number of references; the 3 to 12 reference rows were not predicted in advance (the 2-reference row was added later with the stated expectation of 2 orderings). 2000 identical builds per row, three consecutive runs, complete output:

```console
$ cp verify/probes/c2_ordering_scale_test.go ../wt-c2/pkg/server/middleware/
$ cd ../wt-c2 && for i in 1 2 3; do go test -tags verify -count=1 -v -run '^TestVerifyC2Scale_' ./pkg/server/middleware 2>&1 | grep -E "refs=|^(ok|FAIL|---)" | sed -E 's/^ +c2_ordering_scale_test.go:[0-9]+: //'; echo "--- run $i exit=${PIPESTATUS[0]}"; done
refs=2 builds=2000 distinct_diagnostics=2 of_which_rotations_of_sorted_order=2 MiddlewareInfo.Err_entries_after_2000_shared_builds=2
refs=3 builds=2000 distinct_diagnostics=3 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=3
refs=4 builds=2000 distinct_diagnostics=4 of_which_rotations_of_sorted_order=4 MiddlewareInfo.Err_entries_after_2000_shared_builds=4
refs=8 builds=2000 distinct_diagnostics=8 of_which_rotations_of_sorted_order=8 MiddlewareInfo.Err_entries_after_2000_shared_builds=8
refs=9 builds=2000 distinct_diagnostics=1944 of_which_rotations_of_sorted_order=4 MiddlewareInfo.Err_entries_after_2000_shared_builds=1261
refs=12 builds=2000 distinct_diagnostics=1999 of_which_rotations_of_sorted_order=0 MiddlewareInfo.Err_entries_after_2000_shared_builds=1897
refs=4 builds=2000 distinct_diagnostics=6 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=6
--- PASS: TestVerifyC2Scale_DistinctOrderingsBySize (0.16s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.195s
--- run 1 exit=0
refs=2 builds=2000 distinct_diagnostics=2 of_which_rotations_of_sorted_order=2 MiddlewareInfo.Err_entries_after_2000_shared_builds=2
refs=3 builds=2000 distinct_diagnostics=3 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=3
refs=4 builds=2000 distinct_diagnostics=4 of_which_rotations_of_sorted_order=4 MiddlewareInfo.Err_entries_after_2000_shared_builds=4
refs=8 builds=2000 distinct_diagnostics=8 of_which_rotations_of_sorted_order=8 MiddlewareInfo.Err_entries_after_2000_shared_builds=8
refs=9 builds=2000 distinct_diagnostics=1952 of_which_rotations_of_sorted_order=5 MiddlewareInfo.Err_entries_after_2000_shared_builds=1225
refs=12 builds=2000 distinct_diagnostics=2000 of_which_rotations_of_sorted_order=0 MiddlewareInfo.Err_entries_after_2000_shared_builds=1901
refs=4 builds=2000 distinct_diagnostics=6 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=6
--- PASS: TestVerifyC2Scale_DistinctOrderingsBySize (0.15s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.185s
--- run 2 exit=0
refs=2 builds=2000 distinct_diagnostics=2 of_which_rotations_of_sorted_order=2 MiddlewareInfo.Err_entries_after_2000_shared_builds=2
refs=3 builds=2000 distinct_diagnostics=3 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=3
refs=4 builds=2000 distinct_diagnostics=4 of_which_rotations_of_sorted_order=4 MiddlewareInfo.Err_entries_after_2000_shared_builds=4
refs=8 builds=2000 distinct_diagnostics=8 of_which_rotations_of_sorted_order=8 MiddlewareInfo.Err_entries_after_2000_shared_builds=8
refs=9 builds=2000 distinct_diagnostics=1938 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=1276
refs=12 builds=2000 distinct_diagnostics=1999 of_which_rotations_of_sorted_order=0 MiddlewareInfo.Err_entries_after_2000_shared_builds=1897
refs=4 builds=2000 distinct_diagnostics=6 of_which_rotations_of_sorted_order=3 MiddlewareInfo.Err_entries_after_2000_shared_builds=6
--- PASS: TestVerifyC2Scale_DistinctOrderingsBySize (0.15s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.211s
--- run 3 exit=0
```

(`verify/logs/c2-scale.log`. The last `refs=4` row of each run is 2 top-level plus 2 nested references; the other rows are flat.) Observed: with 2 to 8 top-level references the number of distinct orderings equals the reference count; with 9 and 12 references nearly every build produced a new ordering. I did not investigate why the shared-configuration count is lower than the fresh-configuration count for 9 and 12 references.

### Impact reasoning

- Trigger: a CRD plugin Middleware with two or more options written as `urn:k8s:secret:name:key`, whose plugin fails to build or to create its handler (bad credentials, unreachable backend, rejected configuration). Several Secret-backed options on one plugin is an ordinary configuration; a single reference is unaffected (control).
- What the operator sees: the same failure is reported with the references in a different order on each build. Because `MiddlewareInfo.AddError` dedupes on the exact string, a middleware used by several routers accumulates one entry per ordering in the runtime configuration that `/api/http/middlewares` and `/api/rawdata` serve (4 entries for 4 references and 8 routers above), and the routers sharing it carry different error strings for the one cause. With more than 8 references the list grew with nearly every router in the probe.
- No Secret value is disclosed by this; the references are public metadata. The cost is noisy, non-reproducible diagnostics: duplicated errors on the dashboard, log lines that do not group, and string comparisons of errors across reloads that never match.
- Workaround: none from configuration, short of using at most one Secret reference per plugin.

### Existing checks (side observation, not the basis of the verdict)

```console
$ cd ../wt-c2 && go test -count=1 ./pkg/server/middleware   # branch's own tests, without the verify tag
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.064s
$ cp ~/eval_tests/tests/{eval_crd_behavior_test.go,export_eval_test.go,eval_provider_to_builder_runtime_test.go} pkg/provider/kubernetes/crd/
$ go test -race -count=5 -timeout=5m -run "^TestEval_ProviderToBuilder_Plugin(BuildErrorRedaction|CachedErrorRedaction)$" ./pkg/provider/kubernetes/crd
--- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction (0.00s)
    --- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction/build/opaque=true
    --- FAIL: TestEval_ProviderToBuilder_PluginCachedErrorRedaction/build/opaque=false
            	Error:      	"plugin build rejected credential \"eval_cached_plugin_secret_q9\"" should not contain "eval_cached_plugin_secret_q9"
            	Error:      	"plugin: plugin build failed: internal error" does not contain "urn:k8s:secret:name:key"
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	8.452s
exit=1
```

(`verify/logs/c2-existing-checks.log`.) The branch's own tests pass, so they do not observe the ordering. Every configuration load in the eval's two plugin-error fixtures uses `with_plugin_read_secret.yml`, which holds a single reference, so they cannot observe ordering either:

```console
$ awk 'NR>=371 && NR<=700' ~/eval_tests/tests/eval_provider_to_builder_runtime_test.go | grep -c 'LoadConfiguration'
7
$ awk 'NR>=371 && NR<=700' ~/eval_tests/tests/eval_provider_to_builder_runtime_test.go | grep -c 'with_plugin_read_secret.yml'
7
$ grep -n 'urn:' pkg/provider/kubernetes/crd/fixtures/with_plugin_read_secret.yml
21:      secret: urn:k8s:secret:name:key
```

Their failure on this branch is on different assertions (an error cached by the plugin mock is reported with the Secret value) and concerns a behavior that is outside C2 and was not adjudicated here.

## C3

Target: the audited branch `traefik-secret-references-in-middlewares-perfect` at `84614aa0ce1af141b9fbdeae2980b79e1f53ba73`, in `~/repos/traefik`.

Naming drift: `TestResolveSecret` and `TestNewRedisLimiterSecrets` do not exist on this branch. The solution changes no file in either package scope, but both scopes import packages it does change (`pkg/config/dynamic`, the CRD and Ingress-NGINX providers, `pkg/provider/kubernetes/k8s`), so the runs below compile the solution's code.

```console
$ git diff --stat af6d927c5..HEAD -- ":(glob)pkg/middlewares/*.go" pkg/middlewares/ratelimiter   # files of the two package scopes changed by the solution
(end of diff stat)
$ git diff --stat af6d927c5..HEAD -- pkg/middlewares   # whole subtree, for context
 pkg/middlewares/auth/auth.go             | 11 +++++++--
 pkg/middlewares/auth/basic_auth.go       |  4 +++-
 pkg/middlewares/auth/basic_auth_test.go  | 40 ++++++++++++++++++++++++++++++++
 pkg/middlewares/auth/digest_auth.go      |  5 ++--
 pkg/middlewares/auth/digest_auth_test.go |  4 ++--
 5 files changed, 57 insertions(+), 7 deletions(-)
$ go list -deps ./pkg/middlewares ./pkg/middlewares/ratelimiter/... | grep -E "traefik/v3/pkg/(middlewares/auth|config/dynamic|server/middleware|provider/kubernetes)"
github.com/traefik/traefik/v3/pkg/config/dynamic
github.com/traefik/traefik/v3/pkg/provider/kubernetes/k8s
github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd
github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx
... (generated CRD clientset packages, gateway, ingress, knative; full list in the log)
$ grep -rnE "func (TestResolveSecret|TestNewRedisLimiterSecrets)\b" pkg/
exit=1
$ go test -list '.*' ./pkg/middlewares ./pkg/middlewares/ratelimiter/...
?   	github.com/traefik/traefik/v3/pkg/middlewares	[no test files]
TestNewRateLimiter
TestInMemoryRateLimit
TestRedisRateLimit
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter	0.070s
TestNew_invalidCapacity
TestMap_setAndGet
TestMap_setInvalidTTL
TestMap_expiration
TestMap_setRefreshesExpiration
TestMap_capacityEvictsClosestToExpiry
TestMap_concurrentAccess
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter/ttlmap	0.013s
```

(`verify/logs/c3-scope.log`, `verify/logs/c3-test-list.log`)

The documented exclusion is the `-skip` pattern from the eval's regression command. Three invocations, each run alone on the machine (the rate limiter tests are timing-based):

```console
$ go version
go version go1.26.3 linux/amd64
$ git rev-parse HEAD
84614aa0ce1af141b9fbdeae2980b79e1f53ba73
$ go test -count=1 -timeout=5m -skip '^(TestForwardAuthClientClosedRequest|TestListenProvidersIgnoreSameConfig)$' ./pkg/middlewares ./pkg/middlewares/ratelimiter/...
?   	github.com/traefik/traefik/v3/pkg/middlewares	[no test files]
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter	120.044s
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter/ttlmap	0.004s
exit=0
```

```console
$ go test -parallel 8 -count=1 -timeout=5m -skip '^(TestForwardAuthClientClosedRequest|TestListenProvidersIgnoreSameConfig)$' ./pkg/middlewares ./pkg/middlewares/ratelimiter/...   # -parallel 8 is the option .github/workflows/test-unit.yaml uses
?   	github.com/traefik/traefik/v3/pkg/middlewares	[no test files]
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter	122.048s
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter/ttlmap	0.011s
exit=0
```

```console
$ go test -v -race -count=1 -timeout=10m -skip '^(TestForwardAuthClientClosedRequest|TestListenProvidersIgnoreSameConfig)$' ./pkg/middlewares ./pkg/middlewares/ratelimiter/...   # -v -race as in the eval's focused commands
?   	github.com/traefik/traefik/v3/pkg/middlewares	[no test files]
--- PASS: TestNewRateLimiter (0.00s)
--- PASS: TestInMemoryRateLimit (0.00s)
--- PASS: TestRedisRateLimit (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter	121.122s
--- PASS: TestNew_invalidCapacity (0.00s)
--- PASS: TestMap_setAndGet (0.00s)
--- PASS: TestMap_setInvalidTTL (0.00s)
--- PASS: TestMap_expiration (0.00s)
--- PASS: TestMap_setRefreshesExpiration (0.00s)
--- PASS: TestMap_capacityEvictsClosestToExpiry (0.00s)
--- PASS: TestMap_concurrentAccess (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/middlewares/ratelimiter/ttlmap	1.016s
exit=0
```

(`verify/logs/c3-test-default.log`, `c3-test-parallel8.log`, `c3-test-race-v.log`. An earlier, fourth run of the first command, made while Go modules were still downloading, also ended `ok ... ratelimiter 120.393s`, `exit=0`; its log was overwritten by the clean re-run.)

Both package scopes build; `./pkg/middlewares` has no test files; every selected test passes; no data race reported.

## C4

Target: the audited branch `traefik-secret-references-in-middlewares-perfect` at `84614aa0ce1af141b9fbdeae2980b79e1f53ba73`, in `~/repos/traefik`. `lint.md` does not exist in the repository; the repository documents no extra `go vet` options (the task's lint guideline is plain `go vet <packages>`).

```console
$ git rev-parse HEAD
84614aa0ce1af141b9fbdeae2980b79e1f53ba73
$ go vet ./pkg/middlewares ./pkg/middlewares/ratelimiter/...
exit=0
$ go build ./pkg/middlewares ./pkg/middlewares/ratelimiter/...
build exit=0
$ gofmt -l pkg/middlewares/extractor.go pkg/middlewares/handler_switcher.go pkg/middlewares/middleware.go pkg/middlewares/response_modifier.go pkg/middlewares/stateful.go pkg/middlewares/ratelimiter
gofmt exit=0
```

(`verify/logs/c4-vet.log`; the same command also exited 0 with no output in an earlier run.) `go vet` exits 0 and prints no diagnostic and no package-loading error for either scope. The solution changes no file in these two scopes, though both import packages it does change, which `go vet` had to load and type-check:

```console
$ git diff --stat af6d927c5..HEAD -- ":(glob)pkg/middlewares/*.go" pkg/middlewares/ratelimiter   # files of the two package scopes changed by the solution
(end of diff stat)
$ go list -deps ./pkg/middlewares ./pkg/middlewares/ratelimiter/... | grep -E "traefik/v3/pkg/(middlewares/auth|config/dynamic|server/middleware|provider/kubernetes)"
github.com/traefik/traefik/v3/pkg/config/dynamic
github.com/traefik/traefik/v3/pkg/provider/kubernetes/k8s
github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd
github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx
... (full list in `verify/logs/c3-scope.log`)
```
