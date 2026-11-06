# Evidence — traefik-secret-references-in-middlewares (run v-8bf69973)

Setup common to every section (eval fixtures copied byte-exact from `eval_tests.zip` into the checkout under test):

```sh
F=~/attachments/0b14b93b-9a63-46f8-b00b-2f806668a70d/eval_tests/tests   # extracted eval_tests.zip
cp $F/eval_crd_behavior_test.go $F/export_eval_test.go $F/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/crd/
cp $F/eval_ingress_nginx_test.go $F/export_eval_ingress_nginx_test.go pkg/provider/kubernetes/ingress-nginx/
cp $F/eval_auth_sanitation_test.go pkg/middlewares/auth/
cp $F/eval_logconfiguration_test.go pkg/server/
```

Audited branch: `origin/traefik-secret-references-in-middlewares-perfect` at `81e96a27f` (checked out as `verify/traefik-secret-references-in-middlewares-v-8bf69973` in `~/repos/traefik`).
Claim-target branches were fetched from `https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares` (remote `target`) into worktrees: `~/wt/462ea24b` (`4c1a9556a`), `~/wt/63b324ef` (`17e84ec4c`), `~/wt/fd1b4d78` (`baa09025e`), `~/wt/c10e9fb2` (`89efe63e2`). All four share the base `af6d927c`.

Go: `go1.26.3 linux/amd64`.

## C1

Claim: sanitizing Secret-backed auth constructor failures removes the failing-entry position from returned errors and stored diagnostics. Branch: audited branch (`81e96a27f`).

Code path (`pkg/server/middleware/middlewares.go`, `BuildMiddlewareChain`): on any constructor error, `err = b.configs[name].Middleware.RedactURNRefs(err)` when `URNRefs` is non-empty; `RedactURNRefs` (`pkg/config/dynamic/urnrefs.go`) discards `err` entirely and returns `fmt.Errorf("plugin configuration using %s is invalid: configuration decoding failed", urns)`.

Step 1 — position reported *before* redaction (inner error from `auth.NewBasic`):

```sh
cp verify/repro/c1c2_inner_error_test.go pkg/middlewares/auth/
go test -tags verify -count=1 -v -run '^TestVerify_C1C2_InnerError' ./pkg/middlewares/auth
```

```console
[inner badIdx=0] NewBasic error: parsing user at index 0: expected user:hash format
[inner badIdx=2] NewBasic error: parsing user at index 2: expected user:hash format
--- PASS: TestVerify_C1C2_InnerError (0.00s)
```

Step 2 — the same malformed entry supplied through the Kubernetes Secret `authsecret` (key `users`) at index 0 and index 2, built through the CRD provider and `middleware.Builder`:

```sh
cp verify/repro/c1c2_auth_position_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -v -run '^TestVerify_C1C2_' ./pkg/provider/kubernetes/crd
```

```console
[default-basicauth badIdx=0] dynamic users=[urn:k8s:secret:authsecret:users urn:k8s:secret:authsecret:users urn:k8s:secret:authsecret:users]
[default-basicauth badIdx=0] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-basicauth badIdx=0] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-basicauth badIdx=2] dynamic users=[urn:k8s:secret:authsecret:users urn:k8s:secret:authsecret:users urn:k8s:secret:authsecret:users]
[default-basicauth badIdx=2] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-basicauth badIdx=2] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-digestauth badIdx=0] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-digestauth badIdx=0] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-digestauth badIdx=2] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-digestauth badIdx=2] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
--- PASS: TestVerify_C1C2_SecretBackedAuthPosition (0.15s)
```

The returned error and the stored `MiddlewareInfo.Err` are byte-identical for index 0 and index 2, for both basicAuth and digestAuth: the position (`at index N`) that the constructor produced is gone, and the message wrongly describes a basicAuth/digestAuth middleware as a "plugin configuration".

Fixture context — the eval auth fixtures only exercise inline/file users (no `URNRefs`), so they stay green on this branch and do not measure the Secret-backed path:

