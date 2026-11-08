# Evidence — audit v-a002ef68

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

## C1

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-436d5b5a](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-436d5b5a) (worktree `~/wt/436d5b5a`). **Verdict: REFUTED** (both parts).

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Assertion text of the added tests (unchanged files on the branch):

- `pkg/server/middleware/secrets_test.go` `TestBuilder_pluginWithSecrets` (plugin middleware): whole-object expectation on the production resolver output received by the plugin builder, holding the resolved Secret values (`"this_is_the_token"`, nested `"admin"`), the non-secret sibling `"count": 42` and the literal list sibling `"plain"`, followed by a whole-object check that the published config still holds the references:

```go
	rawConfig := func() dynamic.PluginConf {
		return dynamic.PluginConf{
			"token": tokenRef,
			"users": []any{map[string]any{"name": userRef}, "plain"},
			"count": 42,
		}
	}
	...
	assert.Equal(t, map[string]any{
		"token": "this_is_the_token",
		"users": []any{map[string]any{"name": "admin"}, "plain"},
		"count": 42,
	}, pluginBuilder.configs[0])

	// The configuration exposed by the API must still hold the references.
	assert.Equal(t, map[string]dynamic.PluginConf{"test": rawConfig()}, config.Plugin)
```

- `pkg/server/middleware/secrets_test.go` `TestBuilder_basicAuthWithSecrets` (basicAuth middleware): endpoint-specific request evidence — the handler built through `BuildMiddlewareChain` answers `200` for both users read from the Secret (`test`/`test2`) and `401` for a wrong password, while `assert.Equal(t, dynamic.Users{usersRef}, config.BasicAuth.Users)` checks the configured (non-secret) published value.
- `pkg/config/dynamic/secrets_test.go` `TestSecrets_ResolveAll` (shared production resolver called by `resolveBasicAuth` and `resolveDigestAuth` in `pkg/server/middleware/secrets.go`): `assert.Equal(t, []string{"admin:hash", "value"}, values)` — resolved Secret value next to the literal sibling `"admin:hash"`.
- `pkg/config/dynamic/secrets_test.go` `TestSecrets_ResolveAny`: nested leaf `"users": []any{map[string]any{"name": "admin", "secret": "value2"}, "plain"}` resolved inside a map inside a list, with non-secret siblings.
- `TestBuilder_digestAuthWithSecrets`, `TestBuilder_forwardAuthWithSecrets`, `TestBuilder_rateLimitWithSecrets` additionally assert the published references are untouched after a successful build.

Demonstration run (clean branch):

```console
$ cd ~/wt/436d5b5a && go test -count=1 -v -run '^(TestSecrets_ResolveAll|TestSecrets_ResolveAny|TestBuilder_basicAuthWithSecrets|TestBuilder_digestAuthWithSecrets|TestBuilder_forwardAuthWithSecrets|TestBuilder_rateLimitWithSecrets|TestBuilder_pluginWithSecrets)$' ./pkg/config/dynamic/ ./pkg/server/middleware/
--- PASS: TestSecrets_ResolveAll (0.00s)
--- PASS: TestSecrets_ResolveAny (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.010s
--- PASS: TestBuilder_basicAuthWithSecrets (0.00s)
--- PASS: TestBuilder_digestAuthWithSecrets (0.00s)
--- PASS: TestBuilder_forwardAuthWithSecrets (0.00s)
--- PASS: TestBuilder_rateLimitWithSecrets (0.00s)
--- PASS: TestBuilder_pluginWithSecrets (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.026s
```

Mutation check that the assertions really observe resolved values (temporary, reverted afterwards — `resolveDigestAuth` returns users unresolved, `resolveRateLimit` skips Redis username/password):

```diff
diff --git a/pkg/server/middleware/secrets.go b/pkg/server/middleware/secrets.go
index 2c4140a8e..f127a40f0 100644
--- a/pkg/server/middleware/secrets.go
+++ b/pkg/server/middleware/secrets.go
@@ -24,14 +24,7 @@ func resolveBasicAuth(secrets dynamic.Secrets, config dynamic.BasicAuth) (dynami
 }
 
 func resolveDigestAuth(secrets dynamic.Secrets, config dynamic.DigestAuth) (dynamic.DigestAuth, error) {
-	users, err := secrets.ResolveAll(config.Users)
-	if err != nil {
-		return dynamic.DigestAuth{}, fmt.Errorf("resolving users: %w", err)
-	}
-
-	config.Users = users
-
-	return config, nil
+	return config, nil // MUTANT C1: digest users left unresolved.
 }
 
 func resolveForwardAuth(secrets dynamic.Secrets, config dynamic.ForwardAuth) (dynamic.ForwardAuth, error) {
@@ -67,13 +60,7 @@ func resolveRateLimit(secrets dynamic.Secrets, config dynamic.RateLimit) (dynami
 	redis := *config.Redis
 
 	var err error
-	if redis.Username, err = secrets.Resolve(redis.Username); err != nil {
-		return dynamic.RateLimit{}, fmt.Errorf("resolving Redis username: %w", err)
-	}
-
-	if redis.Password, err = secrets.Resolve(redis.Password); err != nil {
-		return dynamic.RateLimit{}, fmt.Errorf("resolving Redis password: %w", err)
-	}
+	// MUTANT C1: Redis username/password left unresolved.
 
 	if redis.TLS != nil {
 		clientTLS := *redis.TLS
```

