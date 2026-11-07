## Setup

Audited branch: `traefik-secret-references-in-middlewares-perfect` (HEAD `84614aa0ce1af141b9fbdeae2980b79e1f53ba73`), checked out as `verify/traefik-secret-references-in-middlewares-v-857c023f`. Eval base commit: `af6d927c51191ae1df104aaf907793386b816c28`.

Claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `evalrepo`) into separate worktrees:

```sh
git remote add evalrepo https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git fetch evalrepo evalon/traefik-se-65acc655 evalon/traefik-se-e78a464d evalon/traefik-se-7dca6067 evalon/traefik-se-cf8f03b1 evalon/traefik-se-0137858c
for b in 65acc655 e78a464d 7dca6067 cf8f03b1 0137858c; do git worktree add ~/wt/$b evalrepo/evalon/traefik-se-$b; done
git worktree add ~/wt/base af6d927c51191ae1df104aaf907793386b816c28
```

```console
65acc655 85893a79ecfc54e39e68b82ae0b481012fd46013
e78a464d 2db4116488e9df7fe6f14f50c4252bf5c77fe5b6
7dca6067 180c530c00f85c975d986fe96ed2f2c9c0e0c3af
cf8f03b1 1f082f9d9f2c98724bd50df4d73dcb9acea70960
0137858c 632d96ad493031e230bb3747c4a45a3798ed6752
```

All five target branches have the eval base as merge-base. Eval fixtures were extracted from `eval_tests.zip` to `~/evaltests/tests/` and copied to the paths listed in the audit prompt before running them. Every probe in `verify/repro/` carries `//go:build verify` and is run with `go test -tags verify` after being copied into the package directory named in its header comment. Go `1.26.3`.

## C1

Branch: `evalon/traefik-se-65acc655`. `pkg/server/middleware/secrets.go` on that branch:

```go
func resolvePluginConfig(config map[string]any, secrets dynamic.Secrets) (map[string]any, []string) {
	if len(secrets) == 0 {
		return config, nil          // <-- same map, not a copy
	}
	...
}

func (b *Builder) pluginSecretError(pluginType string, rawConfig map[string]any, middlewareName string, references []string) error {
	// The plugin may alter the configuration it receives, so it gets a copy of the published one.
	referenceConfig, _ := resolvePluginConfig(rawConfig, nil)   // <-- nil secrets => returns rawConfig itself
	if _, err := b.pluginBuilder.Build(pluginType, referenceConfig, middlewareName); err != nil {
		return err
	}
	...
}
```

and in `buildConstructor` (`middlewares.go:424-437`) `rawPluginConfig` comes from `findPluginConfig(config.Plugin)` where `config := b.configs[middlewareName]` (the published runtime config, not a copy); the retry is invoked whenever the first `Build` fails and `len(references) > 0`.

Probe `verify/repro/verify_c1_test.go`: a builder that fails on call 1 (resolved config) and on call 2 mutates `config["token"]` and `config["nested"]["token"]` before failing again. It records the identity of each map it receives.

```sh
cp verify/repro/verify_c1_test.go ~/wt/65acc655/pkg/server/middleware/
cd ~/wt/65acc655 && go test -tags verify -count=1 -run TestVerifyC1 -v ./pkg/server/middleware
```

```console
=== RUN   TestVerifyC1_PluginRetryAliasing
    verify_c1_test.go:57: returned error: plugin: rejected again
    verify_c1_test.go:58: stored info.Err: ["plugin: rejected again"]
    verify_c1_test.go:59: builder calls: 2
    verify_c1_test.go:64: call 1: config map identical to published map: false; token=s3cr3t
    verify_c1_test.go:67: call 1: nested map identical to published nested map: false
    verify_c1_test.go:64: call 2: config map identical to published map: true; token=MUTATED-BY-RETRY
    verify_c1_test.go:67: call 2: nested map identical to published nested map: true
    verify_c1_test.go:71: published token after build: MUTATED-BY-RETRY
    verify_c1_test.go:72: published nested token after build: NESTED-MUTATED-BY-RETRY
    verify_c1_test.go:80: CONFIRMED: retry builder mutation is visible in the published (API) configuration
--- PASS: TestVerifyC1_PluginRetryAliasing (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
```