```sh
go test -count=1 -v -run '^TestEval_(GetUsers_.*|BasicAuthParser_OmitCredentialsFromError|DigestAuthParser_OmitCredentialsFromError|ProviderToBuilder_(Basic|Digest)Auth(MalformedEntry|URNPrefixedSecretEntry)Redaction)$' ./pkg/middlewares/auth ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_BasicAuthParser_OmitCredentialsFromError (0.00s)
--- PASS: TestEval_GetUsers_DiagnosticsDistinguishSources (0.00s)
--- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError (0.00s)
--- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.025s
--- PASS: TestEval_ProviderToBuilder_BasicAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthURNPrefixedSecretEntryRedaction (0.05s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthURNPrefixedSecretEntryRedaction (0.07s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.103s
```

Verdict: CONFIRMED. Impact reasoning: a malformed line in an htpasswd Secret is an ordinary operator mistake; the task statement requires that "auth errors should still say which user entry is invalid", and on this branch the operator gets neither the line nor a correct middleware type in `/api/rawdata` or the logs. Workaround: none from the API; the operator has to inspect the Secret line by line.

## C2

Claim: final diagnostics for malformed Secret-backed auth entries omit the Secret reference, the failing-entry position, or both. Branch: audited branch (`81e96a27f`).

Same run as C1 (repeated here so this section stands alone):

```sh
cp verify/repro/c1c2_auth_position_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -v -run '^TestVerify_C1C2_' ./pkg/provider/kubernetes/crd
```

```console
[default-basicauth badIdx=0] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-basicauth badIdx=0] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-basicauth badIdx=2] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-basicauth badIdx=2] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-digestauth badIdx=0] returned err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
[default-digestauth badIdx=2] stored info.Err: plugin configuration using urn:k8s:secret:authsecret:users is invalid: configuration decoding failed
```

Every final returned and stored diagnostic contains the configured Secret reference (`urn:k8s:secret:authsecret:users`) but none contains the failing-entry position (index 0 vs index 2 produce identical text). The pre-redaction constructor error did carry it (`parsing user at index 2: expected user:hash format`, see C1 step 1).

Verdict: CONFIRMED (reference present, position absent).

## C3

Claim: constructing Redis-backed RateLimit middleware exposes a resolved Secret path in diagnostics when TLS setup hits a filesystem error. Branch: `evalon/traefik-se-462ea24b` (`4c1a9556a`, worktree `~/wt/462ea24b`).

Code path: `pkg/types/tls.go` `loadKeyPair` falls back to `tls.LoadX509KeyPair(cert, key)` when the content is not PEM and `os.Stat(cert)` succeeds, so a Secret value that is an existing directory yields `read <path>: is a directory`. On this branch `buildMiddlewareConstructor` calls `ratelimiter.New(ctx, next, *config.RateLimit, ...)` with no redaction wrapper (only basicAuth/digestAuth and plugin errors are wrapped), and `ratelimiter.New` → `newRedisLimiter` → `config.Redis.TLS.CreateTLSConfig(ctx)`.

```sh
cd ~/wt/462ea24b
cp ~/repos/traefik/verify/repro/c3_redis_tls_path_test.go ~/repos/traefik/verify/repro/c3_mkdir_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -v -run '^TestVerify_C3_' ./pkg/provider/kubernetes/crd
```

```console
[C3 part1] CreateTLSConfig error: read /tmp/TestVerify_C3_RedisTLSFilesystemErrorPath5258230/001/secret-resolved-redis-tls-path: is a directory
[C3 part1] contains resolved path: true
[C3 part2] default-ratelimit dynamic tls.cert="urn:k8s:secret:tlssecret:tls.crt" tls.key="urn:k8s:secret:tlssecret:tls.key"
[C3 part2] default-ratelimit returned err text: creating TLS config: read /tmp/TestVerify_C3_RedisTLSFilesystemErrorPath5258230/001/secret-resolved-redis-tls-path: is a directory
[C3 part2] default-ratelimit returned err text: creating redis limiter: creating TLS config: read /tmp/TestVerify_C3_RedisTLSFilesystemErrorPath5258230/001/secret-resolved-redis-tls-path: is a directory
[C3 part2] default-ratelimit returned err text: is a directory
[C3 part2] default-ratelimit returned err text: read /tmp/TestVerify_C3_RedisTLSFilesystemErrorPath5258230/001/secret-resolved-redis-tls-path: is a directory
[C3 part2] default-ratelimit stored info.Err: creating redis limiter: creating TLS config: read /tmp/TestVerify_C3_RedisTLSFilesystemErrorPath5258230/001/secret-resolved-redis-tls-path: is a directory
[C3 part2] default-ratelimit diagnostics contain resolved path: true
--- PASS: TestVerify_C3_RedisTLSFilesystemErrorPath (0.07s)
```