```console
$ cd ~/wt/436d5b5a && go test -count=1 -v -run '^(TestBuilder_digestAuthWithSecrets|TestBuilder_rateLimitWithSecrets|TestBuilder_pluginWithSecrets|TestBuilder_basicAuthWithSecrets)$' ./pkg/server/middleware/   # with the mutant applied
--- PASS: TestBuilder_basicAuthWithSecrets (0.00s)
    secrets_test.go:135: 
        	Error Trace:	/home/ubuntu/wt/436d5b5a/pkg/server/middleware/secrets_test.go:135
        	Error:      	Received unexpected error:
        	            	user entry 1: error parsing DigestUser: expected 3 fields separated by ':', got 5
        	Test:       	TestBuilder_digestAuthWithSecrets
--- FAIL: TestBuilder_digestAuthWithSecrets (0.00s)
    secrets_test.go:217: 
        	Error Trace:	/home/ubuntu/wt/436d5b5a/pkg/server/middleware/secrets_test.go:217
        	Error:      	An error is expected but got nil.
        	Test:       	TestBuilder_rateLimitWithSecrets
--- FAIL: TestBuilder_rateLimitWithSecrets (0.00s)
--- PASS: TestBuilder_pluginWithSecrets (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
FAIL
```

Conclusion: *middleware_breadth* — at least plugin and basicAuth (and digestAuth via the shared `ResolveAll` resolver) carry assertions on a resolved Secret value together with a non-secret sibling; *nested_leaf_coverage* — `TestBuilder_pluginWithSecrets` and `TestSecrets_ResolveAny` assert a resolved value at a nested map-in-list leaf. Both parts refuted.

## C2

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-67c2dfcc](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-67c2dfcc) (worktree `~/wt/67c2dfcc`). **Verdict: CONFIRMED.**

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Production code under test (`loadBasicAuthCredentials` interpolates the raw username bytes into the single stored user entry; `Secrets.ResolveUsers` then splits the Secret value on `\n`):

```console
$ sed -n "/func loadBasicAuthCredentials/,/^}/p" pkg/provider/kubernetes/crd/kubernetes.go
func loadBasicAuthCredentials(secret *corev1.Secret, secrets dynamic.Secrets) (dynamic.Users, error) {
	username, usernameExists := secret.Data["username"]
	password, passwordExists := secret.Data["password"]
	if !(usernameExists && passwordExists) {
		return nil, fmt.Errorf("secret '%s/%s' must contain both username and password keys", secret.Namespace, secret.Name)
	}

	hash := sha1.New()
	hash.Write(password)
	passwordSum := base64.StdEncoding.EncodeToString(hash.Sum(nil))

	ref := dynamic.SecretRef(secret.Name, corev1.BasicAuthPasswordKey)
	secrets[ref] = fmt.Sprintf("%s:{SHA}%s", username, passwordSum)

	return dynamic.Users{ref}, nil
}

$ grep -n "func (s Secrets) ResolveUsers" -A22 pkg/config/dynamic/secrets.go
65:func (s Secrets) ResolveUsers(users Users) ([]string, []string) {
66-	var resolved, locations []string
67-	for i, user := range users {
68-		value, ok := s.lookup(user)
69-		if !ok {
70-			resolved = append(resolved, user)
71-			locations = append(locations, "at index "+strconv.Itoa(i))
72-			continue
73-		}
74-
75-		for j, line := range strings.Split(value, "\n") {
76-			resolved = append(resolved, line)
77-			locations = append(locations, "at line "+strconv.Itoa(j+1)+" of "+user)
78-		}
79-	}
80-
81-	return resolved, locations
82-}
83-
84-func (s Secrets) lookup(v string) (string, bool) {
85-	if !IsSecretRef(v) {
86-		return "", false
87-	}
```

Repro `verify/repro/c2_typed_basicauth_newline_test.go` (+ companion `verify/repro/c2_export_verify.go`, which only exports the unexported provider helper so the probe can live in `pkg/server/middleware` and avoid an import cycle). The typed Secret has `username = "mallory:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\nalice"` (hash of `test`) and `password = "alicepw"`:

```console
$ cd ~/wt/67c2dfcc && cp ~/repos/traefik/verify/repro/c2_export_verify.go pkg/provider/kubernetes/crd/ && cp ~/repos/traefik/verify/repro/c2_typed_basicauth_newline_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC2 ./pkg/server/middleware/
    c2_typed_basicauth_newline_test.go:38: provider users=["urn:k8s:secret:typed:password"]
    c2_typed_basicauth_newline_test.go:39: provider secrets=map["urn:k8s:secret:typed:password":"mallory:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\nalice:{SHA}pXcenXUnxGz6jD4dFmSXV63E49g="]
    c2_typed_basicauth_newline_test.go:46: build err=<nil> infoErr=[]
    c2_typed_basicauth_newline_test.go:54: alice/alicepw -> 200
    c2_typed_basicauth_newline_test.go:54: mallory/wrong -> 401
    c2_typed_basicauth_newline_test.go:54: mallory/test -> 200
    c2_typed_basicauth_newline_test.go:61: 
        	Error Trace:	/home/ubuntu/wt/67c2dfcc/pkg/server/middleware/c2_typed_basicauth_newline_test.go:61
        	Error:      	Not equal: 
        	            	expected: 401
        	            	actual  : 200
        	Test:       	TestVerifyC2_TypedBasicAuthNewlineInjection
        	Messages:   	mallory must NOT be able to authenticate
--- FAIL: TestVerifyC2_TypedBasicAuthNewlineInjection (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.024s
FAIL
```

`mallory/test -> 200`: the injected account authenticates. The byte-exact eval fixtures for typed BasicAuth pass on this branch (they do not cover a newline in the username):

```console
$ cd ~/wt/67c2dfcc && cp ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/eval_crd_behavior_test.go ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/export_eval_test.go ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -timeout=5m -v -run '^TestEval_(CRD_TypedBasicAuth_SecretRetention|ProviderToBuilder_TypedBasicAuthLateBinding)$' ./pkg/provider/kubernetes/crd
--- PASS: TestEval_ProviderToBuilder_TypedBasicAuthLateBinding (0.05s)
--- PASS: TestEval_CRD_TypedBasicAuth_SecretRetention (0.06s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.084s
```

## C3

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-d2f31b62](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-d2f31b62) (worktree `~/wt/d2f31b62`). **Verdict: CONFIRMED.**

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Note: the claim names `pkg/config/dynamic/urnrefs.go: RedactURNRefs`; on this branch the equivalent is `redactSecretValues` in `pkg/server/middleware/secrets.go` (naming drift). It rebuilds the plugin with random placeholder values and, when the placeholder-substituted retry error equals the original error text, returns that text with placeholders swapped for references:

```go
	if placeholderErr := build(conf.ResolveSecretReferences(placeholders)); placeholderErr != nil {
		withValues := placeholderErr.Error()
		withReferences := placeholderErr.Error()
		for _, reference := range references {
			withValues = strings.ReplaceAll(withValues, placeholders[reference], conf.SecretValues[reference])
			withReferences = strings.ReplaceAll(withReferences, placeholders[reference], reference)
		}

		if withValues == err.Error() {
			return errors.New(withReferences)
		}
	}
```

A constructor that caches its first (Secret-bearing) error returns the identical text on the retry, so `withValues == err.Error()` holds while the text contains no placeholder — the resolved Secret is returned verbatim. Repro `verify/repro/c3_cached_plugin_error_test.go` (Secret `urn:k8s:secret:plugin:token` = `hunter2-very-secret`; one builder caches the `Build` error, one caches the constructor error):