- *Configuration aliasing*: CONFIRMED — call 2 receives the very map stored in `rtConf.Middlewares["plugin"].Plugin["test"]` (top-level and nested map pointers identical).
- *Error-path reachability*: CONFIRMED — a single failing `Build` on the resolved config produced a second `Build` call (`builder calls: 2`) on the published map, and the retry's mutation replaced the API-visible URN `urn:k8s:secret:plugin:token` with `MUTATED-BY-RETRY`.

The eval fixtures do not catch this (they never mutate on retry); for reference, with the fixtures copied into `~/wt/65acc655/pkg/provider/kubernetes/crd/`:

```sh
cd ~/wt/65acc655 && go test -count=1 -timeout=5m -run '^TestEval_ProviderToBuilder_PluginBuildErrorRedaction$' -v ./pkg/provider/kubernetes/crd | grep -E '^\s*--- (PASS|FAIL)|^(ok|FAIL)'
```

```console
--- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (0.78s)
        --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=false (0.17s)
        --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build/constant_error=true (0.17s)
    --- PASS: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor (0.34s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build_stage_wrapped_cause (0.02s)
            	Messages:   	stored diagnostic must contain URN reference
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.812s
```

(Those failures are about the retry's generic error text not naming URNs, a different defect; they are not evidence for C1.)

Impact reasoning: the retry runs on every plugin build failure for a Secret-backed plugin middleware. Any plugin builder that normalises/mutates its input map (e.g. defaulting keys, lower-casing, trimming) during a failing build rewrites the configuration served by `/api/rawdata` and the dashboard, and shared by any other router using the same middleware; the code comment claims a copy is passed, so the bug is silent.

## C2

Branch: audited branch (`traefik-secret-references-in-middlewares-perfect`). Code path: `pkg/provider/kubernetes/crd/kubernetes.go` publishes one identical URN per Secret line (`refs.AddUsers("/basicAuth/users", basicAuth.Secret, key, credentials)`), `pkg/middlewares/auth/auth.go` `getUsers` formats `fmt.Errorf("parsing user at index %d: %w", i-fileUsers, err)`, `basicUserParser`/`digestUserParser` return `errors.New("expected user:hash format")` / `errors.New("expected user:realm:hash format")`, and `BuildMiddlewareChain` (`pkg/server/middleware/middlewares.go:84-98`) explicitly skips `RedactURNRefs` for `BasicAuth`/`DigestAuth`, so nothing adds the URN.

Probe `verify/repro/verify_c2_test.go` (package `crd_test`, uses the eval fixture helpers `crd.LoadConfigurationWithSecretUpdate`, `evalBuildHandler`, `evalMiddlewareInfo`, `evalErrorTexts` from `export_eval_test.go` / `eval_provider_to_builder_runtime_test.go`, which must be present in the same directory). The `authsecret` Secret's `users` key is set to three lines, the second being `badsentinel`.

```sh
cp ~/evaltests/tests/export_eval_test.go ~/evaltests/tests/eval_provider_to_builder_runtime_test.go ~/evaltests/tests/eval_crd_behavior_test.go pkg/provider/kubernetes/crd/
cp verify/repro/verify_c2_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -run TestVerifyC2 -v ./pkg/provider/kubernetes/crd
```

```console
=== RUN   TestVerifyC2_AuthDiagnosticsOmitURN/default-basicauth
    verify_c2_test.go:33: published users: ["urn:k8s:secret:authsecret:users" "urn:k8s:secret:authsecret:users" "urn:k8s:secret:authsecret:users"]
    verify_c2_test.go:38: returned error: "parsing user at index 1: expected user:hash format"
    verify_c2_test.go:39: error chain texts: ["expected user:hash format" "parsing user at index 1: expected user:hash format"]
    verify_c2_test.go:40: stored info.Err: ["parsing user at index 1: expected user:hash format"]
    verify_c2_test.go:42: contains 'urn:k8s:secret:authsecret:users': false
    verify_c2_test.go:43: contains 'badsentinel': false
=== RUN   TestVerifyC2_AuthDiagnosticsOmitURN/default-digestauth
    verify_c2_test.go:36: published users: ["urn:k8s:secret:authsecret:users" "urn:k8s:secret:authsecret:users" "urn:k8s:secret:authsecret:users"]
    verify_c2_test.go:38: returned error: "parsing user at index 1: expected user:realm:hash format"
    verify_c2_test.go:39: error chain texts: ["expected user:realm:hash format" "parsing user at index 1: expected user:realm:hash format"]
    verify_c2_test.go:40: stored info.Err: ["parsing user at index 1: expected user:realm:hash format"]
    verify_c2_test.go:42: contains 'urn:k8s:secret:authsecret:users': false
    verify_c2_test.go:43: contains 'badsentinel': false
--- PASS: TestVerifyC2_AuthDiagnosticsOmitURN (0.12s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.151s
```

Both the returned error and the stored `info.Err` (what `/api/http/middlewares` and the log show) omit the configured URN for both auth types; they identify the entry by index only. The eval fixtures that cover this surface pass on the audited branch (they require a position, not the URN):

```sh
go test -count=1 -timeout=5m -run '^TestEval_(GetUsers_ErrorReportsPositionWithoutLeakingCredentials|ProviderToBuilder_BasicAuthSecretEntryPosition|ProviderToBuilder_DigestAuthSecretEntryPosition|BasicAuthParser_OmitCredentialsFromError|ProviderToBuilder_BasicAuthMalformedEntryRedaction|ProviderToBuilder_BasicAuthURNPrefixedSecretEntryRedaction|ProviderToBuilder_BasicAuthUnboundURNPrefixRedaction|DigestAuthParser_OmitCredentialsFromError|ProviderToBuilder_DigestAuthMalformedEntryRedaction|ProviderToBuilder_DigestAuthURNPrefixedSecretEntryRedaction|ProviderToBuilder_DigestAuthUnboundURNPrefixRedaction)$' ./pkg/middlewares/auth ./pkg/provider/kubernetes/crd
```

```console
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.036s
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.127s
```

Impact reasoning: a CRD Middleware has at most one `basicAuth.secret`/`digestAuth.secret`, so the operator can still find the Secret from the middleware; the message just does not name it. The task statement's "Errors should show the reference instead of the value" is not met for auth errors, but its "auth errors should still say which user entry is invalid" is. Low severity.

## C3

Branch: audited branch. Same build, same formatting path and same output as C2 (repeated here so this section stands alone):

```sh
cp verify/repro/verify_c2_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -run TestVerifyC2 -v ./pkg/provider/kubernetes/crd
```

```console
    verify_c2_test.go:38: returned error: "parsing user at index 1: expected user:hash format"
    verify_c2_test.go:40: stored info.Err: ["parsing user at index 1: expected user:hash format"]
    verify_c2_test.go:42: contains 'urn:k8s:secret:authsecret:users': false
    verify_c2_test.go:38: returned error: "parsing user at index 1: expected user:realm:hash format"
    verify_c2_test.go:40: stored info.Err: ["parsing user at index 1: expected user:realm:hash format"]
    verify_c2_test.go:42: contains 'urn:k8s:secret:authsecret:users': false
```

Production formatting path: `auth.getUsers` → `fmt.Errorf("parsing user at index %d: %w", ...)` → returned by the `alice.Constructor` in `buildConstructor` → `BuildMiddlewareChain` stores it verbatim via `AddError` because `m.BasicAuth == nil && m.DigestAuth == nil` is false, so `RedactURNRefs` (the only code that writes URNs into an error) is bypassed. Naming drift: the fixture helper is `evalAssertAuthSecretEntryPosition` in `eval_provider_to_builder_runtime_test.go` (not in `eval_auth_sanitation_test.go`); it asserts position only and passes (see the C2 fixture run: `ok ... pkg/provider/kubernetes/crd 0.127s`). Both auth types report a position without the bound URN: CONFIRMED, same low severity as C2.

## C4

Branch: `evalon/traefik-se-e78a464d`. `pkg/provider/kubernetes/ingress-nginx/build.go` `basicAuthUsers` (auth-map branch) stores `secrets[ref] = user + ":" + string(secret.Data[user])` for each key, and `pkg/config/dynamic/secrets.go` `resolveUsers` splits **every** referenced value on `"\n"`, appending one user entry per non-blank line.

Probe `verify/repro/verify_c4_test.go` (package `ingressnginx_test`, uses `ingressnginx.LoadIngressesForEvalTest` from the fixture `export_eval_ingress_nginx_test.go` and `newEvalIngressMiddlewareBuilder` from `eval_ingress_nginx_test.go`). Ingress with `auth-type: basic`, `auth-secret-type: auth-map`, `auth-secret: default/auth-map-secret`; the Secret has a single key `alice` whose value is `<bcrypt(alice-pass)>\nbob:<bcrypt(bob-pass)>`.

```sh
cd ~/wt/e78a464d
cp ~/evaltests/tests/eval_ingress_nginx_test.go ~/evaltests/tests/export_eval_ingress_nginx_test.go pkg/provider/kubernetes/ingress-nginx/
cp ~/repos/traefik/verify/repro/verify_c4_test.go pkg/provider/kubernetes/ingress-nginx/
go test -tags verify -count=1 -run TestVerifyC4 -v ./pkg/provider/kubernetes/ingress-nginx
```

```console
=== RUN   TestVerifyC4_AuthMapNewlineCreatesExtraUser
    verify_c4_test.go:62: middleware "default-ingress-auth-map-rule-0-path-0-tls-basic-auth" published users: ["urn:k8s:secret:auth-map-secret:alice"]
    verify_c4_test.go:62: middleware "default-ingress-auth-map-rule-0-path-0-basic-auth" published users: ["urn:k8s:secret:auth-map-secret:alice"]
    verify_c4_test.go:72: build error: <nil>
    verify_c4_test.go:73: stored info.Err: []
    verify_c4_test.go:86: alice/alice-pass -> 200
    verify_c4_test.go:87: bob/bob-pass     -> 200 (bob is NOT a key of the Secret)
    verify_c4_test.go:88: bob/wrong        -> 401
    verify_c4_test.go:90: CONFIRMED: the embedded line authenticates as an independent user
--- PASS: TestVerifyC4_AuthMapNewlineCreatesExtraUser (0.06s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.083s
```

Controls — the same probe on the eval base commit (upstream behaviour; `newEvalIngressMiddlewareBuilder` replaced by `middleware.NewBuilder(rtConf.Middlewares, nil, nil)`) rejects the value, and on the audited branch it is rejected too:

```console
# ~/wt/base
    verify_c4_test.go:72: build error: error parsing BasicUser: alice:$2a$04$U0nJ...\nbob:$2a$04$7BYa...
    verify_c4_test.go:75: middleware rejected the value
# audited branch
    verify_c4_test.go:72: build error: parsing user at index 0: expected user:hash format
    verify_c4_test.go:75: middleware rejected the value
```

Impact reasoning: on e78a464d a value that upstream (and ingress-nginx) treats as one broken password hash silently becomes two valid credentials; the published configuration shows only `urn:k8s:secret:auth-map-secret:alice`, so the extra user `bob` is invisible in the API/dashboard. Requires write access to the Secret (which already allows adding keys), so the security impact is limited to hiding a user from the configuration view; the correctness divergence from upstream is real and untested.

## C5

Branch: `evalon/traefik-se-7dca6067`. `pkg/server/middleware/middlewares.go` `redactSecretValues`: when `config.ContainsSecretValue(msg)` (substring match of any Secret value in the error text) it rebuilds with references; if the reference-error with references substituted does not equal the original message, it returns `"error hidden because it contains values of the secrets <refs>"`, dropping the rest of the diagnostic.

Probe `verify/repro/verify_c5_test.go` (package `middleware`): BasicAuth/DigestAuth with `Users: {urn}` and `Secrets: {urn: <value>}`, for value `user`, `alice:<valid>\nuser`, and a control `zzz-malformed`.

```sh
cp verify/repro/verify_c5_test.go ~/wt/7dca6067/pkg/server/middleware/
cd ~/wt/7dca6067 && go test -tags verify -count=1 -run TestVerifyC5 -v ./pkg/server/middleware
```

```console
=== RUN   TestVerifyC5_SecretValueUserLosesPosition/basic/value=user
    verify_c5_test.go:41: returned error: error hidden because it contains values of the secrets urn:k8s:secret:authsecret:users
    verify_c5_test.go:42: stored info.Err: ["error hidden because it contains values of the secrets urn:k8s:secret:authsecret:users"]
=== RUN   TestVerifyC5_SecretValueUserLosesPosition/basic/value=valid+user
    verify_c5_test.go:41: returned error: invalid user entry 0, line 1: error parsing BasicUser: expected format user:hash, got 1 fields
=== RUN   TestVerifyC5_SecretValueUserLosesPosition/basic/control_value=zzz-malformed
    verify_c5_test.go:41: returned error: invalid user entry 0: error parsing BasicUser: expected format user:hash, got 1 fields
=== RUN   TestVerifyC5_SecretValueUserLosesPosition/digest/value=user
    verify_c5_test.go:41: returned error: error hidden because it contains values of the secrets urn:k8s:secret:authsecret:users
    verify_c5_test.go:42: stored info.Err: ["error hidden because it contains values of the secrets urn:k8s:secret:authsecret:users"]
=== RUN   TestVerifyC5_SecretValueUserLosesPosition/digest/value=valid+user
    verify_c5_test.go:41: returned error: invalid user entry 0, line 1: error parsing DigestUser: expected format user:realm:hash, got 1 fields
=== RUN   TestVerifyC5_SecretValueUserLosesPosition/digest/control_value=zzz-malformed
    verify_c5_test.go:41: returned error: invalid user entry 0: error parsing DigestUser: expected format user:realm:hash, got 1 fields
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.024s
```

With value `user`, both BasicAuth and DigestAuth lose the `invalid user entry 0` position (the control value keeps it): the word `user` in `invalid user entry` / `expected format user:hash` matches the Secret value, the reference rebuild yields `..., got 5 fields` (the URN has 5 colon-separated parts) which does not round-trip, so the whole message is replaced. Note the `valid+user` case keeps its position only because the message there contains `line 1` and the rebuild on the single-line URN also differs — it is the `ContainsSecretValue` → fallback path that fires, and it fires whenever the Secret value is a substring of the diagnostic (`user`, `entry`, `hash`, `invalid`, `0`, ...). Both auth types affected: CONFIRMED. Severity low: the Secret must contain a malformed entry whose entire text is a word of the diagnostic; the diagnostic still names the Secret URN.

## C6

Branch: `evalon/traefik-se-65acc655`; same code path and same run as C1 (repeated so this section stands alone).

```sh
cp verify/repro/verify_c1_test.go ~/wt/65acc655/pkg/server/middleware/
cd ~/wt/65acc655 && go test -tags verify -count=1 -run TestVerifyC1 -v ./pkg/server/middleware
```

```console
    verify_c1_test.go:59: builder calls: 2
    verify_c1_test.go:64: call 1: config map identical to published map: false; token=s3cr3t
    verify_c1_test.go:64: call 2: config map identical to published map: true; token=MUTATED-BY-RETRY
    verify_c1_test.go:67: call 2: nested map identical to published nested map: true
    verify_c1_test.go:71: published token after build: MUTATED-BY-RETRY
    verify_c1_test.go:72: published nested token after build: NESTED-MUTATED-BY-RETRY
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
```

- *Retry input isolation*: CONFIRMED — `pluginSecretError` calls `resolvePluginConfig(rawConfig, nil)`, which returns `rawConfig` unchanged when `secrets` is empty, so the retry receives the published `PluginConf` map (and its nested maps) by identity.
- *Retry reachability*: CONFIRMED — one failing `Build` on the resolved config triggers a second `Build` through `pluginSecretError` (`builder calls: 2`), and its mutation lands in `rtConf.Middlewares["plugin"].Plugin["test"]`.

Impact reasoning as in C1.

## C7

Branch: `evalon/traefik-se-cf8f03b1`. Naming drift: this branch has no `urnrefs_test.go`, no `TestMiddleware_RedactURNRefs` and no `TestBuilder_BuildMiddlewareChain_pluginErrorURNRefs`; the committed redaction tests are `pkg/config/dynamic/secrets_test.go` (`TestSecrets_Reveals`, `TestSecrets_Resolve`, `TestMiddleware_WithSecretValues*`, `TestSecrets_notExposed`) and `pkg/server/middleware/secrets_test.go` (`TestBuilder_secretReferences`, `TestBuilder_authErrorHidesSecretValues`, `TestBuilder_pluginErrorHidesSecretValues`, `TestBuilder_pluginSecretValues`). Production sanitiser: `pkg/server/middleware/secrets.go` `hideSecretValues`.

Committed inputs/assertions (verbatim):

```go
// pkg/config/dynamic/secrets_test.go
func TestSecrets_Reveals(t *testing.T) {
	secrets := Secrets{"urn:k8s:secret:name:key": "value", "urn:k8s:secret:name:empty": ""}
	assert.True(t, secrets.Reveals(`invalid "value"`))
	assert.False(t, secrets.Reveals(`invalid "urn:k8s:secret:name:key"`))
	assert.False(t, Secrets(nil).Reveals("value"))
}
// pkg/server/middleware/secrets_test.go — Secret values used by builder tests:
//   "user:$apr1$...\ntest:$apr1$..."  (TestBuilder_secretReferences)
//   "user:hash\nmalformed-secret-entry" (TestBuilder_authErrorHidesSecretValues)
//   "s3cr3t-t0ken"                     (TestBuilder_pluginErrorHidesSecretValues)
//   "token-value"                      (TestBuilder_pluginSecretValues)
	require.Len(t, rtConf.Middlewares["auth"].Err, 1)
	assert.NotContains(t, rtConf.Middlewares["auth"].Err[0], "malformed-secret-entry")
	...
	require.EqualError(t, err, test.expectedError)
	assert.Equal(t, []string{test.expectedError}, rtConf.Middlewares["plugin"].Err)
```

```sh
cd ~/wt/cf8f03b1 && grep -rn 'Unwrap\|errors.As\|errors.Is\|\\\\' pkg/server/middleware/secrets_test.go pkg/config/dynamic/secrets_test.go pkg/server/middleware/middlewares_test.go
```

```console
(no output)
```

Demonstration by mutation — `verify/repro/verify_c7_mutations.sh` run from `~/wt/cf8f03b1` (each mutation is reverted before the next):

```console
### baseline
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.024s
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.012s
### mutation A: hidden error keeps the credential-bearing original as a wrapped cause (same message)
+type leakyWrap struct { msg string; cause error }
+func (e *leakyWrap) Error() string { return e.msg }
+func (e *leakyWrap) Unwrap() error { return e.cause }
-			return errors.New(refErr.Error())
+			return &leakyWrap{msg: refErr.Error(), cause: err}
-	return errSecretHidden
+	return &leakyWrap{msg: errSecretHidden.Error(), cause: err}
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.014s
### mutation B: Secrets.Reveals ignores values containing quotes or backslashes
-		if value != "" && strings.Contains(text, value) {
+		if value != "" && !strings.ContainsAny(value, "\"\\") && strings.Contains(text, value) {
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.024s
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.012s
### mutation C: hideSecretValues skips sanitation when any secret value is empty
+	for _, v := range secrets {
+		if v == "" {
+			return err
+		}
+	}
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.026s
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.014s
### mutation D: also store the raw plugin error in info.Err (control)
+			info.AddError(err, true)
--- FAIL: TestBuilder_pluginErrorHidesSecretValues (0.00s)
        secrets_test.go:151:
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.026s
```

With mutation A applied, a throwaway probe walking `errors.Unwrap` on the returned error shows the leak the committed tests do not see:

```console
Error(): plugin: failed to decode configuration: invalid token "urn:k8s:secret:plugin:token"
  cause: plugin: failed to decode configuration: invalid token "urn:k8s:secret:plugin:token"
  cause: failed to decode configuration: invalid token "urn:k8s:secret:plugin:token"
  cause: failed to decode configuration: invalid token "s3cr3t-t0ken"
```

- *Escaped payloads*: CONFIRMED — no committed Secret value contains a quote or backslash; mutation B (skip such values) stays green.
- *Empty payloads*: CONFIRMED — the only empty value is in `TestSecrets_Reveals`, a unit test of `Reveals` that never reaches `hideSecretValues`/the builder; mutation C (disable sanitation when an empty value exists) stays green.
- *Stored diagnostics*: REFUTED — `TestBuilder_authErrorHidesSecretValues` and `TestBuilder_pluginErrorHidesSecretValues` assert on `rtConf.Middlewares[...].Err` after `BuildMiddlewareChain`; mutation D fails `secrets_test.go:151`.
- *Wrapped causes*: CONFIRMED — no `Unwrap`/`errors.As`/`errors.Is` in any committed test; mutation A (credential-bearing cause behind the sanitised message) stays green.

Overall: 3 of 4 parts absent → CONFIRMED.

## C8

Branch: `evalon/traefik-se-0137858c`. `pkg/server/middleware/middlewares.go:409`:

```go
if config.Plugin != nil && !reflect.ValueOf(b.pluginBuilder).IsNil() { // Using "reflect" because "b.pluginBuilder" is an interface.
```

`reflect.ValueOf(<nil interface>)` is the zero `Value`, and `Value.IsNil` panics on it. Probe `verify/repro/verify_c8_test.go`: plugin middleware, `NewBuilder(configs, nil, nilBuilder)` with `var nilBuilder PluginsBuilder`, build inside `recover()`.

```sh
cp verify/repro/verify_c8_test.go ~/wt/0137858c/pkg/server/middleware/
cd ~/wt/0137858c && go test -tags verify -count=1 -run TestVerifyC8 -v ./pkg/server/middleware
```

```console
=== RUN   TestVerifyC8_NilPluginBuilderPanics
    verify_c8_test.go:29: panic: reflect: call of reflect.Value.IsNil on zero Value
    verify_c8_test.go:30: error: <nil>
    verify_c8_test.go:31: stored info.Err: []
    verify_c8_test.go:35: CONFIRMED: construction panicked
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
```

Same probe on the eval base commit (`~/wt/base`) — the line is byte-identical at `middlewares.go:408`, so the behaviour is pre-existing:

```console
    verify_c8_test.go:29: panic: reflect: call of reflect.Value.IsNil on zero Value
```

Same probe on the audited branch, which added a `b.pluginBuilder != nil &&` guard:

```console
    verify_c8_test.go:29: panic: <nil>
    verify_c8_test.go:30: error: invalid middleware "plugin" configuration: invalid middleware type or middleware does not exist
    verify_c8_test.go:31: stored info.Err: ["invalid middleware \"plugin\" configuration: invalid middleware type or middleware does not exist"]
```

Impact reasoning: `cmd/traefik/traefik.go:255,317` passes a typed `*plugins.Builder` (nil pointer when plugins are disabled, never an untyped nil interface), so the production binary does not hit this; it affects embedders/tests calling `NewBuilder(configs, nil, nil)` with a plugin middleware, and it is unchanged from the base commit. Behaviour as claimed: CONFIRMED, lowest severity.