Part *TLS constructor error*: the local `ClientTLS.CreateTLSConfig` error contains the resolved Secret value (the path). CONFIRMED.
Part *Middleware diagnostic exposure*: the dynamic configuration holds the URNs, but the returned error and the stored `info.Err` of `default-ratelimit` contain the resolved value. CONFIRMED.

Corroboration from the eval fixture on the same branch — the analogous forwardAuth TLS path leaks too (same unredacted `CreateTLSConfig` error):

```sh
cd ~/wt/462ea24b && go test -count=1 -v -run '^TestEval_(ProviderToBuilder_ForwardAuthTLSErrorRedaction|CRD_RateLimitRedis_SecretRetentionAndSerialization)$' ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_CRD_RateLimitRedis_SecretRetentionAndSerialization (0.09s)
        	Error:      	"unable to create client TLS configuration: read /tmp/TestEval_ProviderToBuilder_ForwardAuthTLSErrorRedaction2443064905/001/qx8SentinelForwardAuthDir: is a directory" should not contain "qx8SentinelForwardAuthDir"
        	Messages:   	ForwardAuth constructor error must not leak secret path payload
--- FAIL: TestEval_ProviderToBuilder_ForwardAuthTLSErrorRedaction (0.09s)
```

Verdict: CONFIRMED (both parts). Impact reasoning: the trigger needs a `tls.crt`/`tls.key` Secret value that names an existing directory, which is unusual; but the underlying gap is that rateLimit (and forwardAuth) construction errors on this branch are never passed through any redaction, so any constructor error that echoes a resolved value reaches `/api/rawdata` and the logs.

## C4

Claim: plugin constructor-error sanitization lacks payload-independent secrecy and regression assertions. Branch: `evalon/traefik-se-63b324ef` (`17e84ec4c`, worktree `~/wt/63b324ef`).

Code path (`pkg/server/middleware/secrets.go`): `redactSecrets` returns the error **unchanged** unless `containsSecretValue(err.Error(), config.Secrets)` finds a raw (unescaped, non-empty) Secret value in the message. Plugin decoding errors quote values with `%q`, so a value containing `"` or `\` appears escaped and is not detected. No function named `redactPluginError` exists on this branch; `redactSecrets`/`redactPluginSecrets` are the nearest equivalents (naming drift noted). `pkg/config/dynamic/urnrefs_test.go` and `TestBuilder_BuildMiddlewareChain_pluginErrorURNRefs` do not exist; the committed plugin-error tests are `TestBuilder_secretReferences_plugin` in `pkg/server/middleware/secrets_test.go` and `pkg/config/dynamic/secrets_test.go`.

Part *Diagnostic sanitization* — builder that quotes the offending value with `%q` (plain) or hides it in a wrapped cause (opaque):

```sh
cd ~/wt/63b324ef
cp ~/repos/traefik/verify/repro/c4_plugin_error_payloads_test.go pkg/server/middleware/
go test -tags verify -count=1 -v -run '^TestVerify_C4_' ./pkg/server/middleware
```

```console
[C4 empty opaque=false] value=""
  returned err chain: ["plugin: decoding failed on \"\": invalid token syntax" "decoding failed on \"\": invalid token syntax"]
  stored info.Err: ["plugin: decoding failed on \"\": invalid token syntax"]