```console
$ cd ~/wt/d2f31b62 && cp ~/repos/traefik/verify/repro/c3_cached_plugin_error_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC3 ./pkg/server/middleware/
    c3_cached_plugin_error_test.go:73: Build calls=2
    c3_cached_plugin_error_test.go:74: returned err: plugin: failed to decode configuration: invalid token "hunter2-very-secret"
    c3_cached_plugin_error_test.go:75: MiddlewareInfo.Err: ["plugin: failed to decode configuration: invalid token \"hunter2-very-secret\""]
    c3_cached_plugin_error_test.go:76: 
        	Error Trace:	/home/ubuntu/wt/d2f31b62/pkg/server/middleware/c3_cached_plugin_error_test.go:76
        	Error:      	"plugin: failed to decode configuration: invalid token \"hunter2-very-secret\"" should not contain "hunter2-very-secret"
        	Test:       	TestVerifyC3_CachedBuildError
    c3_cached_plugin_error_test.go:78: 
        	Error Trace:	/home/ubuntu/wt/d2f31b62/pkg/server/middleware/c3_cached_plugin_error_test.go:78
        	Error:      	"plugin: failed to decode configuration: invalid token \"hunter2-very-secret\"" should not contain "hunter2-very-secret"
        	Test:       	TestVerifyC3_CachedBuildError
--- FAIL: TestVerifyC3_CachedBuildError (0.00s)
    c3_cached_plugin_error_test.go:85: constructor calls=2
    c3_cached_plugin_error_test.go:86: returned err: plugin init failed: bad token "hunter2-very-secret"
    c3_cached_plugin_error_test.go:87: MiddlewareInfo.Err: ["plugin init failed: bad token \"hunter2-very-secret\""]
    c3_cached_plugin_error_test.go:88: 
        	Error Trace:	/home/ubuntu/wt/d2f31b62/pkg/server/middleware/c3_cached_plugin_error_test.go:88
        	Error:      	"plugin init failed: bad token \"hunter2-very-secret\"" should not contain "hunter2-very-secret"
        	Test:       	TestVerifyC3_CachedConstructorError
    c3_cached_plugin_error_test.go:90: 
        	Error Trace:	/home/ubuntu/wt/d2f31b62/pkg/server/middleware/c3_cached_plugin_error_test.go:90
        	Error:      	"plugin init failed: bad token \"hunter2-very-secret\"" should not contain "hunter2-very-secret"
        	Test:       	TestVerifyC3_CachedConstructorError
--- FAIL: TestVerifyC3_CachedConstructorError (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
FAIL
```

Both the returned error and `MiddlewareInfo.Err` contain `hunter2-very-secret`. The byte-exact eval fixture for plugin error redaction (which does not model a cached error) passes on this branch:

```console
$ cd ~/wt/d2f31b62 && cp ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/export_eval_test.go ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -timeout=5m -v -run '^TestEval_ProviderToBuilder_PluginBuildErrorRedaction$' ./pkg/provider/kubernetes/crd
--- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (0.81s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build (0.32s)
        --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false (0.17s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/unrelated_baseline (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/type_label_collision_(string) (0.03s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/field_label_collision_(token) (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/escaped_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/single-line_quote_and_backslash_escaping (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/whitespace-only_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false/empty_secret_value (0.03s)
        --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true (0.15s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/unrelated_baseline (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/type_label_collision_(string) (0.03s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/field_label_collision_(token) (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/escaped_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/single-line_quote_and_backslash_escaping (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/whitespace-only_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true/empty_secret_value (0.00s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor (0.34s)
        --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false (0.17s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/unrelated_baseline (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/type_label_collision_(string) (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/field_label_collision_(token) (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/escaped_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/single-line_quote_and_backslash_escaping (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/whitespace-only_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=false/empty_secret_value (0.03s)
        --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true (0.17s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/unrelated_baseline (0.03s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/type_label_collision_(string) (0.03s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/field_label_collision_(token) (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/escaped_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/single-line_quote_and_backslash_escaping (0.03s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/whitespace-only_payload (0.02s)
            --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor/constant_error=true/empty_secret_value (0.02s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build_stage_wrapped_cause (0.05s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor_stage_wrapped_cause (0.05s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.840s
```

## C4

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-374ebb45](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-374ebb45) (worktree `~/wt/374ebb45`). **Verdict: CONFIRMED** (both parts: *list_element_isolation* and *error_retry_isolation*).

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Production path (`pkg/server/middleware/middlewares.go`): `config := midInfo.Middleware.ResolveSecrets()` returns the published pointer itself when the middleware has no Secret values (`if m == nil || len(m.SecretValues) == 0 { return m }` in `pkg/config/dynamic/secrets.go`), and on a Secret-bearing error `pluginBuildError` retries with `findPluginConfig(midInfo.Plugin)` — the published map:

```go
		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
		if err != nil {
			return nil, fmt.Errorf("plugin: %w", b.pluginBuildError(err, midInfo.Plugin, secrets, middlewareName))
		}
...
	pluginType, rawPluginConfig, findErr := findPluginConfig(plugin)
	...
	if _, refErr := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName); refErr != nil {
		return refErr
	}
```

Repro `verify/repro/c4_plugin_isolation_test.go` snapshots `published.Plugin` as JSON, runs a builder that does `list[0] = "REPLACED-BY-PLUGIN"` on its input (with and without a Secret reference in the config) and a builder that fails with a Secret-bearing error then mutates the retry input:

