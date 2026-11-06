## Setup

All commands were run on this machine (Go `go1.26.3 linux/amd64`). Claim branches live in the repository named in the claim URLs, added as a second remote and checked out as worktrees:

```sh
cd ~/repos/traefik
git remote add evalrepo https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
for b in dfb43954 0b0162c2 3790f1c2 78d39bd7 009b2924 0386aeca 33ad4030 a75ff6bf e33f7395 83e20fa3 866b6773 ecef3519 94d51722 d6c9b55f; do
  git fetch -q evalrepo evalon/traefik-se-$b:refs/remotes/evalrepo/evalon/traefik-se-$b
  git worktree add -q ~/wt/$b evalrepo/evalon/traefik-se-$b
done
git worktree add -q ~/wt/base af6d927c51191ae1df104aaf907793386b816c28   # unchanged baseline for C2
```

Every claim branch is a single commit on top of `af6d927c5`. Probe tests referenced below are kept under `verify/repro/` (each carries a `//go:build ignore` line and a `// Run:` header; the header's `sed` strips the build tag when copying the file into the target package).

## C1

Claim: Ingress NGINX auth-map conversion exposes Secret-derived password values in the public `BasicAuth.Users` entries (five branches).

Probe `verify/repro/c1_authmap_probe_test.go` loads an `auth-map` Secret with two distinctive hashes (`$apr1$DISTINCTHASHALICE$zzz`, `$apr1$DISTINCTHASHBOB$yyy`) through the provider (via the eval helper `LoadIngressesForEvalTest`) and prints the public entries and the JSON of the whole configuration. The eval fixture `TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation` was run alongside it.

```sh
for b in dfb43954 0b0162c2 3790f1c2 78d39bd7 009b2924; do
  cp eval_tests/tests/eval_ingress_nginx_test.go eval_tests/tests/export_eval_ingress_nginx_test.go verify/repro/c1_authmap_probe_test.go ~/wt/$b/pkg/provider/kubernetes/ingress-nginx/  # (strip the //go:build ignore line of the probe)
  (cd ~/wt/$b && go test -count=1 -timeout=5m -v -run '^(TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation|TestProbe_C1_AuthMapPublicEntries)$' ./pkg/provider/kubernetes/ingress-nginx)
done
```

Identical output on all five branches (dfb43954, 0b0162c2, 3790f1c2, 78d39bd7, 009b2924):

```console
    c1_probe_test.go:51: USER ENTRY: "alice:urn:k8s:secret:auth-map-secret:alice"
    c1_probe_test.go:51: USER ENTRY: "bob:urn:k8s:secret:auth-map-secret:bob"
    c1_probe_test.go:55: JSON contains DISTINCTHASH: false
    c1_probe_test.go:56: Users contain DISTINCTHASH: false
    c1_probe_test.go:58: MIDDLEWARE JSON: {"basicAuth":{"users":["alice:urn:k8s:secret:auth-map-secret:alice","bob:urn:k8s:secret:auth-map-secret:bob"]}}
--- PASS: TestProbe_C1_AuthMapPublicEntries
        	Messages:   	auth-map users must contain URN reference for :user1
        	Messages:   	rotated configuration must retain URN references, got "user1:urn:k8s:secret:auth-map-secret:user1"
--- FAIL: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation (0.03s)
```

Observation: on every branch the password value is replaced by `urn:k8s:secret:<secret>:<user>`; the hash is absent from the entries and from the serialized configuration. The eval fixture fails only because the entries carry a `user:` prefix before the URN (`strings.HasPrefix(u, "urn:k8s:secret:")`), i.e. public username text — which the claim itself says does not establish exposure. The `basicAuthUsers` code of each branch is the same shape: `users = append(users, user+":"+ref)` with `secrets[ref] = string(secret.Data[user])`.

Verdict: REFUTED on all five branches.

## C2

Claim: the delivered code changes make TLS connection teardown fail in the eval environment (branches 0386aeca and 33ad4030), compared with the unchanged baseline `af6d927c5`.

Neither branch touches TLS or connection code: `git diff --stat af6d927c5 HEAD -- pkg/server/` on 0386aeca lists only `pkg/server/middleware/middlewares.go` and `pkg/server/middleware/secrets_test.go`; on 33ad4030 the only non-middleware server change is `pkg/server/configurationwatcher.go` (nil-ing `middleware.Secrets` in `logConfiguration`).

First attempt (invalid, kept for transparency): the three suites were run **concurrently** in three worktrees and `TestHTTP30RTT` / `TestHTTP3ReadTimeout` failed with `listen tcp 127.0.0.1:8090: bind: address already in use` / `listen udp 127.0.0.1:8091` — these tests use fixed ports, so parallel suites on one machine collide. Baseline, which happened to start last, passed. This is an artifact of my parallel execution, not of the branches.

Sequential runs (one suite at a time), the eval's own command:

```sh
for b in 0386aeca 33ad4030 base; do
  (cd ~/wt/$b && go test -count=1 -timeout=5m -skip '^TestForwardAuthClientClosedRequest$' ./pkg/config/dynamic/... ./pkg/server/... ./pkg/middlewares/auth/... ./pkg/provider/kubernetes/crd/... ./pkg/provider/kubernetes/ingress-nginx/... ./pkg/provider/kubernetes/k8s/... ./pkg/redactor/... ./pkg/plugins/... > /tmp/c2_${b}_seq.log 2>&1; echo "exit=$?" >> /tmp/c2_${b}_seq.log)
done
grep -c '^ok' /tmp/c2_*_seq.log; grep -v '^ok' /tmp/c2_*_seq.log | grep -v 'no test files'
```

```console
/tmp/c2_0386aeca_seq.log:25   exit=0
/tmp/c2_33ad4030_seq.log:25   exit=0
/tmp/c2_base_seq.log:25       exit=0
```

25 `ok` package lines, no `FAIL`, exit 0 on both branches and on the baseline.

Repeated three more times each, still sequentially (`/tmp/c2_<branch>_seq{2,3,4}.log`):

```console
/tmp/c2_0386aeca_seq2.log ok=25 fail=0 exit=0
/tmp/c2_0386aeca_seq3.log ok=25 fail=0 exit=0
/tmp/c2_0386aeca_seq4.log ok=25 fail=0 exit=0
/tmp/c2_33ad4030_seq2.log ok=25 fail=0 exit=0
/tmp/c2_33ad4030_seq3.log ok=25 fail=0 exit=0
/tmp/c2_33ad4030_seq4.log ok=25 fail=0 exit=0
/tmp/c2_base_seq2.log ok=25 fail=0 exit=0
/tmp/c2_base_seq3.log ok=25 fail=0 exit=0
/tmp/c2_base_seq4.log ok=25 fail=0 exit=0
```

Scheduling stress on the TLS/connection-teardown tests (`TestShutdownHijacked/HTTP/TCP/UDPConn`, `TestHTTP30RTT`, `TestHTTP3ReadTimeout`, `TestMTLS`, `TestSpiffeMTLS`, `TestEntryPointTLSResolvedOptions`, `TestHTTP2RequestTLS`, …) with the race detector, three iterations, run one branch at a time:

```sh
for b in 0386aeca 33ad4030 base; do
  (cd ~/wt/$b && go test -race -count=3 -timeout=5m -run 'TLS|Shutdown|HTTP3|MTLS' ./pkg/server/ ./pkg/server/service/... > /tmp/c2_${b}_race.log 2>&1; echo "exit=$?" >> /tmp/c2_${b}_race.log)
done
```

```console
== /tmp/c2_0386aeca_race.log   ok  github.com/traefik/traefik/v3/pkg/server  33.306s   exit=0
== /tmp/c2_33ad4030_race.log   ok  github.com/traefik/traefik/v3/pkg/server  32.909s   exit=0
== /tmp/c2_base_race.log       ok  github.com/traefik/traefik/v3/pkg/server  32.700s   exit=0
```

No teardown failure on either branch in 4 full sequential suite runs plus 3 race-detector iterations of the TLS/shutdown tests; the only failure ever observed (fixed-port collision) also depends on nothing in the delivered diff and would hit the baseline equally when run concurrently.

Verdict: REFUTED on both branches.

## C3

Claim: the committed plugin Secret-error tests (branch a75ff6bf) do not assert `MiddlewareInfo.Err` through `BuildMiddlewareChain` (stored_diagnostic_coverage) and do not inspect unwrapped causes (cause_chain_coverage). The branch has `pkg/config/dynamic/secrets.go` (not `urnrefs.go`) and `pkg/server/middleware/secrets_test.go`.

Assertion inventory:

```sh
cd ~/wt/a75ff6bf
grep -n "MiddlewareInfo\|\.Err\b\|BuildMiddlewareChain\|Unwrap\|errors.As\|errors.Is\|EqualError\|ErrorContains" pkg/server/middleware/secrets_test.go pkg/config/dynamic/secrets_test.go pkg/provider/kubernetes/crd/secrets_test.go pkg/provider/kubernetes/ingress-nginx/secrets_test.go pkg/middlewares/auth/auth_test.go
```

```console
pkg/server/middleware/secrets_test.go:111:			require.EqualError(t, err, test.expectedErr)
pkg/middlewares/auth/auth_test.go:81:			require.EqualError(t, err, "error parsing BasicUser: "+test.expectedErr)
pkg/middlewares/auth/auth_test.go:86:			require.EqualError(t, err, "error parsing DigestUser: "+test.expectedErr)
```

`BuildMiddlewareChain` appears in test code only at `pkg/server/middleware/middlewares_test.go:22,33,274`, a file the branch does not modify (`git diff --stat af6d927c5 HEAD -- pkg/server/middleware/middlewares_test.go` is empty). The plugin Secret-error test `TestBuilder_buildConstructorPluginSecretErrors` calls `builder.buildConstructor` directly through `buildSecretMiddleware` and checks only `err.Error()` via `require.EqualError`; the `rtConf` it gets back is used only to compare `rtConf.Middlewares["secret"].Middleware` with the input.

```sh
go test -count=1 -v -run 'TestBuilder_buildConstructorPluginSecret' ./pkg/server/middleware
```

```console
--- PASS: TestBuilder_buildConstructorPluginSecretValues (0.00s)
--- PASS: TestBuilder_buildConstructorPluginSecretErrors (0.00s)
    --- PASS: TestBuilder_buildConstructorPluginSecretErrors/decoding_error_quoting_the_value (0.00s)
    --- PASS: TestBuilder_buildConstructorPluginSecretErrors/decoding_error_with_a_word_equal_to_the_value (0.00s)
    --- PASS: TestBuilder_buildConstructorPluginSecretErrors/constructor_error_quoting_the_value (0.00s)
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.045s
```

Mutation check (`verify/repro/c3_mutation_check.sh`): (1) `BuildMiddlewareChain` stores `fmt.Errorf("MUTATION stored diagnostic leaks value: invalid")` in `MiddlewareInfo.Err` instead of the returned error; (2) `secretReferencesError` returns an error whose `Unwrap()` yields `errors.New("MUTATION cause quotes value: invalid")` while `Error()` is unchanged (`invalid` is the Secret value used by the tests).

```console
 pkg/server/middleware/middlewares.go |  2 +-
 pkg/server/middleware/plugins.go     | 10 +++++++++-
ok  	github.com/traefik/traefik/v3/pkg/server/middleware	0.032s
ok  	github.com/traefik/traefik/v3/pkg/config/dynamic	0.014s
```

Both mutations leave every committed test green. Files restored with `git checkout` afterwards.

Verdict: CONFIRMED (both parts).

## C4

Claim: on branch e33f7395 `loadCASecretWithKey` re-implements the `tls.ca`-before-`ca.crt` selection that `getCABlocks` already implements, both retained.

Callers (production code):

```sh
cd ~/wt/e33f7395 && grep -n "getCABlocks(\|loadCASecret(\|loadCASecretWithKey(\|loadMiddlewareCASecret(" pkg/provider/kubernetes/crd/*.go | grep -v "func "
```

```console
pkg/provider/kubernetes/crd/kubernetes.go:425:			caSecret, err := loadCASecret(serversTransport.Namespace, secret, client)
pkg/provider/kubernetes/crd/kubernetes.go:444:				ca, err := loadCASecret(serversTransport.Namespace, *rootCA.Secret, client)
pkg/provider/kubernetes/crd/kubernetes.go:621:			caSecret, err := loadCASecret(serversTransportTCP.Namespace, secret, client)
pkg/provider/kubernetes/crd/kubernetes.go:640:				ca, err := loadCASecret(serversTransportTCP.Namespace, *rootCA.Secret, client)
pkg/provider/kubernetes/crd/kubernetes.go:992:			caSecret, err := loadMiddlewareCASecret(namespace, rateLimit.Redis.TLS.CASecret, client, secrets)
pkg/provider/kubernetes/crd/kubernetes.go:1132:			caSecret, err := loadMiddlewareCASecret(namespace, auth.TLS.CASecret, k8sClient, secrets)
pkg/provider/kubernetes/crd/kubernetes.go:1155:	_, value, err := loadCASecretWithKey(namespace, secretName, k8sClient)
pkg/provider/kubernetes/crd/kubernetes.go:1160:	key, value, err := loadCASecretWithKey(namespace, secretName, k8sClient)
pkg/provider/kubernetes/crd/kubernetes.go:1415:			cert, err := getCABlocks(secret, tlsOptionsCRD.Namespace, secretName)
```

`loadCASecretWithKey` (lines 1167-1196) has `for _, key := range []string{"tls.ca", "ca.crt"} { if value, ok := secret.Data[key]; ok { return key, string(value), nil } }` plus the single-key fallback; `getCABlocks` (lines 1666-1678) has the separate `tls.ca` then `ca.crt` lookups and is still called by `buildTLSOptions` (line 1415). Neither calls the other.

Probe `verify/repro/c4_ca_selection_probe_test.go` feeds the same Secrets to both:

```sh
go test -count=1 -v -run TestProbe_C4 ./pkg/provider/kubernetes/crd
```

```console
    c4_probe_test.go:34: secret=both getCABlocks -> "FROM-TLS-CA" err=<nil> | loadCASecretWithKey -> key="tls.ca" value="FROM-TLS-CA" err=<nil>
    c4_probe_test.go:34: secret=onlycrt getCABlocks -> "FROM-CA-CRT" err=<nil> | loadCASecretWithKey -> key="ca.crt" value="FROM-CA-CRT" err=<nil>
--- PASS: TestProbe_C4_CASelectionImplementedTwice (1.20s)
```

Two independent implementations of the same preference order, both live.

Verdict: CONFIRMED.

## C5

Claim: on branch 83e20fa3 `getUsers` drops the users-file name from file-entry diagnostics (file_source_identification) and numbers inline entries in the combined file+inline sequence (inline_source_position).

Code (`pkg/middlewares/auth/auth.go:17-33`): `loadUsers` returns `append(fileLines, appendUsers...)` and `getUsers` loops `for i, user := range users { ... fmt.Errorf("invalid user entry %d: %w", i+1, err) }`.

Probe `verify/repro/c5_getusers_diag_probe_test.go`:

```sh
go test -count=1 -v -run TestProbe_C5 ./pkg/middlewares/auth
```

```console
    c5_probe_test.go:17: FILE  malformed entry -> err=invalid user entry 2: error parsing BasicUser: expected username:password
    c5_probe_test.go:28: INLINE with 0 preceding file entries -> err=invalid user entry 1: error parsing BasicUser: expected username:password
    c5_probe_test.go:28: INLINE with 2 preceding file entries -> err=invalid user entry 3: error parsing BasicUser: expected username:password
    c5_probe_test.go:28: INLINE with 4 preceding file entries -> err=invalid user entry 5: error parsing BasicUser: expected username:password
--- PASS: TestProbe_C5_GetUsersDiagnostics (0.00s)
```

The file diagnostic names no file (`named-users.htpasswd` is absent); the single malformed inline entry is reported as entry 1, 3 or 5 depending only on how many valid file entries precede it.

Verdict: CONFIRMED (both parts).

## C6

Claim: on branch 0b0162c2 the CRD BasicAuth middleware built from a `kubernetes.io/basic-auth` Secret serializes the Secret's username in `BasicAuth.Users`. (`URNRefs.AddUsers` does not exist on this branch; the equivalent is `loadBasicAuthCredentials`, which returns `string(username) + ":" + passwordRef`.)

Probe `verify/repro/c6_basicauth_username_probe_test.go` creates a typed Secret with `username=DISTINCT_USERNAME_zq`, `password=DISTINCT_PASSWORD_zq` and calls `createBasicAuthMiddleware`:

```sh
go test -count=1 -v -run TestProbe_C6 ./pkg/provider/kubernetes/crd
```

```console
    c6_probe_test.go:34: BasicAuth.Users = ["DISTINCT_USERNAME_zq:urn:k8s:secret:typed-ba:password"]
    c6_probe_test.go:35: public middleware JSON = {"basicAuth":{"users":["DISTINCT_USERNAME_zq:urn:k8s:secret:typed-ba:password"]}}
    c6_probe_test.go:36: secret store keys = [urn:k8s:secret:typed-ba:password]
--- PASS: TestProbe_C6_BasicAuthTypedSecretUsername (0.86s)
```

The Secret-derived username is in the public configuration (and therefore in `/api/rawdata`, dashboard, support dump); only the password is referenced. The task statement requires a reference "in place of each value read from a Secret".

Verdict: CONFIRMED.

## C7

Claim: on branch 866b6773 the CRD auth middlewares generate references whose key does not exist in the source Secret (e.g. `users.0`). Code: `secretUsers` (kubernetes.go:1740) builds `fmt.Sprintf("urn:k8s:secret:%s:users.%d", name, i)` for every parsed entry, for Opaque and typed Secrets alike.

Probe `verify/repro/c7_synthesized_keys_probe_test.go` builds BasicAuth from an Opaque Secret (key `users`, 2 lines), BasicAuth from a `kubernetes.io/basic-auth` Secret (keys `username`, `password`) and DigestAuth from an Opaque Secret, then compares each reference key with the Secret's data keys:

```sh
go test -count=1 -v -run TestProbe_C7 ./pkg/provider/kubernetes/crd
```

```console
    c7_probe_test.go:43: secret=opaque-auth dataKeys=[users] ref="urn:k8s:secret:opaque-auth:users.0" refKey="users.0" existsInSecret=false
    c7_probe_test.go:43: secret=opaque-auth dataKeys=[users] ref="urn:k8s:secret:opaque-auth:users.1" refKey="users.1" existsInSecret=false
    c7_probe_test.go:43: secret=typed-ba dataKeys=[password username] ref="urn:k8s:secret:typed-ba:users.0" refKey="users.0" existsInSecret=false
    c7_probe_test.go:43: secret=digest-auth dataKeys=[users] ref="urn:k8s:secret:digest-auth:users.0" refKey="users.0" existsInSecret=false
    c7_probe_test.go:43: secret=digest-auth dataKeys=[users] ref="urn:k8s:secret:digest-auth:users.1" refKey="users.1" existsInSecret=false
--- PASS: TestProbe_C7_ReferenceKeysVsSecretKeys (1.12s)
```

No generated reference names a real Secret key; the typed Secret's `password` key is never referenced.

Verdict: CONFIRMED.

## C8

Claim: on branch ecef3519 `buildPlugin` returns an error that omits every configured Secret URN when the plugin rejects resolved values but accepts the reference-only configuration. (`RedactURNRefs`/`urnrefs.go` do not exist on this branch; the relevant code is `buildPlugin` in `pkg/server/middleware/plugins.go`, which ends with `errors.New("invalid configuration: the plugin rejects the resolved Secret values")`.)

Probe `verify/repro/c8_buildplugin_error_probe_test.go`: plugin builder rejects `token == "RESOLVED_SECRET_VALUE"` and accepts the URN; middleware `Plugin: {"test": {"token": "urn:k8s:secret:plugin:token"}}`, `Secrets: {"urn:k8s:secret:plugin:token": "RESOLVED_SECRET_VALUE"}`.

```sh
go test -count=1 -v -run TestProbe_C8 ./pkg/server/middleware
```

```console
    c8_probe_test.go:40: buildPlugin error: invalid configuration: the plugin rejects the resolved Secret values
    c8_probe_test.go:44: BuildMiddlewareChain error: plugin: invalid configuration: the plugin rejects the resolved Secret values
    c8_probe_test.go:45: MiddlewareInfo.Err: ["plugin: invalid configuration: the plugin rejects the resolved Secret values"]
--- PASS: TestProbe_C8_BuildPluginErrorOmitsURNs (0.00s)
```

`urn:k8s:secret:plugin:token` is absent from the returned error and from the stored diagnostic. Cross-check with the eval fixture (`eval_provider_to_builder_runtime_test.go` + `export_eval_test.go` copied into `pkg/provider/kubernetes/crd/`):

```sh
go test -count=1 -v -run '^TestEval_ProviderToBuilder_PluginBuildErrorRedaction$' ./pkg/provider/kubernetes/crd
```

```console
        	Error:      	"constructor failed on \"this_is_the_secret\": permission denied" should not contain "this_is_the_secret"
        	Error:      	"constructor failed on \"this_is_the_secret\": permission denied" does not contain "urn:k8s:secret:name:key"
--- FAIL: TestEval_ProviderToBuilder_PluginBuildErrorRedaction (0.19s)
```

(The fixture additionally shows that the *constructor*-phase error on this branch still quotes the Secret value — a separate, larger defect than the one C8 names; recorded here as an aside.)

Verdict: CONFIRMED.

## C9

Claim: on branch 94d51722 the final sanitization (`Secrets.RedactError` in `BuildMiddlewareChain`/`buildConstructor`) removes the malformed-entry position when a Secret value is a word of the fixed parser message (`format`, `invalid`).

Probe `verify/repro/c9_sanitization_position_probe_test.go` builds BasicAuth/DigestAuth with `Users: {"urn:k8s:secret:auth:users"}` and `Secrets: {urn: <value>}`, prints the parser diagnostic (constructor called directly with the resolved entry), the error returned by `BuildMiddlewareChain(...).Then(...)` and `MiddlewareInfo.Err`:

```sh
go test -count=1 -v -run TestProbe_C9 ./pkg/server/middleware
```

```console
    [basic/format] secret value="format"
    [basic/format]   parser diagnostic : invalid user entry at index 0: error parsing BasicUser: expected name:hashed-password format
    [basic/format]   returned final    : error message redacted as it may contain the value of one of the Secret references urn:k8s:secret:auth:users
    [basic/format]   stored Info.Err   : ["error message redacted as it may contain the value of one of the Secret references urn:k8s:secret:auth:users"]
    [basic/invalid] secret value="invalid"
    [basic/invalid]   parser diagnostic : invalid user entry at index 0: error parsing BasicUser: expected name:hashed-password format
    [basic/invalid]   returned final    : error message redacted as it may contain the value of one of the Secret references urn:k8s:secret:auth:users
    [basic/unrelated-word] secret value="zzqq_not_in_message"
    [basic/unrelated-word]   returned final    : invalid user entry at index 0: error parsing BasicUser: expected name:hashed-password format
    [digest/format]   parser diagnostic : invalid user entry at index 0: error parsing DigestUser: expected name:realm:encoded-password format
    [digest/format]   returned final    : error message redacted as it may contain the value of one of the Secret references urn:k8s:secret:auth:users
    [digest/invalid]  returned final    : error message redacted as it may contain the value of one of the Secret references urn:k8s:secret:auth:users
```

`index 0` is present before sanitization and absent in the returned and stored diagnostics whenever the Secret value equals a word of the parser message; with an unrelated value the position survives. `ReferencesIn` does a plain `strings.Contains(msg, value)`, so any Secret value that is a substring of the fixed wording (`format`, `invalid`, `user`, `entry`, `index`, `name`, …) triggers it.

Verdict: CONFIRMED.

## C10

Claim: on branch dfb43954 a whitespace-padded single-user Ingress NGINX auth-file without trailing newline fails authentication because neither the provider (provider_normalization) nor the runtime (runtime_normalization) trims it.

Code: `basicAuthUsers` (build.go:1256-1264) stores `string(authFileContent)` verbatim; `getUsers` (auth.go:39-46) only calls `splitUserLines` (which trims) when the entry contains `"\n"`, otherwise passes it to the parser untouched.

Provider probe `verify/repro/c10_provider_normalization_probe_test.go`:

```sh
go test -count=1 -v -run TestProbe_C10 ./pkg/provider/kubernetes/ingress-nginx
```

```console
    auth file "  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  " -> users=["urn:k8s:secret:ba:auth"] stored="  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  "
    auth file "  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  \n" -> users=["urn:k8s:secret:ba:auth"] stored="  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  \n"
```

Runtime probe `verify/repro/c10_runtime_normalization_probe_test.go` (BasicAuth built through `BuildMiddlewareChain` with the stored value, request with `test:test`):

```sh
go test -count=1 -v -run TestProbe_C10 ./pkg/server/middleware
```

```console
    [padded, no newline] value="  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  " -> auth test/test status=401
    [padded, newline-terminated] value="  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  \n" -> auth test/test status=200
    [unpadded, no newline] value="test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/" -> auth test/test status=200
```

The padded entry is stored with its padding and, lacking a newline, reaches the parser untrimmed: username becomes `"  test"` and the hash `"...wj/  "`, so valid credentials get 401. Adding a trailing newline makes the same bytes authenticate.

Verdict: CONFIRMED (both parts).

## C11

Claim: on branch d6c9b55f `ParseSecretReference` (pkg/config/dynamic/secrets.go) has no production callers (production_use) and CRD `getSecretValue` parses references independently (parsing_duplication).

References:

```sh
cd ~/wt/d6c9b55f && grep -rn "ParseSecretReference" --include=*.go . | grep -v _test.go; echo ---; grep -rn "ParseSecretReference" --include=*_test.go .
```

```console
./pkg/config/dynamic/secrets.go:17:// ParseSecretReference returns the Secret name and key of a reference built with SecretReference.
./pkg/config/dynamic/secrets.go:18:func ParseSecretReference(ref string) (string, string, error) {
---
./pkg/config/dynamic/secrets_test.go:12:func TestParseSecretReference(t *testing.T) {
./pkg/config/dynamic/secrets_test.go:13:	name, key, err := ParseSecretReference(SecretReference("name", "key"))
./pkg/config/dynamic/secrets_test.go:19:		_, _, err = ParseSecretReference(ref)
```

Build check (`verify/repro/c11_unexport_build_check.sh`): rename the function and build all production code.

```sh
sed -i 's/^func ParseSecretReference(/func parseSecretReferenceRenamed(/' pkg/config/dynamic/secrets.go
go build ./... && echo "BUILD OK with ParseSecretReference unexported"
go vet ./pkg/config/dynamic/ 2>&1 | head -3
git checkout pkg/config/dynamic/secrets.go
```

```console
BUILD OK with ParseSecretReference unexported
# [github.com/traefik/traefik/v3/pkg/config/dynamic]
vet: pkg/config/dynamic/secrets_test.go:13:20: undefined: ParseSecretReference
```

Only the test file breaks. Parsing divergence probe `verify/repro/c11_parsing_divergence_probe_test.go` feeds the same strings to both functions (`getSecretValue` with a fake client holding Secret `s` with key `k`):

```sh
go test -count=1 -v -run TestProbe_C11 ./pkg/provider/kubernetes/crd
```

```console
    ref="urn:k8s:secret:s:k"         ParseSecretReference -> name="s" key="k" err=<nil> | getSecretValue -> value="v" err=<nil>
    ref="urn:k8s:secret::k"          ParseSecretReference -> err=malformed secret URN "urn:k8s:secret::k" | getSecretValue -> err=secret default/ is not found
    ref="urn:k8s:secret:s:"          ParseSecretReference -> err=malformed secret URN "urn:k8s:secret:s:" | getSecretValue -> err=key "" not found in secret default/s
    ref="xxx:yyy:zzz:s:k"            ParseSecretReference -> err=malformed secret URN "xxx:yyy:zzz:s:k" | getSecretValue -> value="v" err=<nil>
    ref="urn:k8s:secret:s:k:extra"   ParseSecretReference -> err=malformed secret URN ... | getSecretValue -> err=malformed secret URN ...
```

`getSecretValue` (kubernetes.go:852-856) does its own `strings.Split(urn, ":")` with only a `len(parts) != 5` check: it accepts a reference without the `urn:k8s:secret:` prefix and empty name/key, which `ParseSecretReference` rejects — two implementations with different acceptance.

Verdict: CONFIRMED (both parts).