[C4 ordinary opaque=false] value="s3cr3t_ordinary_value"
  returned err chain: ["plugin: decoding failed on \"urn:k8s:secret:plugin:token\": invalid token syntax" "decoding failed on \"urn:k8s:secret:plugin:token\": invalid token syntax"]
  stored info.Err: ["plugin: decoding failed on \"urn:k8s:secret:plugin:token\": invalid token syntax"]
[C4 ordinary opaque=true] value="s3cr3t_ordinary_value"
  returned err chain: ["plugin: plugin build failed: internal error" "plugin build failed: internal error" "underlying failure with secret: s3cr3t_ordinary_value"]
  stored info.Err: ["plugin: plugin build failed: internal error"]
[C4 quote-bearing opaque=false] value="s3cr3t\"quoted"
  returned err chain: ["plugin: decoding failed on \"s3cr3t\\\"quoted\": invalid token syntax" "decoding failed on \"s3cr3t\\\"quoted\": invalid token syntax"]
  stored info.Err: ["plugin: decoding failed on \"s3cr3t\\\"quoted\": invalid token syntax"]
  escaped payload "\"s3cr3t\\\"quoted\"" in diagnostics: true
[C4 backslash-bearing opaque=false] value="s3cr3t\\back"
  returned err chain: ["plugin: decoding failed on \"s3cr3t\\\\back\": invalid token syntax" "decoding failed on \"s3cr3t\\\\back\": invalid token syntax"]
  stored info.Err: ["plugin: decoding failed on \"s3cr3t\\\\back\": invalid token syntax"]
  escaped payload "\"s3cr3t\\\\back\"" in diagnostics: true
[C4 word-collision opaque=false] value="invalid"
  returned err chain: ["plugin: decoding failed on \"urn:k8s:secret:plugin:token\": invalid token syntax" ...]
--- PASS: TestVerify_C4_PluginErrorPayloadClasses (0.00s)
```

Observed: the quote-bearing value `s3cr3t"quoted` is disclosed as `s3cr3t\"quoted` and the backslash value `s3cr3t\back` as `s3cr3t\\back` in both the returned error and `info.Err`; the empty value makes the otherwise constant text differ (`value: ""` instead of the redacted reference form); an error whose message is safe but whose `Unwrap` cause carries the value keeps that cause (`underlying failure with secret: s3cr3t_ordinary_value`). CONFIRMED.

The eval fixture on this branch measures the same thing:

```sh
cd ~/wt/63b324ef && go test -count=1 -v -run '^TestEval_ProviderToBuilder_PluginBuildErrorRedaction$' ./pkg/provider/kubernetes/crd
```

```console
        	Error:      	"plugin: 'token' expected type 'int', got unconvertible type 'string', value: \"qx7Lead\\\"qx7Mid\\\\qx7Tail\"" should not contain "qx7Lead"
        	Messages:   	diagnostic must not leak credential sentinel
        	Error:      	Not equal:
        	            	expected: "plugin: 'token' expected type 'int', got unconvertible type 'string', value: \"urn:k8s:secret:name:key\""
        	            	actual  : "plugin: 'token' expected type 'int', got unconvertible type 'string', value: \"\""
        	Messages:   	diagnostic must be value-independent across public word collisions ('string', 'token'): metadata labels must not be rewritten
--- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (0.25s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/build_stage_wrapped_cause (0.02s)
    --- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction/constructor_stage_wrapped_cause (0.03s)
```

Part *Payload regression coverage* — committed tests inspected for empty, quote-bearing and backslash-bearing Secret payloads:

```sh
cd ~/wt/63b324ef
grep -rn --include=*_test.go -E 'Secrets\{|Secrets:' pkg | grep -v eval_ | grep -v c4_plugin      # all Secret literals in committed tests
grep -rn '\\\\\\\\\|\\\\"\|: ""\|NotContains\|Unwrap\|errors.Is\|errors.As' pkg/server/middleware/secrets_test.go pkg/config/dynamic/secrets_test.go
go test -count=1 -v -run 'TestBuilder_secretReferences|TestMiddleware_|TestSecretReference' ./pkg/server/middleware ./pkg/config/dynamic
```