```console
$ cd ~/wt/374ebb45 && cp ~/repos/traefik/verify/repro/c4_plugin_isolation_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC4 ./pkg/server/middleware/
    c4_plugin_isolation_test.go:77: build err=<nil>
    c4_plugin_isolation_test.go:77: published before: {"test":{"list":["a","b"],"token":"urn:k8s:secret:plugin:token"}}
    c4_plugin_isolation_test.go:77: published after:  {"test":{"list":["a","b"],"token":"urn:k8s:secret:plugin:token"}}
--- PASS: TestVerifyC4_ListElementIsolation_WithSecrets (0.00s)
    c4_plugin_isolation_test.go:81: build err=<nil>
    c4_plugin_isolation_test.go:81: published before: {"test":{"list":["a","b"],"token":"literal"}}
    c4_plugin_isolation_test.go:81: published after:  {"test":{"list":["REPLACED-BY-PLUGIN","b"],"token":"literal"}}
    c4_plugin_isolation_test.go:81: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c4_plugin_isolation_test.go:73
        	            				/home/ubuntu/wt/374ebb45/pkg/server/middleware/c4_plugin_isolation_test.go:81
        	Error:      	Not equal: 
        	            	expected: map[string]interface {}{"test":map[string]interface {}{"list":[]interface {}{"a", "b"}, "token":"literal"}}
        	            	actual  : map[string]interface {}{"test":map[string]interface {}{"list":[]interface {}{"REPLACED-BY-PLUGIN", "b"}, "token":"literal"}}
        	            	
        	            	Diff:
        	            	--- Expected
        	            	+++ Actual
        	            	@@ -3,3 +3,3 @@
        	            	   (string) (len=4) "list": ([]interface {}) (len=2) {
        	            	-   (string) (len=1) "a",
        	            	+   (string) (len=18) "REPLACED-BY-PLUGIN",
        	            	    (string) (len=1) "b"
        	Test:       	TestVerifyC4_ListElementIsolation_WithoutSecrets
        	Messages:   	published plugin configuration must not change
--- FAIL: TestVerifyC4_ListElementIsolation_WithoutSecrets (0.00s)
    c4_plugin_isolation_test.go:86: build err=plugin: invalid token "MUTATED-BY-RETRY"
    c4_plugin_isolation_test.go:86: published before: {"test":{"list":["a","b"],"token":"urn:k8s:secret:plugin:token"}}
    c4_plugin_isolation_test.go:86: published after:  {"test":{"list":["REPLACED-BY-RETRY","b"],"token":"MUTATED-BY-RETRY"}}
    c4_plugin_isolation_test.go:86: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c4_plugin_isolation_test.go:73
        	            				/home/ubuntu/wt/374ebb45/pkg/server/middleware/c4_plugin_isolation_test.go:86
        	Error:      	Not equal: 
        	            	expected: map[string]interface {}{"test":map[string]interface {}{"list":[]interface {}{"a", "b"}, "token":"urn:k8s:secret:plugin:token"}}
        	            	actual  : map[string]interface {}{"test":map[string]interface {}{"list":[]interface {}{"REPLACED-BY-RETRY", "b"}, "token":"MUTATED-BY-RETRY"}}
        	            	
        	            	Diff:
        	            	--- Expected
        	            	+++ Actual
        	            	@@ -3,6 +3,6 @@
        	            	   (string) (len=4) "list": ([]interface {}) (len=2) {
        	            	-   (string) (len=1) "a",
        	            	+   (string) (len=17) "REPLACED-BY-RETRY",
        	            	    (string) (len=1) "b"
        	            	   },
        	            	-  (string) (len=5) "token": (string) (len=27) "urn:k8s:secret:plugin:token"
        	            	+  (string) (len=5) "token": (string) (len=16) "MUTATED-BY-RETRY"
        	            	  }
        	Test:       	TestVerifyC4_ErrorRetryIsolation
        	Messages:   	published plugin configuration must not change
    c4_plugin_isolation_test.go:87: Build calls=2
--- FAIL: TestVerifyC4_ErrorRetryIsolation (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
FAIL
```

Observed: with a Secret reference the deep copy isolates the list (`WithSecrets` passes); a plugin config without Secret references is handed to the builder by reference and the published list element becomes `REPLACED-BY-PLUGIN` (*list_element_isolation* confirmed); the Secret-error retry hands the published map to the builder, and the published config ends up `{"list":["REPLACED-BY-RETRY","b"],"token":"MUTATED-BY-RETRY"}` while `Build calls=2` (*error_retry_isolation* confirmed). The byte-exact eval fixture passes on this branch because its fixture config holds a Secret reference (deep-copied path) and its builder does not mutate the retry input:

```console
$ cd ~/wt/374ebb45 && cp ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/export_eval_test.go ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -timeout=5m -v -run '^TestEval_ProviderToBuilder_PluginInputMutationIsolation$' ./pkg/provider/kubernetes/crd
--- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation (0.00s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/build_error (0.06s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/success (0.06s)
    --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation/constructor_error (0.06s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.089s
```

## C5

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-374ebb45](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-374ebb45) (worktree `~/wt/374ebb45`). **Verdict: CONFIRMED.**

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Production path (`pkg/server/middleware/middlewares.go`): every construction error passes through `redactSecretsError`, which hides the whole message whenever any Secret value is a substring of it:

```go
func redactSecretsError(err error, secrets dynamic.SecretValues) error {
	if err == nil || !secrets.ContainedIn(err.Error()) {
		return err
	}

	return fmt.Errorf("error hidden as it discloses the value of one of the secrets %v", secrets.References())
}
```

Repro `verify/repro/c5_auth_error_position_test.go` builds BasicAuth and DigestAuth from `urn:k8s:secret:authsecret:users` whose value is exactly `user`, `format`, or the control `zzz-unrelated`, and checks the returned error and `MiddlewareInfo.Err` for `entry 1`:

```console
$ cd ~/wt/374ebb45 && cp ~/repos/traefik/verify/repro/c5_auth_error_position_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC5 ./pkg/server/middleware/
    c5_auth_error_position_test.go:33: returned err: error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]
    c5_auth_error_position_test.go:34: MiddlewareInfo.Err: ["error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]"]
    c5_auth_error_position_test.go:35: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:35
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/basic/user
        	Messages:   	returned error must keep the failing entry position
    c5_auth_error_position_test.go:37: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:37
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/basic/user
        	Messages:   	MiddlewareInfo.Err must keep the failing entry position
    c5_auth_error_position_test.go:33: returned err: error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]
    c5_auth_error_position_test.go:34: MiddlewareInfo.Err: ["error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]"]
    c5_auth_error_position_test.go:35: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:35
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/digest/user
        	Messages:   	returned error must keep the failing entry position
    c5_auth_error_position_test.go:37: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:37
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/digest/user
        	Messages:   	MiddlewareInfo.Err must keep the failing entry position
    c5_auth_error_position_test.go:33: returned err: error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]
    c5_auth_error_position_test.go:34: MiddlewareInfo.Err: ["error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]"]
    c5_auth_error_position_test.go:35: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:35
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/basic/format
        	Messages:   	returned error must keep the failing entry position
    c5_auth_error_position_test.go:37: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:37
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/basic/format
        	Messages:   	MiddlewareInfo.Err must keep the failing entry position
    c5_auth_error_position_test.go:33: returned err: error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]
    c5_auth_error_position_test.go:34: MiddlewareInfo.Err: ["error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]"]
    c5_auth_error_position_test.go:35: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:35
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/digest/format
        	Messages:   	returned error must keep the failing entry position
    c5_auth_error_position_test.go:37: 
        	Error Trace:	/home/ubuntu/wt/374ebb45/pkg/server/middleware/c5_auth_error_position_test.go:37
        	Error:      	"error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]" does not contain "entry 1"
        	Test:       	TestVerifyC5_AuthErrorKeepsEntryPosition/digest/format
        	Messages:   	MiddlewareInfo.Err must keep the failing entry position
    c5_auth_error_position_test.go:33: returned err: invalid user entry 1: error parsing BasicUser: expected the name:hash format
    c5_auth_error_position_test.go:34: MiddlewareInfo.Err: ["invalid user entry 1: error parsing BasicUser: expected the name:hash format"]
    c5_auth_error_position_test.go:33: returned err: invalid user entry 1: error parsing DigestUser: expected the name:realm:hash format
    c5_auth_error_position_test.go:34: MiddlewareInfo.Err: ["invalid user entry 1: error parsing DigestUser: expected the name:realm:hash format"]
--- FAIL: TestVerifyC5_AuthErrorKeepsEntryPosition (0.00s)
    --- FAIL: TestVerifyC5_AuthErrorKeepsEntryPosition/basic/user (0.00s)
    --- FAIL: TestVerifyC5_AuthErrorKeepsEntryPosition/digest/user (0.00s)
    --- FAIL: TestVerifyC5_AuthErrorKeepsEntryPosition/basic/format (0.00s)
    --- FAIL: TestVerifyC5_AuthErrorKeepsEntryPosition/digest/format (0.00s)
    --- PASS: TestVerifyC5_AuthErrorKeepsEntryPosition/basic/zzz-unrelated (0.00s)
    --- PASS: TestVerifyC5_AuthErrorKeepsEntryPosition/digest/zzz-unrelated (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
FAIL
```

Observed: for `user` and `format` both diagnostics collapse to `error hidden as it discloses the value of one of the secrets [urn:k8s:secret:authsecret:users]` (the parser message `invalid user entry 1: error parsing BasicUser: expected the name:hash format` contains the words `user`/`format`), while the control keeps `invalid user entry 1: ...`. The byte-exact eval fixtures pass on this branch because they place `user`/`format` inside a multi-line Secret whose full value never appears in the message:

```console
$ cd ~/wt/374ebb45 && cp ~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/eval_auth_sanitation_test.go pkg/middlewares/auth/ && go test -count=1 -timeout=5m -v -run '^TestEval_(GetUsers_ErrorReportsPositionWithoutLeakingCredentials|ProviderToBuilder_BasicAuthSecretEntryPosition|ProviderToBuilder_DigestAuthSecretEntryPosition)$' ./pkg/middlewares/auth ./pkg/provider/kubernetes/crd
--- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/file (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/inline (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/digest_file (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/digest_inline (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.026s
--- PASS: TestEval_ProviderToBuilder_DigestAuthSecretEntryPosition (0.19s)
    --- PASS: TestEval_ProviderToBuilder_DigestAuthSecretEntryPosition/badsentinel (0.11s)
    --- PASS: TestEval_ProviderToBuilder_DigestAuthSecretEntryPosition/user (0.03s)
    --- PASS: TestEval_ProviderToBuilder_DigestAuthSecretEntryPosition/format (0.05s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthSecretEntryPosition (0.21s)
    --- PASS: TestEval_ProviderToBuilder_BasicAuthSecretEntryPosition/badsentinel (0.11s)
    --- PASS: TestEval_ProviderToBuilder_BasicAuthSecretEntryPosition/user (0.05s)
    --- PASS: TestEval_ProviderToBuilder_BasicAuthSecretEntryPosition/format (0.05s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.240s
```

## C6

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-682bd5b0](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-682bd5b0) (worktree `~/wt/682bd5b0`). **Verdict: CONFIRMED.**

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Note: the claim names `TestBuilder_BuildMiddlewareChain_pluginURNRefs`; on this branch the authored immutability assertion is in `pkg/server/middleware/secrets_test.go` `TestBuilder_buildConstructor_pluginSecrets` (naming drift). The runtime config is built from `test.middleware` itself, so both operands of the assertion dereference the same `*dynamic.Middleware` and the same `PluginConf` map:

```go
			rtConf := runtime.NewConfig(dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{
					Middlewares: map[string]*dynamic.Middleware{"plugin": test.middleware},
				},
			})
			...
			// The shared configuration keeps the reference.
			assert.Equal(t, test.middleware.Plugin["test"]["token"], rtConf.Middlewares["plugin"].Plugin["test"]["token"])
```

Authored test on the clean branch:

```console
$ cd ~/wt/682bd5b0 && go test -count=1 -v -run 'TestBuilder_buildConstructor_pluginSecrets' ./pkg/server/middleware/
--- PASS: TestBuilder_buildConstructor_pluginSecrets (0.00s)
    --- PASS: TestBuilder_buildConstructor_pluginSecrets/accepted_value_read_from_a_Secret (0.00s)
    --- PASS: TestBuilder_buildConstructor_pluginSecrets/rejected_value_read_from_a_Secret (0.00s)
    --- PASS: TestBuilder_buildConstructor_pluginSecrets/rejected_literal_value (0.00s)
    --- PASS: TestBuilder_buildConstructor_pluginSecrets/rejected_value_read_from_a_Secret,_while_the_reference_is_accepted (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.024s
```

Repro `verify/repro/c6_plugin_assertion_aliasing_test.go` reproduces the "accepted value read from a Secret" case, overwrites the published map entry with the resolved value right before the assertion, then runs the authored assertion verbatim and an assertion against an independent expected value:

```console
$ cd ~/wt/682bd5b0 && cp ~/repos/traefik/verify/repro/c6_plugin_assertion_aliasing_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC6 ./pkg/server/middleware/
    c6_plugin_assertion_aliasing_test.go:41: same *dynamic.Middleware pointer: true
    c6_plugin_assertion_aliasing_test.go:42: expected operand (test.middleware...): "expected-token"
    c6_plugin_assertion_aliasing_test.go:43: actual operand (rtConf...):           "expected-token"
    c6_plugin_assertion_aliasing_test.go:49: 
        	Error Trace:	/home/ubuntu/wt/682bd5b0/pkg/server/middleware/c6_plugin_assertion_aliasing_test.go:49
        	Error:      	Not equal: 
        	            	expected: "urn:k8s:secret:plugin:token"
        	            	actual  : "expected-token"
        	            	
        	            	Diff:
        	            	--- Expected
        	            	+++ Actual
        	            	@@ -1 +1 @@
        	            	-urn:k8s:secret:plugin:token
        	            	+expected-token
        	Test:       	TestVerifyC6_AuthoredAssertionSurvivesSharedMapMutation
        	Messages:   	independent expectation
--- FAIL: TestVerifyC6_AuthoredAssertionSurvivesSharedMapMutation (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.024s
FAIL
```

Observed: `same *dynamic.Middleware pointer: true`; both operands read `"expected-token"` after the mutation; the authored assertion (line 46) produces no failure, only the independent expectation (line 49) fails.

## C7

**Target:** [https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-3e8db7d3](https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares/tree/evalon/traefik-se-3e8db7d3) (worktree `~/wt/3e8db7d3`). **Verdict: CONFIRMED.**

Setup shared by every section (repeated here so the section is self-contained): the six claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalon`) into `~/wt/<8-hex>` worktrees with `git fetch evalon evalon/traefik-se-<id> && git worktree add ~/wt/<id> FETCH_HEAD`; repro files live in `verify/repro/` on this branch and are copied into the target package before running with `go test -tags verify`; eval fixtures are the byte-exact files from `eval_tests.zip` (~/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests/) copied to the package path given by their `package` clause. Go: `go1.26.3 linux/amd64`.

Note: the claim names `pkg/config/dynamic/urnrefs_test.go: TestMiddleware_URNRefs_hash`; no `urnrefs*` file exists on this branch (`ls pkg/config/dynamic | grep -i urn` is empty); the Secret tests are in `pkg/config/dynamic/secrets_test.go`. Inspection of every added/modified test file:

```console
$ git diff --stat af6d927c HEAD -- "*_test.go"
 pkg/config/dynamic/secrets_test.go                 | 227 +++++++++++++++++++++
 pkg/middlewares/auth/basic_auth_test.go            |  15 ++
 pkg/middlewares/auth/digest_auth_test.go           |  15 ++
 pkg/provider/kubernetes/crd/kubernetes_test.go     | 149 ++++++++++++--
 .../kubernetes/ingress-nginx/kubernetes_test.go    | 125 +++++++++---
 pkg/server/middleware/secrets_test.go              | 147 +++++++++++++
 6 files changed, 634 insertions(+), 44 deletions(-)

$ git diff af6d927c HEAD -- "*_test.go" | grep -n -i "hashstructure\|Hash(\|reload\|ProviderChangeHash"
(exit 1)

$ grep -n "^+func Test" <(git diff af6d927c HEAD -- "*_test.go")
+func TestMiddleware_ResolveSecrets(t *testing.T) {
+func TestMiddleware_ResolveSecrets_WithoutSecrets(t *testing.T) {
+func TestMiddleware_SecretsAreNotMarshaled(t *testing.T) {
+func TestMiddleware_DeepCopySecrets(t *testing.T) {
+func TestSecretReference(t *testing.T) {
+func TestBasicAuthMalformedUserError(t *testing.T) {
+func TestDigestAuthMalformedUserError(t *testing.T) {
+func TestLoadIngressRoutes_SecretValuesNotExposed(t *testing.T) {
+func TestLoadConfiguration_AuthSecretReferences(t *testing.T) {
+func TestBuilder_ResolveSecrets(t *testing.T) {
+func TestBuilder_ResolveSecrets_UnknownReference(t *testing.T) {
+func TestBuilder_PluginErrorDoesNotRevealSecretValues(t *testing.T) {
```

No added or modified test mentions hashing or reloading; the added tests only assert that Secret values are absent from serialized configuration and are not written back by the builder. Demonstration run `verify/repro/c7_secret_only_change_mutant.sh`: a mutant that excludes `Middleware.Secrets` from `hashstructure` (`hash:"ignore"`), which makes a Secret-only change invisible to the providers' `hashstructure.Hash(conf, nil)` change detection in `pkg/provider/kubernetes/crd/kubernetes.go:159` and `pkg/provider/kubernetes/ingress-nginx/kubernetes.go:224`, keeps all authored tests green while the byte-exact eval fixture catches it:

```console
$ cd ~/wt/3e8db7d3 && sh ~/repos/traefik/verify/repro/c7_secret_only_change_mutant.sh
-- added/modified tests mentioning hash/reload:
0
-- applying mutant: Secrets excluded from hashing
 pkg/config/dynamic/middlewares.go | 2 +-
 1 file changed, 1 insertion(+), 1 deletion(-)
-- authored tests under the mutant:
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.015s
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.247s
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.027s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.398s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.439s
-- eval fixture under the mutant:
--- FAIL: TestEval_CRD_SecretRotationAndHashStability (0.08s)
    eval_crd_behavior_test.go:255: 
        	Error Trace:	/home/ubuntu/wt/3e8db7d3/pkg/provider/kubernetes/crd/eval_crd_behavior_test.go:255
        	Error:      	Should not be: 0xfcfd8fd30e5a98c4
        	Test:       	TestEval_CRD_SecretRotationAndHashStability
        	Messages:   	secret rotation must change the provider change-detection value to trigger reload
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.090s
FAIL
```

The same fixture passes once the mutant is reverted (`--- PASS: TestEval_CRD_SecretRotationAndHashStability (0.08s)`), so the production code does change the hash on Secret rotation — only the regression assertion is missing from the authored tests.