```console
pkg/server/middleware/secrets_test.go:133:			secrets:      dynamic.Secrets{tokenRef: "s3cr3t"},
pkg/server/middleware/secrets_test.go:139:			secrets: dynamic.Secrets{
pkg/server/middleware/secrets_test.go:149:			secrets:       dynamic.Secrets{tokenRef: "s3cr3t"},
pkg/server/middleware/secrets_test.go:156:			secrets:       dynamic.Secrets{tokenRef: "s3cr3t"},
pkg/config/dynamic/secrets_test.go:89:		Secrets:   Secrets{ref: "user:$apr1$secret-hash"},
pkg/config/dynamic/secrets_test.go:104:			Secrets:   Secrets{ref: value},
(second grep: no match in either file)
--- PASS: TestBuilder_secretReferences_plugin/resolves_the_values (0.00s)
--- PASS: TestBuilder_secretReferences_plugin/plugin_creation_error_quoting_a_value (0.00s)
--- PASS: TestBuilder_secretReferences_plugin/value_rejected_while_its_reference_is_accepted (0.00s)
--- PASS: TestBuilder_secretReferences_plugin/decoding_error_quoting_a_value_equal_to_a_word_of_the_message (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.027s
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.011s
```

`TestBuilder_secretReferences_plugin` (lines 118-190 of `secrets_test.go`) uses only the payloads `s3cr3t` and `string`; no committed test uses an empty, quote-bearing or backslash-bearing Secret value with a redaction assertion. CONFIRMED.

Part *Cause-removal regression coverage* — the same grep shows no `Unwrap`/`errors.Is`/`errors.As`/`NotContains` assertion in `pkg/server/middleware/secrets_test.go` or `pkg/config/dynamic/secrets_test.go`; the plugin cases assert only `require.EqualError(t, err, test.expectedError)` on the top-level message and `info.Err`, so a retained credential-bearing cause would not fail them. CONFIRMED.

Verdict: CONFIRMED (all three parts). Impact reasoning: a plugin token/password containing `"` or `\` is ordinary; when the plugin rejects its configuration the escaped value is written into `/api/rawdata`, the dashboard and the logs, exactly the leak the task was meant to close.

## C5

Claim: Redis credential fields lose their Secret references when a present key has an empty value. Branch: `evalon/traefik-se-fd1b4d78` (`baa09025e`, worktree `~/wt/fd1b4d78`).

Code path (`pkg/provider/kubernetes/crd/kubernetes.go` ~1031-1045, `createRateLimitMiddleware`): after `loadRedisCredentials`, the reference is substituted only `if rl.Redis.Username != ""` / `if rl.Redis.Password != ""`.

```sh
cd ~/wt/fd1b4d78
cp ~/repos/traefik/verify/repro/c5_redis_empty_secret_test.go pkg/provider/kubernetes/crd/
go test -tags verify -count=1 -v -run '^TestVerify_C5_' ./pkg/provider/kubernetes/crd
```

```console
[C5 empty username / empty password] default-ratelimit Username="" Password="" hash=16518223710295023432
[C5 nonempty username / nonempty password] default-ratelimit Username="urn:k8s:secret:redissecret:username" Password="urn:k8s:secret:redissecret:password" hash=6336009857509564883
[C5 empty username / empty password again] default-ratelimit Username="" Password="" hash=16518223710295023432
--- PASS: TestVerify_C5_RedisEmptyCredentialReference (0.06s)
```

With `username`/`password` keys present but empty, the public fields are `""`; after rotating to non-empty values they become the URNs; rotating back to empty removes the URNs again. Fixture context on the same branch (non-empty values only):

```sh
cd ~/wt/fd1b4d78 && go test -count=1 -v -run '^TestEval_CRD_(RateLimitRedis_SecretRetentionAndSerialization|SecretRotationAndHashStability)$' ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_CRD_RateLimitRedis_SecretRetentionAndSerialization (0.06s)
--- PASS: TestEval_CRD_SecretRotationAndHashStability (0.09s)
```

Verdict: CONFIRMED. Impact reasoning: a Redis deployment without ACL users commonly ships a Secret with an empty `username`; the public routing configuration then shows `username: ""` and flips to the URN once a value is set, so the API/dashboard view of the middleware changes shape across rotations instead of showing a stable reference. The password/username still reach Redis correctly (the value is substituted only when non-empty, and an empty value needs no substitution), so this is a consistency defect, not a connectivity one.

## C6

Claim: malformed inline BasicAuth/DigestAuth entries with an unbound URN prefix disclose credential-bearing entry text. Branch: `evalon/traefik-se-c10e9fb2` (`89efe63e2`, worktree `~/wt/c10e9fb2`).

Code path (`pkg/server/middleware/middlewares.go`): `withUserSource(err, config.BasicAuth.Users)` sets `UserError.Source = users[userErr.Index]` whenever `dynamic.IsSecretReference(users[userErr.Index])` (prefix test only); it never checks `config.Secrets` for a binding. `UserError.Error()` prints `Source` in parentheses.

```sh
cd ~/wt/c10e9fb2
cp ~/repos/traefik/verify/repro/c6_unbound_urn_inline_test.go pkg/server/middleware/
go test -tags verify -count=1 -v -run '^TestVerify_C6_' ./pkg/server/middleware
```

```console
[C6 basicAuth] UserError.Index=1 Source="urn:k8s:secret:authsecret:users:leaked_inline_credential_hash_q9"
[C6 basicAuth] returned err: invalid user entry 1 (urn:k8s:secret:authsecret:users:leaked_inline_credential_hash_q9): error parsing BasicUser: expected the name:hash format
[C6 basicAuth] stored info.Err: ["invalid user entry 1 (urn:k8s:secret:authsecret:users:leaked_inline_credential_hash_q9): error parsing BasicUser: expected the name:hash format"]
[C6 basicAuth] diagnostics contain sentinel: true
[C6 digestAuth] UserError.Index=0 Source="urn:k8s:secret:authsecret:users:leaked_inline_credential_hash_q9"
[C6 digestAuth] returned err: invalid user entry 0 (urn:k8s:secret:authsecret:users:leaked_inline_credential_hash_q9): error parsing DigestUser: expected the name:realm:hash format
[C6 digestAuth] stored info.Err: ["invalid user entry 0 (urn:k8s:secret:authsecret:users:leaked_inline_credential_hash_q9): error parsing DigestUser: expected the name:realm:hash format"]
[C6 digestAuth] diagnostics contain sentinel: true
--- PASS: TestVerify_C6_UnboundURNPrefixInlineEntries (0.00s)
```

Part *Unbound source annotation*: with `Secrets == nil`, the inline entry text (including the trailing `leaked_inline_credential_hash_q9` segment) is copied into `UserError.Source`. CONFIRMED.
Part *Middleware diagnostic exposure*: the returned error and the stored `info.Err` contain the full entry for both basicAuth and digestAuth. CONFIRMED.

The eval fixtures for Secret-*bound* URN-prefixed entries pass on this branch (the bound case substitutes the reference, so `Source` is the bare URN):

```sh
cd ~/wt/c10e9fb2 && go test -count=1 -v -run '^TestEval_(GetUsers_.*|ProviderToBuilder_(Basic|Digest)Auth(MalformedEntry|URNPrefixedSecretEntry)Redaction)$' ./pkg/middlewares/auth ./pkg/provider/kubernetes/crd
```

```console
--- PASS: TestEval_GetUsers_DiagnosticsDistinguishSources (0.00s)
--- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials (0.00s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthMalformedEntryRedaction (0.00s)
--- PASS: TestEval_ProviderToBuilder_BasicAuthURNPrefixedSecretEntryRedaction (0.07s)
--- PASS: TestEval_ProviderToBuilder_DigestAuthURNPrefixedSecretEntryRedaction (0.07s)
```

Verdict: CONFIRMED (both parts). Impact reasoning: the trigger needs an inline (file/label provider) user entry that starts with `urn:k8s:secret:` yet is not a provider-bound reference, e.g. an operator pasting a Kubernetes-style reference into a file-provider config together with a hash; uncommon, but when it happens the whole entry, hash included, is written to the API and logs, which the base parser change (`expected the name:hash format`) was specifically meant to prevent.
