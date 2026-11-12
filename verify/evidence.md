Observations for the behavioral audit `v-63ba222f` of the Secret-reference task. Every section lists the commands that were run and the key output lines verbatim (lines longer than 640 characters are cut and end in `[…]`; the full captured output of every run is under `verify/logs/`). Go 1.26.3, linux/amd64. All claim branches and the control share the merge base `af6d927c5`. No production file is changed on this branch: probes are copied into detached worktrees (`~/wt/<id>`), and every production mutation used as a probe is reverted with `git checkout` by the script that applied it (each script prints the changed-file count before and after).

## C1

Claim: Secret-backed plugin construction fails to support execution with resolved values on configuration isolated from the original input (parts: *execution*, *input isolation*). Branches: `evalon/traefik-se-{5a526bc0,75aa045a,827fd201,073d30ad,43f17bfb}`; control: `traefik-secret-references-in-middlewares-perfect`.

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-5a526bc0 && git worktree add --detach ~/wt/5a526bc0 FETCH_HEAD   # 17ab152ce
git fetch evals evalon/traefik-se-75aa045a && git worktree add --detach ~/wt/75aa045a FETCH_HEAD   # dd2f44455
git fetch evals evalon/traefik-se-827fd201 && git worktree add --detach ~/wt/827fd201 FETCH_HEAD   # 57defaf57
git fetch evals evalon/traefik-se-073d30ad && git worktree add --detach ~/wt/073d30ad FETCH_HEAD   # 9937a6ec8
git fetch evals evalon/traefik-se-43f17bfb && git worktree add --detach ~/wt/43f17bfb FETCH_HEAD   # 9b7593ae6
# verify/repro/install.sh <worktree> copies the archived fixtures to the paths the eval provisions them at, plus the audit probes/plugins
```

```sh
for b in perfect 5a526bc0 75aa045a 827fd201 073d30ad 43f17bfb; do
  verify/repro/run_c1_c2.sh ~/wt/$b "$PWD/verify/logs/c1c2-$b.log"     # install.sh, then the two commands below
  verify/repro/run_retain.sh ~/wt/$b "$PWD/verify/logs/retain-$b.log"  # install.sh, then the third command below
done
# inside each worktree:
go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(SuccessExecution|TypedSecretField|NestedExecution|EscapedKeyExecution|InputMutationIsolation|BuildErrorRedaction|CachedErrorRedaction)$' ./pkg/provider/kubernetes/crd
go test -tags verify -count=1 -v -run '^TestVerify_C[12]_' ./pkg/provider/kubernetes/crd
go test -tags verify -count=1 -v -run '^TestVerify_Retain' ./pkg/provider/kubernetes/crd
```

Probes (all drive the production chain: CRD provider with a fake clientset → published `dynamic.Middleware` → `runtime` configuration → `middleware.Builder.BuildMiddlewareChain(...).Then` → production `plugins.Builder` running a local Yaegi plugin):

- `verify/repro/c1_c2_plugin_probe_test.go` + `verify/repro/verifyplugin/`: a plugin with a typed `int` field (`port`), a typed `bool`, a nested struct, a list of structs and a `,remain` map with the keys `a/b`, `m~n`, `~1`, `/`, `x.y-z` (literal siblings `a.b`, `/`, list entry `literal-0`, `keep`). The handler serves the configuration it was built with; the plugin rewrites its nested maps inside `New`.
- `verify/repro/c1_c5_retain_probe_test.go` + `verify/repro/verifyretain/`: a plugin that keeps the `*Config` it returned from `CreateConfig` and writes `MUTATED-AFTER-CALLBACK` into every nested map/list leaf of that configuration only after the decoder callback / `New` has returned (from its handler on a request, or from a later "fire" instance). The published and runtime configurations are compared (JSON and `reflect.DeepEqual`, Secret side data included) before and after.

### Part "execution" — archived fixtures and probe

Top-level results of the archived fixtures per branch (`--- PASS/FAIL` lines of the first command):

| worktree | SuccessExecution | TypedSecretField | NestedExecution | EscapedKeyExecution | InputMutationIsolation | BuildErrorRedaction | CachedErrorRedaction |
|---|---|---|---|---|---|---|---|
| perfect | PASS | PASS | PASS | PASS | PASS | PASS | PASS |
| 5a526bc0 | PASS | PASS | PASS | PASS | FAIL | FAIL | FAIL |
| 75aa045a | PASS | PASS | PASS | PASS | FAIL | FAIL | FAIL |
| 827fd201 | PASS | PASS | PASS | PASS | FAIL | FAIL | FAIL |
| 073d30ad | PASS | PASS | PASS | PASS | FAIL | FAIL | FAIL |
| 43f17bfb | PASS | PASS | PASS | PASS | FAIL | FAIL | FAIL |

Audit probe, `TestVerify_C1_Execution`, output on `073d30ad` (the three lines are identical on all five branches and on the control; `grep -c "resolved as expected = true"` is 1 in each `c1c2-*.log`):

```console
    verify_c1_c2_plugin_probe_test.go:175: C1 execution: plugin served decoded config = {"port":8443,"enabled":false,"token":"tok-VALUE","public":"PUBLIC-LITERAL","deep":{"inner":{"token":"deep-VALUE","label":"literal-label"}},"users":[{"name":"admin","secret":"admin-VALUE"},{"name":"user","secret":"user-VALUE"}],"remain":{"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"slash-VALUE","m~n":"tilde-VALUE","x.y-z":["literal-0",{"keep":"literal-keep","~0":"listed-VALUE"}],"~1":"escaped-VALUE"}}
    verify_c1_c2_plugin_probe_test.go:187: C1 execution: typed int field + nested + list + escaped keys resolved as expected = true
    verify_c1_c2_plugin_probe_test.go:188: C1 execution: published config unchanged = true, runtime config unchanged = true, DeepEqual (incl. Secret side map) unchanged = true
--- PASS: TestVerify_C1_Execution (0.06s)
```

| worktree | `TestVerify_C1_Execution` | "resolved as expected = true" lines |
|---|---|---|
| perfect | PASS | 1 |
| 5a526bc0 | PASS | 1 |
| 75aa045a | PASS | 1 |
| 827fd201 | PASS | 1 |
| 073d30ad | PASS | 1 |
| 43f17bfb | PASS | 1 |

The Secret values used were `port=8443`, `token=tok-VALUE`, `deep=deep-VALUE`, `u1=admin-VALUE`, `u2=user-VALUE`, `slash=slash-VALUE`, `tilde=tilde-VALUE`, `escaped=escaped-VALUE`, `listed=listed-VALUE`; the served JSON shows the string `8443` decoded into the `int` field, every nested / indexed / special-key reference replaced by its value, and every literal sibling preserved, while the published and runtime configurations still hold the URNs.

### Part "input isolation" — archived fixture

`TestEval_ProviderToBuilder_PluginInputMutationIsolation` fails on all five branches, only in its `decode_error` subtests. Where the failures are raised (count of `Error Trace` frames per fixture line in `c1c2-<branch>.log`; identical on all five branches):

```console
$ for b in 5a526bc0 75aa045a 827fd201 073d30ad 43f17bfb; do echo "== $b"; grep -oE 'eval_provider_to_builder_runtime_test.go:[0-9]+$' verify/logs/c1c2-$b.log | sed 's/eval_provider_to_builder_runtime_test.go/line/' | sort | uniq -c | tr -s ' ' | tr '\n' ';'; echo; done
== 5a526bc0
 12 line:1080; 15 line:227; 3 line:235; 2 line:672; 4 line:676; 6 line:763; 6 line:766; 20 line:770; 3 line:776; 4 line:884; 2 line:885; 2 line:888; 8 line:891;
== 75aa045a
 12 line:1080; 15 line:227; 3 line:235; 2 line:672; 4 line:676; 6 line:763; 6 line:766; 20 line:770; 3 line:776; 4 line:884; 2 line:885; 2 line:888; 8 line:891;
== 827fd201
 12 line:1080; 15 line:227; 3 line:235; 2 line:672; 4 line:676; 6 line:763; 6 line:766; 20 line:770; 3 line:776; 4 line:884; 2 line:885; 2 line:888; 8 line:891;
== 073d30ad
 12 line:1080; 15 line:227; 3 line:235; 2 line:672; 4 line:676; 6 line:763; 6 line:766; 20 line:770; 3 line:776; 4 line:884; 2 line:885; 2 line:888; 8 line:891;
== 43f17bfb
 12 line:1080; 15 line:227; 3 line:235; 2 line:672; 4 line:676; 6 line:763; 6 line:766; 20 line:770; 3 line:776; 4 line:884; 2 line:885; 2 line:888; 8 line:891;
$ for b in 5a526bc0 75aa045a 827fd201 073d30ad 43f17bfb; do grep -cE 'eval_provider_to_builder_runtime_test.go:(1087|1092)' verify/logs/c1c2-$b.log; done
0
0
0
0
0
```

Line 1080 is the call `evalAssertPluginErrorRedacted(t, buildErr, refs, credentialValues)`; line 227 inside it is `assert.Contains(t, text, ref, "the redacted error must name the URN reference")`. Lines 1087 and 1092 are the fixture's only ownership assertions (`assert.JSONEq(... "plugin mutations and error retries must not alter published configuration")` / `"... shared runtime configuration"`); they never fail. The failing message (same on every branch, here `5a526bc0`):

```console
    eval_provider_to_builder_runtime_test.go:1080:
        	Error Trace:	/home/ubuntu/wt/5a526bc0/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:227
        	            				/home/ubuntu/wt/5a526bc0/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:1080
        	Error:      	"plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'eval_reject' as int: strconv.ParseInt: parsing \"not-an-integer\": invalid syntax" does not contain "urn:k8s:secret:name:key"
        	Test:       	TestEval_ProviderToBuilder_PluginInputMutationIsolation/payload_equals_reference/decode_error
        	Messages:   	the redacted error must name the URN reference
```

The fixture sets the *literal* field `eval_reject` to `"not-an-integer"` (fixture line 1034) and requires the URN in the resulting error; the branches return the decoder error of that literal field without a URN. The fixture failure therefore measures the error wording, not a configuration mutation.

### Part "input isolation" — probe with a plugin that writes inside `New` (`TestVerify_C1_Isolation`)

Output on `073d30ad` (cases: success with the plugin rewriting nested maps in `New`; decode error on the Secret-bound `int` field; decode error confined to an unrelated literal field, replayed reference-only; literal-field error next to a Secret-bound typed field; constructor error after the plugin rewrote nested maps; and a positive control handing the published map straight to the plugin builder):

```console
    verify_c1_c2_plugin_probe_test.go:224: C1 isolation: published unchanged = true, runtime unchanged = true, DeepEqual (incl. Secret side map) unchanged = true
    verify_c1_c2_plugin_probe_test.go:218: C1 isolation: attempt 1 returned error = "plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:224: C1 isolation: published unchanged = true, runtime unchanged = true, DeepEqual (incl. Secret side map) unchanged = true
    verify_c1_c2_plugin_probe_test.go:218: C1 isolation: attempt 1 returned error = "plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:224: C1 isolation: published unchanged = true, runtime unchanged = true, DeepEqual (incl. Secret side map) unchanged = true
    verify_c1_c2_plugin_probe_test.go:218: C1 isolation: attempt 1 returned error = "plugin: failed to decode configuration: 2 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:224: C1 isolation: published unchanged = true, runtime unchanged = true, DeepEqual (incl. Secret side map) unchanged = true
    verify_c1_c2_plugin_probe_test.go:218: C1 isolation: attempt 1 returned error = "creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:escaped urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:port urn:k8s:secret:vsec:slash urn:k8s:secret:vsec:tilde urn:k8s:secret:vsec:token urn:k8s:secret:vsec:u1 urn:k8s:secret:vsec:u2] failed"
    verify_c1_c2_plugin_probe_test.go:224: C1 isolation: published unchanged = true, runtime unchanged = true, DeepEqual (incl. Secret side map) unchanged = true
    verify_c1_c2_plugin_probe_test.go:257: C1 isolation control: published unchanged = false (expected false: the probe detects aliasing)
--- PASS: TestVerify_C1_Isolation (0.02s)
    --- PASS: TestVerify_C1_Isolation/success_(plugin_mutates_nested_maps_in_New) (0.00s)
    --- PASS: TestVerify_C1_Isolation/decode_error_on_Secret-bound_int_field (0.00s)
    --- PASS: TestVerify_C1_Isolation/decode_error_confined_to_unrelated_literal_field_(reference-only_replay) (0.00s)
    --- PASS: TestVerify_C1_Isolation/literal-field_decode_error_next_to_a_Secret-bound_typed_field_(reference-only_replay) (0.00s)
    --- PASS: TestVerify_C1_Isolation/constructor_error_after_plugin_mutates_nested_maps (0.00s)
    --- PASS: TestVerify_C1_Isolation/control:_plugin_built_directly_on_the_published_map (0.00s)
```

| worktree | `TestVerify_C1_Isolation` | "published unchanged = true, runtime unchanged = true, DeepEqual … unchanged = true" lines (of 5 cases) | positive control "unchanged = false" |
|---|---|---|---|
| perfect | PASS | 5 | 1 |
| 5a526bc0 | PASS | 5 | 1 |
| 75aa045a | PASS | 5 | 1 |
| 827fd201 | PASS | 5 | 1 |
| 073d30ad | PASS | 5 | 1 |
| 43f17bfb | PASS | 5 | 1 |

### Part "input isolation" — probe with a plugin that keeps its decoded configuration (`TestVerify_Retain_PostCallbackOwnership`)

`RESULT` lines per worktree (`verify/logs/retain-<id>.log`; "unchanged" abbreviates the logged phrase "original published+runtime configuration unchanged after post-callback writes"):

`perfect`:

```console
RESULT success: handler writes to retained config after New returned: unchanged = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT constructor error with literal port (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
--- PASS: TestVerify_Retain_PostCallbackOwnership (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.114s
```

`5a526bc0`:

```console
RESULT success: handler writes to retained config after New returned: unchanged = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=28)
RESULT constructor error with literal port (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=28)
--- PASS: TestVerify_Retain_PostCallbackOwnership (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.111s
```

`75aa045a`:

```console
RESULT success: handler writes to retained config after New returned: unchanged = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=28)
RESULT constructor error with literal port (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=28)
--- PASS: TestVerify_Retain_PostCallbackOwnership (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.113s
```

`827fd201`:

```console
RESULT success: handler writes to retained config after New returned: unchanged = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=28)
RESULT constructor error with literal port (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=28)
--- PASS: TestVerify_Retain_PostCallbackOwnership (0.08s)
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.110s
```

`073d30ad`:

```console
RESULT success: handler writes to retained config after New returned: unchanged = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): unchanged = false (plugin rewrote leaves: handler=0 fire=28)
RESULT constructor error with literal port (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): unchanged = false (plugin rewrote leaves: handler=0 fire=28)
--- FAIL: TestVerify_Retain_PostCallbackOwnership (0.08s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.118s
```

`43f17bfb`:

```console
RESULT success: handler writes to retained config after New returned: unchanged = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): unchanged = false (plugin rewrote leaves: handler=0 fire=28)
RESULT constructor error with literal port (reference-only replay): unchanged = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): unchanged = false (plugin rewrote leaves: handler=0 fire=28)
--- FAIL: TestVerify_Retain_PostCallbackOwnership (0.08s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.115s
```

Full failing output on `073d30ad` (`43f17bfb` prints the same two failures and the same `published AFTER` documents):

```console
    verify_c1_c5_retain_probe_test.go:146: RESULT decode error on unrelated literal field (reference-only replay): original published+runtime configuration unchanged after post-callback writes = false (plugin rewrote leaves: handler=0 fire=28)
    verify_c1_c5_retain_probe_test.go:148:     published AFTER: {"plugin":{"vretain":{"deep":{"inner":{"label":"MUTATED-AFTER-CALLBACK","token":"MUTATED-AFTER-CALLBACK"}},"enabled":"not-a-bool","list":["MUTATED-AFTER-CALLBACK",{"name":"MUTATED-AFTER-CALLBACK","secret":"MUTATED-AFTER-CALLBACK"}],"plain":{"items":["MUTATED-AFTER-CALLBACK"],"nested":{"k":"MUTATED-AFTER-CALLBACK"}},"port":"urn:k8s:secret:vsec:port"}}}
    verify_c1_c5_retain_probe_test.go:150:
        	Error Trace:	/home/ubuntu/wt/073d30ad/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:150
        	Error:      	Should be true
        	Test:       	TestVerify_Retain_PostCallbackOwnership/decode_error_on_unrelated_literal_field_(reference-only_replay)
        	Messages:   	the original reference-bearing configuration must not be reachable from what the plugin retained
    verify_c1_c5_retain_probe_test.go:145:     returned error: "creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:u1] failed"
    verify_c1_c5_retain_probe_test.go:145:     returned error: "creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:u1] failed"
    verify_c1_c5_retain_probe_test.go:146: RESULT constructor error with literal port (reference-only replay): original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=0 fire=14)
    verify_c1_c5_retain_probe_test.go:145:     returned error: "plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
    verify_c1_c5_retain_probe_test.go:145:     returned error: "plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
    verify_c1_c5_retain_probe_test.go:146: RESULT decode error on Secret-bound int field (reference-only replay): original published+runtime configuration unchanged after post-callback writes = false (plugin rewrote leaves: handler=0 fire=28)
    verify_c1_c5_retain_probe_test.go:148:     published AFTER: {"plugin":{"vretain":{"deep":{"inner":{"label":"MUTATED-AFTER-CALLBACK","token":"MUTATED-AFTER-CALLBACK"}},"list":["MUTATED-AFTER-CALLBACK",{"name":"MUTATED-AFTER-CALLBACK","secret":"MUTATED-AFTER-CALLBACK"}],"plain":{"items":["MUTATED-AFTER-CALLBACK"],"nested":{"k":"MUTATED-AFTER-CALLBACK"}},"port":"urn:k8s:secret:vsec:port"}}}
    verify_c1_c5_retain_probe_test.go:150:
        	Error Trace:	/home/ubuntu/wt/073d30ad/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:150
        	Error:      	Should be true
        	Test:       	TestVerify_Retain_PostCallbackOwnership/decode_error_on_Secret-bound_int_field_(reference-only_replay)
        	Messages:   	the original reference-bearing configuration must not be reachable from what the plugin retained
--- FAIL: TestVerify_Retain_PostCallbackOwnership (0.08s)
    --- PASS: TestVerify_Retain_PostCallbackOwnership/control:_published_map_handed_directly_to_the_plugin_builder (0.07s)
    --- PASS: TestVerify_Retain_PostCallbackOwnership/success:_handler_writes_to_retained_config_after_New_returned (0.00s)
    --- FAIL: TestVerify_Retain_PostCallbackOwnership/decode_error_on_unrelated_literal_field_(reference-only_replay) (0.01s)
    --- PASS: TestVerify_Retain_PostCallbackOwnership/constructor_error_with_literal_port_(reference-only_replay) (0.00s)
    --- FAIL: TestVerify_Retain_PostCallbackOwnership/decode_error_on_Secret-bound_int_field_(reference-only_replay) (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.118s
FAIL
```

The positive control of this probe (published map handed directly to `plugins.Builder.Build`, decode fails on an unrelated literal field, then the plugin fires) passes on every worktree, i.e. the probe detects an alias when there is one:

```console
    verify_c1_c5_retain_probe_test.go:129: CONTROL direct decode error "failed to decode configuration: 2 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"not-a-bool\": invalid syntax\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"; plugin rewrote 7 leaves after decode returned; published unchanged = false (expected false)
    verify_c1_c5_retain_probe_test.go:129: CONTROL direct decode error "failed to decode configuration: 2 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"not-a-bool\": invalid syntax\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"; plugin rewrote 7 leaves after decode returned; published unchanged = false (expected false)
```

Where each branch replays the reference-only build (production code, unmodified):

```console
$ for b in 5a526bc0 75aa045a 827fd201 073d30ad 43f17bfb perfect; do echo "== $b"; (cd ~/wt/$b && grep -nE 'DeepCopy\(\)\.Plugin|[bB]uilder\.Build\(|ResolveURNRefs\(\)|ResolvePluginConf\(' pkg/server/middleware/*.go | grep -v _test); done
== 5a526bc0
pkg/server/middleware/middlewares.go:424:		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
pkg/server/middleware/plugins.go:52:	pluginType, rawPluginConfig, findErr := findPluginConfig(original.DeepCopy().Plugin)
pkg/server/middleware/plugins.go:57:	if _, refErr := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName); refErr != nil {
== 75aa045a
pkg/server/middleware/middlewares.go:441:		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
pkg/server/middleware/secrets.go:65:	pluginType, rawPluginConfig, err := findPluginConfig(conf.DeepCopy().Plugin)
pkg/server/middleware/secrets.go:70:	if _, err = b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName); err != nil {
== 827fd201
pkg/server/middleware/middlewares.go:432:		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
pkg/server/middleware/plugins.go:50:	if _, refConfig, findErr := findPluginConfig(original.DeepCopy().Plugin); findErr == nil {
pkg/server/middleware/plugins.go:51:		if _, refErr := b.pluginBuilder.Build(pluginType, refConfig, middlewareName); refErr != nil {
== 073d30ad
pkg/server/middleware/secrets.go:112:		return builder.Build(pluginType, config, middlewareName)
pkg/server/middleware/secrets.go:115:	plug, err := builder.Build(pluginType, secrets.ResolvePluginConf(config), middlewareName)
pkg/server/middleware/secrets.go:120:	if _, refErr := builder.Build(pluginType, config, middlewareName); refErr != nil {
== 43f17bfb
pkg/server/middleware/plugins.go:45:	pluginConfig, references := secrets.ResolvePluginConf(rawPluginConfig)
pkg/server/middleware/plugins.go:47:		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
pkg/server/middleware/plugins.go:51:	plug, err := b.pluginBuilder.Build(pluginType, pluginConfig, middlewareName)
pkg/server/middleware/plugins.go:58:	if _, refErr := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName); refErr != nil {
== perfect
pkg/server/middleware/middlewares.go:113:	config, err := info.Middleware.ResolveURNRefs()
pkg/server/middleware/middlewares.go:430:		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
```

On `5a526bc0`, `75aa045a` and `827fd201` the replay decodes a `DeepCopy()` of the original; on `073d30ad` (`secrets.go:120`, `config` is the middleware's own plugin map) and `43f17bfb` (`plugins.go:58`, `rawPluginConfig` is the middleware's own plugin map) the replay decodes the original map itself; the control never replays (it rewrites the error of the single build on its resolved copy).

### Summary of what was observed

| branch | execution: resolved typed/nested/indexed/escaped-key values delivered and plugin runs | isolation, plugin writes inside `New` (fixture JSONEq + `TestVerify_C1_Isolation`) | isolation, plugin writes through retained configuration after the callback returned |
|---|---|---|---|
| 5a526bc0 | yes | original unchanged | original unchanged (all 4 cases) |
| 75aa045a | yes | original unchanged | original unchanged (all 4 cases) |
| 827fd201 | yes | original unchanged | original unchanged (all 4 cases) |
| 073d30ad | yes | original unchanged | original **changed** in both decode-error replay cases (incl. the error confined to an unrelated literal field) |
| 43f17bfb | yes | original unchanged | original **changed** in both decode-error replay cases (incl. the error confined to an unrelated literal field) |
| perfect (control) | yes | original unchanged | original unchanged (all 4 cases) |

### Impact reasoning (`073d30ad`, `43f17bfb`)

When the build with resolved values fails while decoding, these two branches decode the *original* reference-bearing plugin map a second time to obtain an error that quotes references. The production decoder (`mapstructure`, `,remain` / `map` / `[]interface{}` fields) stores the original's nested maps and slices by reference in the `*Config` object the plugin created in `CreateConfig`. That object is plugin-owned memory; a plugin that still holds it can write to it at any later time, and every such write lands in the configuration that the API (`/api/rawdata`), the dashboard, the configuration hash and other routers read — observed above as `"label":"MUTATED-AFTER-CALLBACK"`, `"token":"MUTATED-AFTER-CALLBACK"` (a Secret reference overwritten), and the secret-free `plain` containers rewritten in the published document. The replayed decode itself writes nothing, and a plugin that only writes inside `New` cannot reach the original (its `New` is not called after a decode error), so the archived fixture and the first isolation probe stay green. The trigger needs (a) a decode error on the resolved configuration, e.g. one mistyped literal field, and (b) a plugin that keeps the object returned by `CreateConfig` and later writes to its nested containers. That is an unusual plugin, but nothing in the plugin contract forbids it, and the three sibling branches and the control are immune under the identical probe because the replay (if any) runs on a copy. Workaround for an operator: none short of fixing the configuration error; fix: replay on a deep copy (`conf.DeepCopy().Plugin`), as the sibling branches do.

## C2

Claim: returned or cached plugin-construction errors expose confidential values substituted from Secret-bound configuration fields. Branches: `evalon/traefik-se-{5a526bc0,75aa045a,827fd201,073d30ad,43f17bfb}`; control: `traefik-secret-references-in-middlewares-perfect`.

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-5a526bc0 && git worktree add --detach ~/wt/5a526bc0 FETCH_HEAD   # 17ab152ce
git fetch evals evalon/traefik-se-75aa045a && git worktree add --detach ~/wt/75aa045a FETCH_HEAD   # dd2f44455
git fetch evals evalon/traefik-se-827fd201 && git worktree add --detach ~/wt/827fd201 FETCH_HEAD   # 57defaf57
git fetch evals evalon/traefik-se-073d30ad && git worktree add --detach ~/wt/073d30ad FETCH_HEAD   # 9937a6ec8
git fetch evals evalon/traefik-se-43f17bfb && git worktree add --detach ~/wt/43f17bfb FETCH_HEAD   # 9b7593ae6
# verify/repro/install.sh <worktree> copies the archived fixtures to the paths the eval provisions them at, plus the audit probes/plugins
```

```sh
for b in perfect 5a526bc0 75aa045a 827fd201 073d30ad 43f17bfb; do
  verify/repro/run_c1_c2.sh ~/wt/$b "$PWD/verify/logs/c1c2-$b.log"   # install.sh, then, inside the worktree:
done
go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(SuccessExecution|TypedSecretField|NestedExecution|EscapedKeyExecution|InputMutationIsolation|BuildErrorRedaction|CachedErrorRedaction)$' ./pkg/provider/kubernetes/crd
go test -tags verify -count=1 -v -run '^TestVerify_C[12]_' ./pkg/provider/kubernetes/crd
```

### Archived fixtures `PluginBuildErrorRedaction` / `PluginCachedErrorRedaction`

| worktree | BuildErrorRedaction | failing subtests | CachedErrorRedaction | failing subtests |
|---|---|---|---|---|
| perfect | PASS | — | PASS | — |
| 5a526bc0 | FAIL | `decode`, `decode/escaped_payload`, `decode/field_label_collision_(token)`, `decode/single-line_quote_and_backslash_escaping`, `decode/type_label_collision_(string)`, `decode/unrelated_baseline`, `decode/whitespace-only_payload`, `decode_error` | FAIL | `decode_error` |
| 75aa045a | FAIL | `decode`, `decode/escaped_payload`, `decode/field_label_collision_(token)`, `decode/single-line_quote_and_backslash_escaping`, `decode/type_label_collision_(string)`, `decode/unrelated_baseline`, `decode/whitespace-only_payload`, `decode_error` | FAIL | `decode_error` |
| 827fd201 | FAIL | `decode`, `decode/escaped_payload`, `decode/field_label_collision_(token)`, `decode/single-line_quote_and_backslash_escaping`, `decode/type_label_collision_(string)`, `decode/unrelated_baseline`, `decode/whitespace-only_payload`, `decode_error` | FAIL | `decode_error` |
| 073d30ad | FAIL | `decode`, `decode/escaped_payload`, `decode/field_label_collision_(token)`, `decode/single-line_quote_and_backslash_escaping`, `decode/type_label_collision_(string)`, `decode/unrelated_baseline`, `decode/whitespace-only_payload`, `decode_error` | FAIL | `decode_error` |
| 43f17bfb | FAIL | `decode`, `decode/escaped_payload`, `decode/field_label_collision_(token)`, `decode/single-line_quote_and_backslash_escaping`, `decode/type_label_collision_(string)`, `decode/unrelated_baseline`, `decode/whitespace-only_payload`, `decode_error` | FAIL | `decode_error` |

Every failing subtest on the claim branches is a `decode` shape; the `constructor`, `constant`, `reject_resolved_value`, `wrapped_cause` and `format_…` shapes pass. First failing assertions on `073d30ad` (same text on the other four branches):

```console
    eval_provider_to_builder_runtime_test.go:672:
        	Error Trace:	/home/ubuntu/wt/073d30ad/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:227
        	            				/home/ubuntu/wt/073d30ad/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:672
        	Error:      	"plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'eval_reject' as int: strconv.ParseInt: parsing \"this_is_the_secret\": invalid syntax" does not contain "urn:k8s:secret:name:key"
        	Test:       	TestEval_ProviderToBuilder_PluginBuildErrorRedaction/decode_error
        	Messages:   	the redacted error must name the URN reference
    eval_provider_to_builder_runtime_test.go:672:
        	Error Trace:	/home/ubuntu/wt/073d30ad/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:235
        	            				/home/ubuntu/wt/073d30ad/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:672
        	Error:      	"plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'eval_reject' as int: strconv.ParseInt: parsing \"this_is_the_secret\": invalid syntax" should not contain "this_is_the_secret"
```

How the fixture produces that text (archived `eval_provider_to_builder_runtime_test.go`): the Secret `name/key` holds `this_is_the_secret` and is referenced by the plugin's `secret` field; the fixture then writes a second, *literal* entry into the published configuration and makes it equal to the Secret value:

```go
// line 671
observation := evalBuildPluginError(t, fixtureSecret, map[string]any{"eval_reject": fixtureSecret})
// evalObservePluginBuild, lines 329-333
pluginConfig["eval_trace_file"] = traceFile
for key, value := range keys {
	pluginConfig[key] = value
}
```

`eval_reject` is an `int` field of the plugin, so decoding fails on that literal. The quoted text `"this_is_the_secret"` in the branch errors is the content of the literal `eval_reject` entry of the provider-visible configuration; the Secret-bound field (`secret`) is not quoted. The fixture's failure therefore does not separate "text derived from the bound value" from "text of an unchanged public literal"; the probe below does.

### Probe `TestVerify_C2_ErrorConfidentiality` (public literals fixed, Secret value varied)

Four failing constructions, each run with Secret value #1 `qA7-distinct-SECRET-alpha` and #2 `rB8-other-"SECRET"\beta 'x'` while every literal stays fixed; each is built twice through the same `middleware.Builder` (first error + retry), and the probe collects the returned error, every text reachable through `errors.Unwrap` (single and multi-error), and the stored `MiddlewareInfo.Err`. It fails on any occurrence of the Secret value or of its fragments (`qA7`, `SECRET-alpha`, `distinct`, `rB8`, `beta`, `other`, …) outside the fixed public literal, and requires the diagnostics for #1 and #2 to be identical. A positive control builds the resolved configuration directly with `plugins.Builder` to show the raw decoder does quote the value.

Output on `073d30ad`:

```console
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#1 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#1="qA7-distinct-SECRET-alpha" stored(MiddlewareInfo.Err)=["plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#1: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#2 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" stored(MiddlewareInfo.Err)=["plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#2: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:321: C2 diagnostics identical for both Secret values (public literals fixed) = true
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:escaped urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:port urn:k8s:secret:vsec:slash urn:k8s:secret:vsec:tilde urn:k8s:secret:vsec:token urn:k8s:secret:vsec:u1 urn:k8s:secret:vsec:u2] failed"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#1 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#1="qA7-distinct-SECRET-alpha" stored(MiddlewareInfo.Err)=["creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:escaped urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:port urn:k8s:secret:vsec:slash urn:k8s:secret:vsec:tilde urn:k8s:secret:vsec:token urn:k8s:secret:vsec:u1 urn:k8s:secret:vsec:u2] failed"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#1: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" returned(1st)="creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:escaped urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:port urn:k8s:secret:vsec:slash urn:k8s:secret:vsec:tilde urn:k8s:secret:vsec:token urn:k8s:secret:vsec:u1 urn:k8s:secret:vsec:u2] failed"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#2 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" stored(MiddlewareInfo.Err)=["creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:escaped urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:port urn:k8s:secret:vsec:slash urn:k8s:secret:vsec:tilde urn:k8s:secret:vsec:token urn:k8s:secret:vsec:u1 urn:k8s:secret:vsec:u2] failed"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#2: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:321: C2 diagnostics identical for both Secret values (public literals fixed) = true
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#1 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#1="qA7-distinct-SECRET-alpha" stored(MiddlewareInfo.Err)=["plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#1: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#2 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" stored(MiddlewareInfo.Err)=["plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#2: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:321: C2 diagnostics identical for both Secret values (public literals fixed) = true
    verify_c1_c2_plugin_probe_test.go:324: C2 quoted text "PUBLIC-NOT-A-BOOL" originates from the fixed public literal: present for both Secret values = true
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#1 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#1="qA7-distinct-SECRET-alpha" stored(MiddlewareInfo.Err)=["plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#1: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:302: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
    verify_c1_c2_plugin_probe_test.go:303: C2 secret#2 retry on the same builder returns the same text = true
    verify_c1_c2_plugin_probe_test.go:304: C2 secret#2="rB8-other-\"SECRET\"\\beta 'x'" stored(MiddlewareInfo.Err)=["plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"]
    verify_c1_c2_plugin_probe_test.go:317: C2 secret#2: Secret-derived text in returned/retried/stored errors = false
    verify_c1_c2_plugin_probe_test.go:321: C2 diagnostics identical for both Secret values (public literals fixed) = true
    verify_c1_c2_plugin_probe_test.go:324: C2 quoted text "qA7-distinct-SECRET-alpha" originates from the fixed public literal: present for both Secret values = true
    verify_c1_c2_plugin_probe_test.go:337: C2 control: raw decoder error = "failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
--- PASS: TestVerify_C2_ErrorConfidentiality (0.02s)
    --- PASS: TestVerify_C2_ErrorConfidentiality/decode_error_on_Secret-bound_int_field_(port) (0.00s)
    --- PASS: TestVerify_C2_ErrorConfidentiality/constructor_error_quoting_Secret-bound_token (0.01s)
    --- PASS: TestVerify_C2_ErrorConfidentiality/decode_error_on_fixed_public_literal_field_(enabled) (0.00s)
    --- PASS: TestVerify_C2_ErrorConfidentiality/fixture_shape:_fixed_public_literal_equal_to_Secret_value_A (0.00s)
    --- PASS: TestVerify_C2_ErrorConfidentiality/control:_raw_plugins.Builder_error_for_the_resolved_configuration (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.115s
```

Counts per worktree (`grep -c` on `verify/logs/c1c2-<id>.log`):

| worktree | `TestVerify_C2_ErrorConfidentiality` | "Secret-derived text in returned/retried/stored errors = false" (of 8) | "… = true" | "diagnostics identical for both Secret values (public literals fixed) = true" (of 4) | "retry on the same builder returns the same text = true" (of 8) |
|---|---|---|---|---|---|
| perfect | PASS | 8 | 0 | 4 | 8 |
| 5a526bc0 | PASS | 8 | 0 | 4 | 8 |
| 75aa045a | PASS | 8 | 0 | 4 | 8 |
| 827fd201 | PASS | 8 | 0 | 4 | 8 |
| 073d30ad | PASS | 8 | 0 | 4 | 8 |
| 43f17bfb | PASS | 8 | 0 | 4 | 8 |

First returned error of each shape for Secret #1, per claim branch (wording differs per branch; none contains the Secret value except the fourth shape, where the quoted text is the fixed public literal and is unchanged for Secret #2):

`5a526bc0`:

```console
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: the error is withheld as it may reveal the values of urn:k8s:secret:vsec:deep, urn:k8s:secret:vsec:escaped, urn:k8s:secret:vsec:listed, urn:k8s:secret:vsec:port, urn:k8s:secret:vsec:slash, urn:k8s:secret:vsec:tilde, urn:k8s:secret:vsec:token, urn:k8s:secret:vsec:u1, urn:k8s:secret:vsec:u2"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
```

`75aa045a`:

```console
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="invalid configuration: the configuration holding the values of the Secret references urn:k8s:secret:vsec:deep, urn:k8s:secret:vsec:escaped, urn:k8s:secret:vsec:listed, urn:k8s:secret:vsec:port, urn:k8s:secret:vsec:slash, urn:k8s:secret:vsec:tilde, urn:k8s:secret:vsec:token, urn:k8s:secret:vsec:u1, urn:k8s:secret:vsec:u2  […]
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
```

`827fd201`:

```console
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="invalid configuration: the plugin rejected the values of urn:k8s:secret:vsec:deep, urn:k8s:secret:vsec:escaped, urn:k8s:secret:vsec:listed, urn:k8s:secret:vsec:port, urn:k8s:secret:vsec:slash, urn:k8s:secret:vsec:tilde, urn:k8s:secret:vsec:token, urn:k8s:secret:vsec:u1, urn:k8s:secret:vsec:u2"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
```

`073d30ad`:

```console
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="creating the plugin handler with the Secret references [urn:k8s:secret:vsec:deep urn:k8s:secret:vsec:escaped urn:k8s:secret:vsec:listed urn:k8s:secret:vsec:port urn:k8s:secret:vsec:slash urn:k8s:secret:vsec:tilde urn:k8s:secret:vsec:token urn:k8s:secret:vsec:u1 urn:k8s:secret:vsec:u2] failed"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
```

`43f17bfb`:

```console
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'port' as int: strconv.ParseInt: parsing \"urn:k8s:secret:vsec:port\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: creating middleware with a configuration using urn:k8s:secret:vsec:deep, urn:k8s:secret:vsec:escaped, urn:k8s:secret:vsec:listed, urn:k8s:secret:vsec:port, urn:k8s:secret:vsec:slash, urn:k8s:secret:vsec:tilde, urn:k8s:secret:vsec:token, urn:k8s:secret:vsec:u1, urn:k8s:secret:vsec:u2 failed, the error is not repor […]
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"PUBLIC-NOT-A-BOOL\": invalid syntax"
C2 secret#1="qA7-distinct-SECRET-alpha" returned(1st)="plugin: failed to decode configuration: 1 error(s) decoding:\n\n* cannot parse 'enabled' as bool: strconv.ParseBool: parsing \"qA7-distinct-SECRET-alpha\": invalid syntax"
```

Observed on all five branches: a decode error on the Secret-bound `int` field quotes the URN (`parsing "urn:k8s:secret:vsec:port"`), a constructor error formatted from the Secret-bound token is replaced by a message listing URNs only, a decode error on a fixed literal quotes that literal identically for both Secret values, and in the fixture shape (literal equal to Secret value #1) the quoted text stays `qA7-distinct-SECRET-alpha` when the Secret is changed to value #2. The control line shows the raw production decoder does quote the resolved value (`parsing "qA7-distinct-SECRET-alpha"`), so the probe reaches a leaking decoder and the builder output above is what removes it.

## C3

Claim: the added or modified tests do not assert indexed resolution with content-bearing siblings or special-character configuration-map keys. Branches: `evalon/traefik-se-{5fb40ae0,2cadadef,42ec8d90}`.

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-5fb40ae0 && git worktree add --detach ~/wt/5fb40ae0 FETCH_HEAD   # 34e01c8cb
git fetch evals evalon/traefik-se-2cadadef && git worktree add --detach ~/wt/2cadadef FETCH_HEAD   # acf97b578
git fetch evals evalon/traefik-se-42ec8d90 && git worktree add --detach ~/wt/42ec8d90 FETCH_HEAD   # 4af798f02
```

```sh
for b in 5fb40ae0 2cadadef 42ec8d90; do verify/repro/c3_probe.sh ~/wt/$b > verify/logs/c3-$b.log 2>&1; done
# the script runs, unmodified and then under each mutation (reverted with git checkout afterwards):
go test -count=1 -v -run '^TestLoadIngressRoutes$/^Simple_Ingress_Route,_with_test_middleware_read_config_from_(secret|deep_secret|an_array_of_secret)(#[0-9]+)?$' ./pkg/provider/kubernetes/crd
```

The test path under audit is the branch's own modification of the table test `TestLoadIngressRoutes` (`pkg/provider/kubernetes/crd/kubernetes_test.go`): each branch resolves the Secret references of the loaded configuration and then runs the existing `assert.Equal(t, test.expected, conf)`. The pre-existing table case `Simple Ingress Route, with test middleware read config from an array of secret` (second occurrence, `#01`, fixture `fixtures/with_plugin_read_array_of_map_contain_secret.yml`) expects, under the dashed plugin key `test-secret`, a list of maps `users: [{name: admin, secret: admin_password}, {name: user, secret: user_password}]` — a resolved indexed value next to a content-bearing literal sibling.

Mutations applied by the script: **F1** fixture, literal sibling `name: admin` → `admin-MUTATED-SIBLING`; **F2** fixture, Secret `key1` value → `MUTATED-VALUE`; **F3** fixture, dashed plugin configuration-map key `test-secret` → `test_secret`; **T1** test, the added resolve call removed; **P1** production, the resolver skips list entries.

### `5fb40ae0`

Modified hunk and baseline:

```diff
diff --git a/pkg/provider/kubernetes/crd/kubernetes_test.go b/pkg/provider/kubernetes/crd/kubernetes_test.go
index d4dd19655..c1dbe98d9 100644
--- a/pkg/provider/kubernetes/crd/kubernetes_test.go
+++ b/pkg/provider/kubernetes/crd/kubernetes_test.go
@@ -10212,6 +10212,13 @@ func TestLoadIngressRoutes(t *testing.T) {
 			require.NoError(t, p.Init())
 			conf := p.loadConfigurationFromCRD(t.Context(), client)
+			if conf.HTTP != nil {
+				for name, middleware := range conf.HTTP.Middlewares {
+					if len(middleware.Secrets) > 0 {
+						conf.HTTP.Middlewares[name] = middleware.Resolve()
+					}
+				}
+			}
 			assert.Equal(t, test.expected, conf)
```

```console
### branch HEAD: 34e01c8cb015e184f52820e1e1bfb3795a83d987
### git status (must be clean before): 0 changed files
### BASELINE (unmodified branch)
$ go test -count=1 -v -run '^TestLoadIngressRoutes$/^Simple_Ingress_Route,_with_test_middleware_read_config_from_(secret|deep_secret|an_array_of_secret)(#[0-9]+)?$' ./pkg/provider/kubernetes/crd
--- PASS: TestLoadIngressRoutes (0.00s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_deep_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.06s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.074s
```

Under each mutation (headline, assertion site, the differing lines of the `assert.Equal` diff, and result lines):

```console
### MUTATION F1 — fixture: literal sibling 'name: admin' -> 'name: admin-MUTATED-SIBLING' (does an assertion check the preserved content-bearing sibling of an indexed entry?)
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	-        (string) (len=4) "name": (string) (len=5) "admin",
        	            	+        (string) (len=4) "name": (string) (len=21) "admin-MUTATED-SIBLING",
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.071s
### MUTATION F2 — fixture: Secret key1 value -> 'MUTATED-VALUE' (does an assertion check the resolved value of indexed entry [0]?)
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	-        (string) (len=6) "secret": (string) (len=14) "admin_password"
        	            	+        (string) (len=6) "secret": (string) (len=13) "MUTATED-VALUE"
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.073s
### MUTATION F3 — fixture: dashed plugin configuration-map key 'test-secret' -> 'test_secret' (does an assertion check the preserved special-character key above the resolved value?)
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	-     (string) (len=11) "test-secret": (dynamic.PluginConf) (len=1) {
        	            	+     (string) (len=11) "test_secret": (dynamic.PluginConf) (len=1) {
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.06s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.074s
### MUTATION T1 — test: drop the added Resolve call before assert.Equal (is resolution on the asserted path?)
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	+     (string) (len=23) "urn:k8s:secret:name:key": (dynamic.Secret) {
        	            	-      (string) (len=6) "secret": (string) (len=18) "this_is_the_secret",
        	            	+      (string) (len=6) "secret": (string) (len=23) "urn:k8s:secret:name:key",
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	+     (string) (len=23) "urn:k8s:secret:name:key": (dynamic.Secret) {
### production mutation P1 diff:
-		for i := 0; i < v.Len(); i++ {
+		for i := 0; i < 0; i++ {
### MUTATION P1 — production: resolver skips list (indexed) entries
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	-        (string) (len=6) "secret": (string) (len=14) "admin_password"
        	            	+        (string) (len=6) "secret": (string) (len=24) "urn:k8s:secret:name:key1"
        	            	-        (string) (len=6) "secret": (string) (len=13) "user_password"
        	            	+        (string) (len=6) "secret": (string) (len=24) "urn:k8s:secret:name:key2"
        	Error Trace:	/home/ubuntu/wt/5fb40ae0/pkg/provider/kubernetes/crd/kubernetes_test.go:10222
        	            	-       (string) (len=12) "secret_data1",
        	            	-       (string) (len=12) "secret_data2"
--- FAIL: TestLoadIngressRoutes (0.02s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.24s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.26s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.356s
### git status (must be clean after): 0 changed files
```

### `2cadadef`

Modified hunk and baseline:

```diff
diff --git a/pkg/provider/kubernetes/crd/kubernetes_test.go b/pkg/provider/kubernetes/crd/kubernetes_test.go
index d4dd19655..5105fd072 100644
--- a/pkg/provider/kubernetes/crd/kubernetes_test.go
+++ b/pkg/provider/kubernetes/crd/kubernetes_test.go
@@ -10212,6 +10212,9 @@ func TestLoadIngressRoutes(t *testing.T) {
 			require.NoError(t, p.Init())
 			conf := p.loadConfigurationFromCRD(t.Context(), client)
+			for name, middleware := range conf.HTTP.Middlewares {
+				conf.HTTP.Middlewares[name] = middleware.ResolveSecrets()
+			}
 			assert.Equal(t, test.expected, conf)
 		})
 	}
@@ -14605,9 +14608,13 @@ func TestCreateBasicAuthCredentials(t *testing.T) {
 		<-eventCh
```

```console
### branch HEAD: acf97b5786d32ffe34791b77659bc5611a39f420
### git status (must be clean before): 0 changed files
### BASELINE (unmodified branch)
$ go test -count=1 -v -run '^TestLoadIngressRoutes$/^Simple_Ingress_Route,_with_test_middleware_read_config_from_(secret|deep_secret|an_array_of_secret)(#[0-9]+)?$' ./pkg/provider/kubernetes/crd
--- PASS: TestLoadIngressRoutes (0.00s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_deep_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_secret (0.06s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.06s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.074s
```

Under each mutation (headline, assertion site, the differing lines of the `assert.Equal` diff, and result lines):

```console
### MUTATION F1 — fixture: literal sibling 'name: admin' -> 'name: admin-MUTATED-SIBLING' (does an assertion check the preserved content-bearing sibling of an indexed entry?)
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-        (string) (len=4) "name": (string) (len=5) "admin",
        	            	+        (string) (len=4) "name": (string) (len=21) "admin-MUTATED-SIBLING",
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.30s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.322s
### MUTATION F2 — fixture: Secret key1 value -> 'MUTATED-VALUE' (does an assertion check the resolved value of indexed entry [0]?)
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-        (string) (len=6) "secret": (string) (len=14) "admin_password"
        	            	+        (string) (len=6) "secret": (string) (len=13) "MUTATED-VALUE"
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.075s
### MUTATION F3 — fixture: dashed plugin configuration-map key 'test-secret' -> 'test_secret' (does an assertion check the preserved special-character key above the resolved value?)
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-     (string) (len=11) "test-secret": (dynamic.PluginConf) (len=1) {
        	            	+     (string) (len=11) "test_secret": (dynamic.PluginConf) (len=1) {
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.06s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.079s
### MUTATION T1 — test: drop the added Resolve call before assert.Equal (is resolution on the asserted path?)
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-         (string) (len=6) "secret": (string) (len=28) "this_is_the_very_deep_secret",
        	            	+         (string) (len=6) "secret": (string) (len=23) "urn:k8s:secret:name:key",
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-       (string) (len=12) "secret_data1",
        	            	-       (string) (len=12) "secret_data2"
        	            	+       (string) (len=24) "urn:k8s:secret:name:key1",
        	            	+       (string) (len=24) "urn:k8s:secret:name:key2"
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
### production mutation P1 diff:
-		for i := range value.Len() {
+		for i := range 0 {
+			_ = value.Len()
### MUTATION P1 — production: resolver skips list (indexed) entries
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-        (string) (len=6) "secret": (string) (len=14) "admin_password"
        	            	+        (string) (len=6) "secret": (string) (len=24) "urn:k8s:secret:name:key1"
        	            	-        (string) (len=6) "secret": (string) (len=13) "user_password"
        	            	+        (string) (len=6) "secret": (string) (len=24) "urn:k8s:secret:name:key2"
        	Error Trace:	/home/ubuntu/wt/2cadadef/pkg/provider/kubernetes/crd/kubernetes_test.go:10218
        	            	-       (string) (len=12) "secret_data1",
        	            	-       (string) (len=12) "secret_data2"
--- FAIL: TestLoadIngressRoutes (0.01s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.07s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.07s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.095s
### git status (must be clean after): 0 changed files
```

### `42ec8d90`

Modified hunk and baseline:

```diff
diff --git a/pkg/provider/kubernetes/crd/kubernetes_test.go b/pkg/provider/kubernetes/crd/kubernetes_test.go
index d4dd19655..1383fffde 100644
--- a/pkg/provider/kubernetes/crd/kubernetes_test.go
+++ b/pkg/provider/kubernetes/crd/kubernetes_test.go
@@ -10212,6 +10212,11 @@ func TestLoadIngressRoutes(t *testing.T) {
 			require.NoError(t, p.Init())
 			conf := p.loadConfigurationFromCRD(t.Context(), client)
+			for name, middleware := range conf.HTTP.Middlewares {
+				resolved := middleware.ResolveSecrets()
+				resolved.Secrets = nil
+				conf.HTTP.Middlewares[name] = resolved
+			}
 			assert.Equal(t, test.expected, conf)
 		})
 	}
```

```console
### branch HEAD: 4af798f02eb3dff0a4e8d9bb52f7fe260786d333
### git status (must be clean before): 0 changed files
### BASELINE (unmodified branch)
$ go test -count=1 -v -run '^TestLoadIngressRoutes$/^Simple_Ingress_Route,_with_test_middleware_read_config_from_(secret|deep_secret|an_array_of_secret)(#[0-9]+)?$' ./pkg/provider/kubernetes/crd
--- PASS: TestLoadIngressRoutes (0.00s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.05s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_secret (0.05s)
    --- PASS: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_deep_secret (0.05s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.074s
```

Under each mutation (headline, assertion site, the differing lines of the `assert.Equal` diff, and result lines):

```console
### MUTATION F1 — fixture: literal sibling 'name: admin' -> 'name: admin-MUTATED-SIBLING' (does an assertion check the preserved content-bearing sibling of an indexed entry?)
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	            	-        (string) (len=4) "name": (string) (len=5) "admin",
        	            	+        (string) (len=4) "name": (string) (len=21) "admin-MUTATED-SIBLING",
--- FAIL: TestLoadIngressRoutes (0.01s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.14s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.177s
### MUTATION F2 — fixture: Secret key1 value -> 'MUTATED-VALUE' (does an assertion check the resolved value of indexed entry [0]?)
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	            	-        (string) (len=6) "secret": (string) (len=14) "admin_password"
        	            	+        (string) (len=6) "secret": (string) (len=13) "MUTATED-VALUE"
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.073s
### MUTATION F3 — fixture: dashed plugin configuration-map key 'test-secret' -> 'test_secret' (does an assertion check the preserved special-character key above the resolved value?)
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	            	-     (string) (len=11) "test-secret": (dynamic.PluginConf) (len=1) {
        	            	+     (string) (len=11) "test_secret": (dynamic.PluginConf) (len=1) {
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.071s
### MUTATION T1 — test: drop the added Resolve call before assert.Equal (is resolution on the asserted path?)
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	            	-         (string) (len=6) "secret": (string) (len=28) "this_is_the_very_deep_secret",
        	            	+         (string) (len=6) "secret": (string) (len=23) "urn:k8s:secret:name:key",
        	            	-       (string) (len=12) "secret_data1",
        	            	-       (string) (len=12) "secret_data2"
        	            	+       (string) (len=24) "urn:k8s:secret:name:key1",
        	            	+       (string) (len=24) "urn:k8s:secret:name:key2"
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
--- FAIL: TestLoadIngressRoutes (0.01s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.07s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.07s)
### production mutation P1 diff:
-		for i, value := range v {
+		for i, value := range v[:0] {
### MUTATION P1 — production: resolver skips list (indexed) entries
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	            	-        (string) (len=6) "secret": (string) (len=14) "admin_password"
        	            	+        (string) (len=6) "secret": (string) (len=24) "urn:k8s:secret:name:key1"
        	            	-        (string) (len=6) "secret": (string) (len=13) "user_password"
        	            	+        (string) (len=6) "secret": (string) (len=24) "urn:k8s:secret:name:key2"
        	Error Trace:	/home/ubuntu/wt/42ec8d90/pkg/provider/kubernetes/crd/kubernetes_test.go:10220
        	            	-       (string) (len=12) "secret_data1",
        	            	-       (string) (len=12) "secret_data2"
--- FAIL: TestLoadIngressRoutes (0.00s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret#01 (0.05s)
    --- FAIL: TestLoadIngressRoutes/Simple_Ingress_Route,_with_test_middleware_read_config_from_an_array_of_secret (0.05s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.074s
### git status (must be clean after): 0 changed files
```

### Summary of what was observed

| branch | added call before `assert.Equal` | F1 literal sibling | F2 resolved indexed value | F3 dashed key | T1 resolve call removed | P1 production skips list entries |
|---|---|---|---|---|---|---|
| 5fb40ae0 | `middleware.Resolve()` | test fails | test fails | test fails | test fails | test fails |
| 2cadadef | `middleware.ResolveSecrets()` | test fails | test fails | test fails | test fails | test fails |
| 42ec8d90 | `middleware.ResolveSecrets()` (+ `Secrets = nil`) | test fails | test fails | test fails | test fails | test fails |

On each branch the modified table path resolves the references and its equality assertion fails when the literal sibling of an indexed entry changes, when the resolved indexed value changes, when the dashed configuration-map key changes, when the added resolve call is removed, and when production stops resolving list entries. The script reports `0 changed files` before and after on every branch.

## C4

Claim: the added or modified regression tests lack assertions verifying sanitization of returned construction, decoding, or authentication errors after production redaction. Branches: `evalon/traefik-se-{5a6681ac,6f1a5ee0,85523a87}`.

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-5a6681ac && git worktree add --detach ~/wt/5a6681ac FETCH_HEAD   # 7de816dbc
git fetch evals evalon/traefik-se-6f1a5ee0 && git worktree add --detach ~/wt/6f1a5ee0 FETCH_HEAD   # bdc759086
git fetch evals evalon/traefik-se-85523a87 && git worktree add --detach ~/wt/85523a87 FETCH_HEAD   # f9769c349
```

```sh
for b in 5a6681ac 6f1a5ee0 85523a87; do python3 verify/repro/c4_probe.py ~/wt/$b $b > verify/logs/c4-$b.log 2>&1; done
```

The script runs the branch's *own* added test files (`pkg/server/middleware/secrets_test.go`, `pkg/middlewares/auth/secrets_test.go` or `secret_error_test.go`) unmodified, then under production mutations that put a confidential fragment back into the returned error or drop the URN: **R** the redactor appends the raw construction/decoding error; **U** the redactor drops the URN list; **V** the redactor keeps the URNs but appends the resolved Secret value; **A** the Secret-backed auth-entry error carries the malformed entry; **P** the auth parsers quote the malformed entry again (as on the base commit). Each mutation is reverted with `git checkout`; the script prints the changed-file count before and after.

Assertions under audit (branch test sources, unmodified):

```go
// 5a6681ac pkg/server/middleware/secrets_test.go:40-50 (TestSecretMiddlewareConstruction; plugin builder fails with "error decoding secret-value")
chain := builder.BuildMiddlewareChain(t.Context(), []string{"test"})
...
_, err = chain.Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
require.Error(t, err)
assert.Contains(t, err.Error(), ref)
assert.NotContains(t, err.Error(), "secret-value")
assert.NotContains(t, info.Err, "secret-value")

// 6f1a5ee0 pkg/server/middleware/secrets_test.go:75-88 (TestMiddlewareSecretErrors, kinds plugin / plugin constructor / basic / digest)
_, err := builder.BuildMiddlewareChain(t.Context(), []string{"test"}).Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
require.Error(t, err)
assert.Contains(t, err.Error(), ref)
assert.NotContains(t, err.Error(), value)
if kind == "basic" || kind == "digest" {
	assert.Contains(t, err.Error(), "entry 2")
}
// 6f1a5ee0 pkg/middlewares/auth/secrets_test.go:14-15 (TestInvalidUserDoesNotExposeCredentials)
assert.Contains(t, err.Error(), "entry 1")
assert.NotContains(t, err.Error(), "private-malformed-credential")

// 85523a87 pkg/server/middleware/secrets_test.go:61-66 (TestSecretPluginErrorAndConfigurationIsolation) and :75-81 (TestSecretMalformedAuthEntry)
_, err := chain.Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
require.Error(t, err)
assert.Contains(t, err.Error(), ref)
assert.NotContains(t, err.Error(), "decoder rejected value")
...
assert.Contains(t, err.Error(), "entry 2")
assert.Contains(t, err.Error(), ref)
assert.NotContains(t, err.Error(), "private-invalid")
// 85523a87 pkg/middlewares/auth/secret_error_test.go:12-15 (TestSecretAuthErrorsDoNotExposeEntries)
_, err := getUsers("", []string{"secret-invalid"}, parser)
require.Error(t, err)
assert.Contains(t, err.Error(), "entry 1")
assert.NotContains(t, err.Error(), "secret-invalid")
```

### `5a6681ac`

```console
### branch HEAD: 7de816dbc7456cf5079879f552da7565244b2588
### changed files before: 0
### added test files in scope: pkg/middlewares/auth/secrets_test.go pkg/server/middleware/secrets_test.go
### BASELINE (unmodified branch)
$ go test -count=1 -v -run '^(TestMalformedUserDoesNotExposeCredential|TestSecretMiddlewareConstruction|TestSecretBasicAuthConstruction|TestSecretTLSAndRedisResolution|TestSecretDigestAuthConstruction)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareConstruction (0.00s)
--- PASS: TestSecretBasicAuthConstruction (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretDigestAuthConstruction (0.00s)
--- PASS: TestMalformedUserDoesNotExposeCredential (0.00s)
### MUTATION R  (redactor appends the raw construction/decoding error after the URN list)
-	return fmt.Errorf("middleware construction failed with secret references: %s", strings.Join(refs, ", "))
+	return fmt.Errorf("middleware construction failed with secret references: %s: %w", strings.Join(refs, ", "), err)
### run under MUTATION R
$ go test -count=1 -v -run '^(TestMalformedUserDoesNotExposeCredential|TestSecretMiddlewareConstruction|TestSecretBasicAuthConstruction|TestSecretTLSAndRedisResolution|TestSecretDigestAuthConstruction)$' ./pkg/server/middleware ./pkg/middlewares/auth
        	Error:      	"middleware construction failed with secret references: urn:k8s:secret:test:key: plugin: error decoding secret-value" should not contain "secret-value"
--- FAIL: TestSecretMiddlewareConstruction (0.00s)
--- PASS: TestSecretBasicAuthConstruction (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretDigestAuthConstruction (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.022s
--- PASS: TestMalformedUserDoesNotExposeCredential (0.00s)
### MUTATION U  (redactor drops the URN list)
-	return fmt.Errorf("middleware construction failed with secret references: %s", strings.Join(refs, ", "))
+	return fmt.Errorf("middleware construction failed%s", strings.Repeat("", len(refs)))
### run under MUTATION U
$ go test -count=1 -v -run '^(TestMalformedUserDoesNotExposeCredential|TestSecretMiddlewareConstruction|TestSecretBasicAuthConstruction|TestSecretTLSAndRedisResolution|TestSecretDigestAuthConstruction)$' ./pkg/server/middleware ./pkg/middlewares/auth
        	Error:      	"middleware construction failed" does not contain "urn:k8s:secret:test:key"
--- FAIL: TestSecretMiddlewareConstruction (0.00s)
--- PASS: TestSecretBasicAuthConstruction (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretDigestAuthConstruction (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestMalformedUserDoesNotExposeCredential (0.00s)
### MUTATION V  (redactor keeps the URN list but appends only the resolved Secret value)
-	return fmt.Errorf("middleware construction failed with secret references: %s", strings.Join(refs, ", "))
+	return fmt.Errorf("middleware construction failed with secret references: %s: %s", strings.Join(refs, ", "), config.SecretValue(refs[0]))
### run under MUTATION V
$ go test -count=1 -v -run '^(TestMalformedUserDoesNotExposeCredential|TestSecretMiddlewareConstruction|TestSecretBasicAuthConstruction|TestSecretTLSAndRedisResolution|TestSecretDigestAuthConstruction)$' ./pkg/server/middleware ./pkg/middlewares/auth
        	Error:      	"middleware construction failed with secret references: urn:k8s:secret:test:key: secret-value" should not contain "secret-value"
--- FAIL: TestSecretMiddlewareConstruction (0.00s)
--- PASS: TestSecretBasicAuthConstruction (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretDigestAuthConstruction (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestMalformedUserDoesNotExposeCredential (0.00s)
### MUTATION A  (Secret-backed auth entry error carries the malformed entry through the redactor)
-				return nil, &SecretUserError{Reference: entry, Index: i + 1}
+				return nil, fmt.Errorf("%w: %s", &SecretUserError{Reference: entry, Index: i + 1}, user)
-		return userErr
+		return err
### run under MUTATION A
$ go test -count=1 -v -run '^(TestMalformedUserDoesNotExposeCredential|TestSecretMiddlewareConstruction|TestSecretBasicAuthConstruction|TestSecretTLSAndRedisResolution|TestSecretDigestAuthConstruction)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareConstruction (0.00s)
        	Error:      	"invalid user entry 2 in urn:k8s:secret:test:users: secret-value" should not contain "secret-value"
--- FAIL: TestSecretBasicAuthConstruction (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
        	Error:      	"invalid user entry 2 in urn:k8s:secret:test:users: malformed-secret" should not contain "malformed-secret"
--- FAIL: TestSecretDigestAuthConstruction (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
--- PASS: TestMalformedUserDoesNotExposeCredential (0.00s)
### MUTATION P  (auth parsers quote the malformed entry again, as on the base commit)
-		return "", "", fmt.Errorf("error parsing BasicUser")
+		return "", "", fmt.Errorf("error parsing BasicUser: %v", user)
-		return "", "", fmt.Errorf("error parsing DigestUser")
+		return "", "", fmt.Errorf("error parsing DigestUser: %v", user)
### run under MUTATION P
$ go test -count=1 -v -run '^(TestMalformedUserDoesNotExposeCredential|TestSecretMiddlewareConstruction|TestSecretBasicAuthConstruction|TestSecretTLSAndRedisResolution|TestSecretDigestAuthConstruction)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareConstruction (0.00s)
--- PASS: TestSecretBasicAuthConstruction (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretDigestAuthConstruction (0.00s)
        	Error:      	"invalid user entry 1: error parsing BasicUser: secret-value" should not contain "secret-value"
        	Error:      	"invalid user entry 1: error parsing DigestUser: secret-value" should not contain "secret-value"
--- FAIL: TestMalformedUserDoesNotExposeCredential (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.020s
### changed files after: 0
```

### `6f1a5ee0`

```console
### branch HEAD: bdc7590866e8a518085dcb5812130e12256903d8
### changed files before: 0
### added test files in scope: pkg/middlewares/auth/secrets_test.go pkg/server/middleware/secrets_test.go
### BASELINE (unmodified branch)
$ go test -count=1 -v -run '^(TestInvalidUserDoesNotExposeCredentials|TestMiddlewareSecretConstruction|TestMiddlewareSecretErrors)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestMiddlewareSecretConstruction (0.00s)
--- PASS: TestMiddlewareSecretErrors (0.00s)
--- PASS: TestInvalidUserDoesNotExposeCredentials (0.00s)
### MUTATION R  (redactor appends the raw construction/decoding error after the URN list)
-	return fmt.Errorf("cannot build middleware using secrets %s", refs)
+	return fmt.Errorf("cannot build middleware using secrets %s: %w", refs, err)
### run under MUTATION R
$ go test -count=1 -v -run '^(TestInvalidUserDoesNotExposeCredentials|TestMiddlewareSecretConstruction|TestMiddlewareSecretErrors)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestMiddlewareSecretConstruction (0.00s)
        	Error:      	"cannot build middleware using secrets urn:k8s:secret:credentials:key: plugin: error decoding value \"error\"" should not contain "error"
        	Error:      	"cannot build middleware using secrets urn:k8s:secret:credentials:key: error constructing plugin with \"error\"" should not contain "error"
--- FAIL: TestMiddlewareSecretErrors (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestInvalidUserDoesNotExposeCredentials (0.00s)
### MUTATION U  (redactor drops the URN list)
-	return fmt.Errorf("cannot build middleware using secrets %s", refs)
+	return fmt.Errorf("cannot build middleware using secrets%s", "")
### run under MUTATION U
$ go test -count=1 -v -run '^(TestInvalidUserDoesNotExposeCredentials|TestMiddlewareSecretConstruction|TestMiddlewareSecretErrors)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestMiddlewareSecretConstruction (0.00s)
        	Error:      	"cannot build middleware using secrets" does not contain "urn:k8s:secret:credentials:key"
        	Error:      	"{\"plugin\":{\"test\":{\"token\":\"urn:k8s:secret:credentials:key\"}},\"error\":[\"cannot build middleware using secrets\"],\"status\":\"disabled\"}" does not contain "\"error\":[\"cannot build middleware using secrets "
        	Error:      	"cannot build middleware using secrets" does not contain "urn:k8s:secret:credentials:key"
        	Error:      	"{\"plugin\":{\"test\":{\"token\":\"urn:k8s:secret:credentials:key\"}},\"error\":[\"cannot build middleware using secrets\"],\"status\":\"disabled\"}" does not contain "\"error\":[\"cannot build middleware using secrets "
--- FAIL: TestMiddlewareSecretErrors (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestInvalidUserDoesNotExposeCredentials (0.00s)
### MUTATION V  (redactor keeps the URN list but appends only the resolved plugin configuration)
-	return fmt.Errorf("cannot build middleware using secrets %s", refs)
+	return fmt.Errorf("cannot build middleware using secrets %s: %v", refs, config.Resolve().Plugin)
### run under MUTATION V
$ go test -count=1 -v -run '^(TestInvalidUserDoesNotExposeCredentials|TestMiddlewareSecretConstruction|TestMiddlewareSecretErrors)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestMiddlewareSecretConstruction (0.00s)
        	Error:      	"cannot build middleware using secrets urn:k8s:secret:credentials:key: map[test:map[token:error]]" should not contain "error"
        	Error:      	"cannot build middleware using secrets urn:k8s:secret:credentials:key: map[test:map[token:error]]" should not contain "error"
--- FAIL: TestMiddlewareSecretErrors (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestInvalidUserDoesNotExposeCredentials (0.00s)
### MUTATION A  (auth parsers quote the malformed entry AND the redactor appends the raw auth error)
-		return "", "", errors.New("error parsing BasicUser")
+		return "", "", errors.New("error parsing BasicUser: " + user)
-		return "", "", errors.New("error parsing DigestUser")
+		return "", "", errors.New("error parsing DigestUser: " + user)
-		return fmt.Errorf("invalid user entry %d in %s", userErr.Index, refs)
+		return fmt.Errorf("invalid user entry %d in %s: %w", userErr.Index, refs, err)
### run under MUTATION A
$ go test -count=1 -v -run '^(TestInvalidUserDoesNotExposeCredentials|TestMiddlewareSecretConstruction|TestMiddlewareSecretErrors)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestMiddlewareSecretConstruction (0.00s)
--- PASS: TestMiddlewareSecretErrors (0.00s)
        	Error:      	"user entry 1: error parsing BasicUser: private-malformed-credential" should not contain "private-malformed-credential"
        	Error:      	"user entry 1: error parsing DigestUser: private-malformed-credential" should not contain "private-malformed-credential"
--- FAIL: TestInvalidUserDoesNotExposeCredentials (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.022s
### MUTATION P  (auth parsers quote the malformed entry again, as on the base commit)
-		return "", "", errors.New("error parsing BasicUser")
+		return "", "", errors.New("error parsing BasicUser: " + user)
-		return "", "", errors.New("error parsing DigestUser")
+		return "", "", errors.New("error parsing DigestUser: " + user)
### run under MUTATION P
$ go test -count=1 -v -run '^(TestInvalidUserDoesNotExposeCredentials|TestMiddlewareSecretConstruction|TestMiddlewareSecretErrors)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestMiddlewareSecretConstruction (0.00s)
--- PASS: TestMiddlewareSecretErrors (0.00s)
        	Error:      	"user entry 1: error parsing BasicUser: private-malformed-credential" should not contain "private-malformed-credential"
        	Error:      	"user entry 1: error parsing DigestUser: private-malformed-credential" should not contain "private-malformed-credential"
--- FAIL: TestInvalidUserDoesNotExposeCredentials (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.023s
### changed files after: 0
```

### `85523a87`

```console
### branch HEAD: f9769c349063f114982c649140df84338a501dbb
### changed files before: 0
### added test files in scope: pkg/middlewares/auth/secret_error_test.go pkg/server/middleware/secrets_test.go
### BASELINE (unmodified branch)
$ go test -count=1 -v -run '^(TestSecretAuthErrorsDoNotExposeEntries|TestSecretMiddlewareAuthenticationAndRotation|TestSecretPluginErrorAndConfigurationIsolation|TestSecretMalformedAuthEntry|TestSecretTLSAndRedisResolution|TestSecretBasicAuthTypedCredentials|TestSecretNestedPluginResolution|TestSecretDigestErrorIdentifiesEntry)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareAuthenticationAndRotation (0.00s)
--- PASS: TestSecretPluginErrorAndConfigurationIsolation (0.00s)
--- PASS: TestSecretMalformedAuthEntry (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretBasicAuthTypedCredentials (0.00s)
--- PASS: TestSecretNestedPluginResolution (0.00s)
--- PASS: TestSecretDigestErrorIdentifiesEntry (0.00s)
--- PASS: TestSecretAuthErrorsDoNotExposeEntries (0.00s)
### MUTATION R  (plugin Build error: redacted error wraps the raw decoder error)
-				return nil, pluginSecretError(config.Secrets)
+				return nil, fmt.Errorf("%w: %w", pluginSecretError(config.Secrets), err)
### run under MUTATION R
$ go test -count=1 -v -run '^(TestSecretAuthErrorsDoNotExposeEntries|TestSecretMiddlewareAuthenticationAndRotation|TestSecretPluginErrorAndConfigurationIsolation|TestSecretMalformedAuthEntry|TestSecretTLSAndRedisResolution|TestSecretBasicAuthTypedCredentials|TestSecretNestedPluginResolution|TestSecretDigestErrorIdentifiesEntry)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareAuthenticationAndRotation (0.00s)
        	Error:      	"plugin configuration or initialization failed with secret references [\"urn:k8s:secret:name:token\"]: decoder rejected value: configuration" should not contain "decoder rejected value"
--- FAIL: TestSecretPluginErrorAndConfigurationIsolation (0.00s)
--- PASS: TestSecretMalformedAuthEntry (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretBasicAuthTypedCredentials (0.00s)
--- PASS: TestSecretNestedPluginResolution (0.00s)
--- PASS: TestSecretDigestErrorIdentifiesEntry (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.025s
--- PASS: TestSecretAuthErrorsDoNotExposeEntries (0.00s)
### MUTATION U  (redactor drops the URN list)
-	return fmt.Errorf("plugin configuration or initialization failed with secret references %q", secrets.References())
+	return fmt.Errorf("plugin configuration or initialization failed%s", "")
### run under MUTATION U
$ go test -count=1 -v -run '^(TestSecretAuthErrorsDoNotExposeEntries|TestSecretMiddlewareAuthenticationAndRotation|TestSecretPluginErrorAndConfigurationIsolation|TestSecretMalformedAuthEntry|TestSecretTLSAndRedisResolution|TestSecretBasicAuthTypedCredentials|TestSecretNestedPluginResolution|TestSecretDigestErrorIdentifiesEntry)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareAuthenticationAndRotation (0.00s)
        	Error:      	"plugin configuration or initialization failed" does not contain "urn:k8s:secret:name:token"
--- FAIL: TestSecretPluginErrorAndConfigurationIsolation (0.00s)
--- PASS: TestSecretMalformedAuthEntry (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretBasicAuthTypedCredentials (0.00s)
--- PASS: TestSecretNestedPluginResolution (0.00s)
--- PASS: TestSecretDigestErrorIdentifiesEntry (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestSecretAuthErrorsDoNotExposeEntries (0.00s)
### MUTATION V  (plugin Build error: redacted error keeps the URN list but appends only the resolved Secret value)
-				return nil, pluginSecretError(config.Secrets)
+				return nil, fmt.Errorf("%w: %v", pluginSecretError(config.Secrets), rawPluginConfig["token"])
### run under MUTATION V
$ go test -count=1 -v -run '^(TestSecretAuthErrorsDoNotExposeEntries|TestSecretMiddlewareAuthenticationAndRotation|TestSecretPluginErrorAndConfigurationIsolation|TestSecretMalformedAuthEntry|TestSecretTLSAndRedisResolution|TestSecretBasicAuthTypedCredentials|TestSecretNestedPluginResolution|TestSecretDigestErrorIdentifiesEntry)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareAuthenticationAndRotation (0.00s)
--- PASS: TestSecretPluginErrorAndConfigurationIsolation (0.00s)
--- PASS: TestSecretMalformedAuthEntry (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretBasicAuthTypedCredentials (0.00s)
--- PASS: TestSecretNestedPluginResolution (0.00s)
--- PASS: TestSecretDigestErrorIdentifiesEntry (0.00s)
--- PASS: TestSecretAuthErrorsDoNotExposeEntries (0.00s)
### MUTATION A  (Secret-backed auth entry error quotes the malformed entry)
-				return nil, fmt.Errorf("error parsing %s: entry %d from %q", kind, i+1, refs)
+				return nil, fmt.Errorf("error parsing %s: entry %d from %q: %s", kind, i+1, refs, entry)
### run under MUTATION A
$ go test -count=1 -v -run '^(TestSecretAuthErrorsDoNotExposeEntries|TestSecretMiddlewareAuthenticationAndRotation|TestSecretPluginErrorAndConfigurationIsolation|TestSecretMalformedAuthEntry|TestSecretTLSAndRedisResolution|TestSecretBasicAuthTypedCredentials|TestSecretNestedPluginResolution|TestSecretDigestErrorIdentifiesEntry)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareAuthenticationAndRotation (0.00s)
--- PASS: TestSecretPluginErrorAndConfigurationIsolation (0.00s)
        	Error:      	"error parsing BasicUser: entry 2 from \"urn:k8s:secret:auth:users\": private-invalid" should not contain "private-invalid"
--- FAIL: TestSecretMalformedAuthEntry (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretBasicAuthTypedCredentials (0.00s)
--- PASS: TestSecretNestedPluginResolution (0.00s)
        	Error:      	"error parsing DigestUser: entry 2 from \"urn:k8s:secret:auth:users\": invalid:private" should not contain "invalid:private"
--- FAIL: TestSecretDigestErrorIdentifiesEntry (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/server/middleware	0.023s
--- PASS: TestSecretAuthErrorsDoNotExposeEntries (0.00s)
### MUTATION P  (getUsers quotes the malformed entry)
-			return nil, fmt.Errorf("invalid user entry %d", i+1)
+			return nil, fmt.Errorf("invalid user entry %d: %s", i+1, user)
### run under MUTATION P
$ go test -count=1 -v -run '^(TestSecretAuthErrorsDoNotExposeEntries|TestSecretMiddlewareAuthenticationAndRotation|TestSecretPluginErrorAndConfigurationIsolation|TestSecretMalformedAuthEntry|TestSecretTLSAndRedisResolution|TestSecretBasicAuthTypedCredentials|TestSecretNestedPluginResolution|TestSecretDigestErrorIdentifiesEntry)$' ./pkg/server/middleware ./pkg/middlewares/auth
--- PASS: TestSecretMiddlewareAuthenticationAndRotation (0.00s)
--- PASS: TestSecretPluginErrorAndConfigurationIsolation (0.00s)
--- PASS: TestSecretMalformedAuthEntry (0.00s)
--- PASS: TestSecretTLSAndRedisResolution (0.00s)
--- PASS: TestSecretBasicAuthTypedCredentials (0.00s)
--- PASS: TestSecretNestedPluginResolution (0.00s)
--- PASS: TestSecretDigestErrorIdentifiesEntry (0.00s)
        	Error:      	"invalid user entry 1: secret-invalid" should not contain "secret-invalid"
        	Error:      	"invalid user entry 1: secret-invalid" should not contain "secret-invalid"
--- FAIL: TestSecretAuthErrorsDoNotExposeEntries (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.023s
### changed files after: 0
```

### Summary of what was observed

| branch | baseline | R raw error appended | U URN dropped | V resolved value appended | A auth entry leaked through redactor | P parser quotes entry |
|---|---|---|---|---|---|---|
| 5a6681ac | all pass | `TestSecretMiddlewareConstruction` fails (`should not contain "secret-value"`) | fails (`does not contain "urn:k8s:secret:test:key"`) | fails | `TestSecretBasicAuthConstruction`, `TestSecretDigestAuthConstruction` fail | `TestMalformedUserDoesNotExposeCredential` fails |
| 6f1a5ee0 | all pass | `TestMiddlewareSecretErrors` fails | fails (returned error and marshalled stored error) | fails | `TestInvalidUserDoesNotExposeCredentials` fails; `TestMiddlewareSecretErrors` basic/digest still pass | `TestInvalidUserDoesNotExposeCredentials` fails |
| 85523a87 | all pass | `TestSecretPluginErrorAndConfigurationIsolation` fails | fails (`does not contain "urn:k8s:secret:name:token"`) | **all pass** | `TestSecretMalformedAuthEntry`, `TestSecretDigestErrorIdentifiesEntry` fail | `TestSecretAuthErrorsDoNotExposeEntries` fails |

Each branch has at least one added test that takes the error returned by `BuildMiddlewareChain(...).Then` after production redaction, requires the URN, and fails when a confidential fragment is put back; each also has an added auth test that fails when a parser or the Secret-backed entry error quotes the malformed entry. Weaker spots that were observed and do not change those results: on `85523a87` the plugin assertion checks the absence of the fixed phrase `decoder rejected value` rather than of the resolved value, so mutation V (value appended without that phrase) stays green there; on `6f1a5ee0` the basic/digest subtests of `TestMiddlewareSecretErrors` compare against the whole multi-line Secret value and stay green under mutation A (the auth-package test catches it); on `5a6681ac` `assert.NotContains(t, info.Err, "secret-value")` is an element check on a `[]string` and cannot fail on a substring, while the returned-error assertion one line above is the live one.

## C5

Claim: the provided plugin-execution tests omit production integer decoding and post-callback configuration-ownership checks (parts: *production typed-decoding coverage*, *retained configuration coverage*). Branch: `evalon/traefik-se-fa481ef5`; control: `traefik-secret-references-in-middlewares-perfect`. "Provided tests" are the archived fixtures `tests/eval_provider_to_builder_runtime_test.go` and `tests/eval_local_plugin.go` (the branch carries no copy of its own).

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-fa481ef5 && git worktree add --detach ~/wt/fa481ef5 FETCH_HEAD   # a7df04b85
# verify/repro/install.sh <worktree> copies the archived fixtures to the paths the eval provisions them at, plus the audit probes/plugins
```

```sh
for b in fa481ef5 perfect; do
  verify/repro/install.sh ~/wt/$b && verify/repro/c5_probe.sh ~/wt/$b > verify/logs/c5-$b.log 2>&1
  verify/repro/run_retain.sh ~/wt/$b "$PWD/verify/logs/retain-$b.log"
done
# c5_probe.sh runs, at baseline and under mutation M1 (reverted with git checkout by a trap):
go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(TypedSecretField|SuccessExecution|NestedExecution|EscapedKeyExecution|InputMutationIsolation)$' ./pkg/provider/kubernetes/crd
go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' ./pkg/provider/kubernetes/crd
go test -count=1 -v -run '^TestEval_' ./pkg/provider/kubernetes/crd    # every provided test of the package, result lines sorted and diffed baseline vs M1
```

The script output is filtered through `sort | uniq -c`, so each line below is prefixed by its occurrence count.

### What the provided probes do (assertion text, `fa481ef5` worktree after `install.sh`)

```console
### branch HEAD: a7df04b8564f11d61146c19d1d65d7c34b23f214
### provided typed-Secret probe (eval_local_plugin.go) and every eval_reject assignment in the provided tests
44:	DecodeReject    int                    `mapstructure:"eval_reject"`
94:	secretInt, convErr := strconv.Atoi(fmt.Sprint(config.Remain["secret"]))
98:		SecretIsInt: convErr == nil,
671:		observation := evalBuildPluginError(t, fixtureSecret, map[string]any{"eval_reject": fixtureSecret})
700:		{name: "decode", keys: map[string]any{"eval_reject": "placeholder"}},
751:					if _, decodes := keys["eval_reject"]; decodes {
759:						keys["eval_reject"] = tc.secretVal
864:		{name: "decode error", keys: map[string]any{"eval_reject": secretValue}},
1034:					pluginConfig["eval_reject"] = "not-an-integer"
### where the provided plugin writes to its configuration, and what it keeps (eval_local_plugin.go)
56:func CreateConfig() *Config {
57:	return &Config{}
86:func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
105:	evalMutateNested(config.Remain)
117:		_, _ = rw.Write(body)
### ownership assertions of the provided isolation test
1087:					assert.JSONEq(t, string(expected), string(actualPublished), "plugin mutations and error retries must not alter published configuration")
1092:					assert.JSONEq(t, string(expected), string(actualRuntime), "plugin mutations and error retries must not alter shared runtime configuration")

```

- The typed-Secret probe: the plugin's only typed `int` field is `DecodeReject` (`eval_reject`); every `eval_reject` assignment in the provided tests is a literal the test writes itself (`fixtureSecret`, `"placeholder"`, `tc.secretVal`, `secretValue`, `"not-an-integer"` — all strings used to make decoding *fail*), never a URN bound to a Secret. `PluginTypedSecretField` reads the resolved value from the untyped `Remain["secret"]` map entry and converts it with `strconv.Atoi` inside `New` (plugin line 94).
- The ownership probe: `CreateConfig` returns a fresh `&Config{}` that the plugin does not keep; the only writes are `evalMutateNested(config.Remain)` inside `New` (line 105); the handler only writes the response body. The two `assert.JSONEq` ownership assertions (lines 1087/1092) run right after the build.

### Part "production typed-decoding coverage" — mutation M1

M1 changes production so that a resolved Secret string can no longer be decoded into an `int` field:

```diff
--- a/pkg/plugins/middlewareyaegi.go
+++ b/pkg/plugins/middlewareyaegi.go
-		WeaklyTypedInput: true,
+		WeaklyTypedInput: false,
```

`fa481ef5` — provided fixtures and audit probes, baseline then M1 (top-level results, the typed case, and the audit probes):

```console
### BASELINE (unmodified production)
$ go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(TypedSecretField|SuccessExecution|NestedExecution|EscapedKeyExecution|InputMutationIsolation)$' ./pkg/provider/kubernetes/crd   # provided fixtures
     24         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:227
      1 --- FAIL: TestEval_ProviderToBuilder_PluginInputMutationIsolation
      1 --- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginSuccessExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginTypedSecretField
$ go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' ./pkg/provider/kubernetes/crd   # audit probes
      2         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:150
      1 --- FAIL: TestVerify_Retain_PostCallbackOwnership
      1 --- PASS: TestVerify_C1_Execution
### MUTATION M1 — production: pkg/plugins/middlewareyaegi.go 'WeaklyTypedInput: true' -> false (a resolved Secret string can no longer be decoded into an int field)
$ go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(TypedSecretField|SuccessExecution|NestedExecution|EscapedKeyExecution|InputMutationIsolation)$' ./pkg/provider/kubernetes/crd   # provided fixtures
     24         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:227
      1 --- FAIL: TestEval_ProviderToBuilder_PluginInputMutationIsolation
      1 --- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginSuccessExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginTypedSecretField
$ go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' ./pkg/provider/kubernetes/crd   # audit probes
      1         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/verify_c1_c2_plugin_probe_test.go:172
      1         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:100
      2         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:150
      1 --- FAIL: TestVerify_C1_Execution
      1 --- FAIL: TestVerify_Retain_PostCallbackOwnership
### every provided TestEval_ test of ./pkg/provider/kubernetes/crd: result lines at BASELINE vs under M1
$ go test -count=1 -v -run '^TestEval_' ./pkg/provider/kubernetes/crd   # run at baseline and under M1, '--- PASS/FAIL' lines sorted and diffed
baseline: 65 PASS, 51 FAIL; under M1: 65 PASS, 51 FAIL
diff: IDENTICAL (no provided test changes result when production can no longer decode a string into an int field)
```

`perfect` — provided fixtures and audit probes, baseline then M1 (top-level results, the typed case, and the audit probes):

```console
### BASELINE (unmodified production)
$ go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(TypedSecretField|SuccessExecution|NestedExecution|EscapedKeyExecution|InputMutationIsolation)$' ./pkg/provider/kubernetes/crd   # provided fixtures
      1 --- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation
      1 --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginSuccessExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginTypedSecretField
$ go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' ./pkg/provider/kubernetes/crd   # audit probes
      1 --- PASS: TestVerify_C1_Execution
      1 --- PASS: TestVerify_Retain_PostCallbackOwnership
### MUTATION M1 — production: pkg/plugins/middlewareyaegi.go 'WeaklyTypedInput: true' -> false (a resolved Secret string can no longer be decoded into an int field)
$ go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(TypedSecretField|SuccessExecution|NestedExecution|EscapedKeyExecution|InputMutationIsolation)$' ./pkg/provider/kubernetes/crd   # provided fixtures
      1 --- PASS: TestEval_ProviderToBuilder_PluginEscapedKeyExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginInputMutationIsolation
      1 --- PASS: TestEval_ProviderToBuilder_PluginNestedExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginSuccessExecution
      1 --- PASS: TestEval_ProviderToBuilder_PluginTypedSecretField
$ go test -tags verify -count=1 -v -run '^TestVerify_(C1_Execution|Retain)' ./pkg/provider/kubernetes/crd   # audit probes
      1         	Error Trace:	/home/ubuntu/wt/perfect/pkg/provider/kubernetes/crd/verify_c1_c2_plugin_probe_test.go:172
      1         	Error Trace:	/home/ubuntu/wt/perfect/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:100
      1 --- FAIL: TestVerify_C1_Execution
      1 --- FAIL: TestVerify_Retain_PostCallbackOwnership
### every provided TestEval_ test of ./pkg/provider/kubernetes/crd: result lines at BASELINE vs under M1
$ go test -count=1 -v -run '^TestEval_' ./pkg/provider/kubernetes/crd   # run at baseline and under M1, '--- PASS/FAIL' lines sorted and diffed
baseline: 116 PASS, 0 FAIL; under M1: 116 PASS, 0 FAIL
diff: IDENTICAL (no provided test changes result when production can no longer decode a string into an int field)
```

Observed: under M1 `TestEval_ProviderToBuilder_PluginTypedSecretField` still passes, and every one of the 116 `TestEval_` result lines of the package is identical to baseline, on the claim branch (65 PASS / 51 FAIL both times) and on the control (116 PASS / 0 FAIL both times). The audit probe `TestVerify_C1_Execution`, whose plugin has `Port int` bound to `urn:k8s:secret:vsec:port` (Secret value `8443`), passes at baseline and fails under M1 on both worktrees. No executed provided assertion checks production conversion of a resolved Secret value into an integer field.

### Part "retained configuration coverage"

`fa481ef5`, baseline, provided isolation fixture next to the retaining audit probe (`verify/repro/c1_c5_retain_probe_test.go`, plugin `verify/repro/verifyretain/verifyretain.go` keeps the `*Config` from `CreateConfig` and writes to its nested maps/lists only after the decoder callback / `New` returned):

```console
$ go test -tags verify -count=1 -v -run '^TestVerify_Retain' ./pkg/provider/kubernetes/crd
    verify_c1_c5_retain_probe_test.go:146: RESULT success: handler writes to retained config after New returned: original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=7 fire=14)
    verify_c1_c5_retain_probe_test.go:146: RESULT decode error on unrelated literal field (reference-only replay): original published+runtime configuration unchanged after post-callback writes = false (plugin rewrote leaves: handler=0 fire=28)
    verify_c1_c5_retain_probe_test.go:148:     published AFTER: {"plugin":{"vretain":{"deep":{"inner":{"label":"MUTATED-AFTER-CALLBACK","token":"MUTATED-AFTER-CALLBACK"}},"enabled":"not-a-bool","list":["MUTATED-AFTER-CALLBACK",{"name":"MUTATED-AFTER-CALLBACK","secret":"MUTATED-AFTER-CALLBACK"}],"plain":{"items":["MUTATED-AFTER-CALLBACK"],"nested":{"k":"MUTATED-AFTER-CALLBACK"}},"port":"urn:k8s:secret:vsec:port"}}}
    verify_c1_c5_retain_probe_test.go:146: RESULT constructor error with literal port (reference-only replay): original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=0 fire=14)
    verify_c1_c5_retain_probe_test.go:146: RESULT decode error on Secret-bound int field (reference-only replay): original published+runtime configuration unchanged after post-callback writes = false (plugin rewrote leaves: handler=0 fire=28)
    verify_c1_c5_retain_probe_test.go:148:     published AFTER: {"plugin":{"vretain":{"deep":{"inner":{"label":"MUTATED-AFTER-CALLBACK","token":"MUTATED-AFTER-CALLBACK"}},"list":["MUTATED-AFTER-CALLBACK",{"name":"MUTATED-AFTER-CALLBACK","secret":"MUTATED-AFTER-CALLBACK"}],"plain":{"items":["MUTATED-AFTER-CALLBACK"],"nested":{"k":"MUTATED-AFTER-CALLBACK"}},"port":"urn:k8s:secret:vsec:port"}}}
--- FAIL: TestVerify_Retain_PostCallbackOwnership (0.09s)
    --- PASS: TestVerify_Retain_PostCallbackOwnership/control:_published_map_handed_directly_to_the_plugin_builder (0.07s)
    --- PASS: TestVerify_Retain_PostCallbackOwnership/success:_handler_writes_to_retained_config_after_New_returned (0.00s)
    --- FAIL: TestVerify_Retain_PostCallbackOwnership/decode_error_on_unrelated_literal_field_(reference-only_replay) (0.01s)
    --- PASS: TestVerify_Retain_PostCallbackOwnership/constructor_error_with_literal_port_(reference-only_replay) (0.00s)
    --- FAIL: TestVerify_Retain_PostCallbackOwnership/decode_error_on_Secret-bound_int_field_(reference-only_replay) (0.00s)
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd	0.124s
```

```console
$ sed -n '/### BASELINE/,/### MUTATION M1/p' verify/logs/c5-fa481ef5.log | grep 'Error Trace'
     24         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go:227
      2         	Error Trace:	/home/ubuntu/wt/fa481ef5/pkg/provider/kubernetes/crd/verify_c1_c5_retain_probe_test.go:150
```

```console
$ (cd ~/wt/fa481ef5 && grep -nE '[bB]uilder\.Build\(' pkg/server/middleware/*.go | grep -v _test)
pkg/server/middleware/middlewares.go:421:		plug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)
pkg/server/middleware/plugins.go:76:	if _, refErr := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName); refErr != nil {
```

Control (`perfect`), same retaining probe:

```console
RESULT success: handler writes to retained config after New returned: original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=7 fire=14)
RESULT decode error on unrelated literal field (reference-only replay): original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=0 fire=14)
RESULT constructor error with literal port (reference-only replay): original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=0 fire=14)
RESULT decode error on Secret-bound int field (reference-only replay): original published+runtime configuration unchanged after post-callback writes = true (plugin rewrote leaves: handler=0 fire=14)
--- PASS: TestVerify_Retain_PostCallbackOwnership (0.08s)
```

Observed: on `fa481ef5` the reference-only replay (`plugins.go:76`) decodes the middleware's own plugin map; a plugin that kept its configuration then rewrites the published and runtime configuration after the callback returned (`unchanged = false`, `published AFTER` shows `MUTATED-AFTER-CALLBACK` in place of a Secret reference and of literal values). The provided `PluginInputMutationIsolation` fixture runs on the same branch and fails 8 subtests, but all 24 failing frames are at fixture line 227 (`assert.Contains(t, text, ref, "the redacted error must name the URN reference")`); its two ownership assertions (`assert.JSONEq`, lines 1087/1092) never fail. The provided plugin mutates only inside `New` and keeps nothing after the callback, so no executed provided assertion mutates a nested configuration map retained after callback return.

### Impact reasoning

Both gaps are in the checks, and both hide behavior that matters for the task ("Traefik should only use the value when it builds the middleware"; "Building a middleware must not write the values back into the configuration that the API and other routers read"):

1. Typed decoding. Every Secret value arrives as a string; a plugin field typed `int`, `bool`, `time.Duration`… only works because the production Yaegi decoder is weakly typed. A solution (or a later refactor) that resolves references on a path that loses that conversion would break every Secret-backed numeric plugin field at build time, and the provided suite would stay exactly as green as before (116/116 identical result lines under M1 on the control). Numeric fields bound to Secrets (ports, limits, timeouts) are an ordinary configuration.
2. Retained configuration. The provided ownership check cannot see an alias that is only written through after the callback returned. `fa481ef5` has exactly that defect (replay on the original map) and the provided ownership assertions pass on it; the same defect is present on two of the C1 branches. The trigger in production needs a plugin that keeps the object from `CreateConfig` and a decode error, so the product-side exposure is narrow, but the check-side consequence is that solutions with and without the defensive copy are indistinguishable to the provided tests.

## C6

Claim: parsing a malformed single-entry UsersFile without a trailing newline returns an authentication error that omits the entry position. Branch: `evalon/traefik-se-e9366632`; control: `traefik-secret-references-in-middlewares-perfect`.

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-e9366632 && git worktree add --detach ~/wt/e9366632 FETCH_HEAD   # 49ebf053d
# verify/repro/install.sh <worktree> copies the archived fixtures to the paths the eval provisions them at, plus the audit probes/plugins
```

```sh
for b in e9366632 perfect; do
  cp verify/repro/c6_usersfile_probe_test.go ~/wt/$b/pkg/middlewares/auth/verify_c6_usersfile_probe_test.go
  (cd ~/wt/$b && go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/middlewares/auth) > verify/logs/c6-$b.log 2>&1
  verify/repro/install.sh ~/wt/$b && (cd ~/wt/$b && go test -count=1 -v -run '^TestEval_' ./pkg/middlewares/auth) > verify/logs/c6-fixtures-$b.log 2>&1
done
```

The probe writes a UsersFile to a temp dir and constructs the real middlewares with `NewBasic(ctx, nil, dynamic.BasicAuth{UsersFile: file}, "authName")` / `NewDigest(ctx, nil, dynamic.DigestAuth{UsersFile: file}, "authName")`. It prints the error with the temp path replaced by `<file>` and lists every number left in the message. File contents: `malformedentry_verify_hash` (basic, no colon) and `malformedentry:verify_hash` (digest, two fields), with and without a trailing newline, plus a two-entry control.

### `e9366632`

```console
### branch HEAD: 49ebf053d3e6e8819d11983d0a390dde01285ee6
$ go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/middlewares/auth
    verify_c6_usersfile_probe_test.go:54: RESULT basic: one malformed entry, NO trailing newline: file content "malformedentry_verify_hash" -> error "error parsing BasicUser: invalid user entry file <file>" | names file = true | position numbers outside the path = []
    verify_c6_usersfile_probe_test.go:59:
        	Error Trace:	/home/ubuntu/wt/e9366632/pkg/middlewares/auth/verify_c6_usersfile_probe_test.go:59
        	Error:      	Should NOT be empty, but was []
        	Test:       	TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/basic:_one_malformed_entry,_NO_trailing_newline
        	Messages:   	the diagnostic must identify the entry position
    verify_c6_usersfile_probe_test.go:54: RESULT digest: one malformed entry, NO trailing newline: file content "malformedentry:verify_hash" -> error "error parsing DigestUser: invalid user entry file <file>" | names file = true | position numbers outside the path = []
    verify_c6_usersfile_probe_test.go:59:
        	Error Trace:	/home/ubuntu/wt/e9366632/pkg/middlewares/auth/verify_c6_usersfile_probe_test.go:59
        	Error:      	Should NOT be empty, but was []
        	Test:       	TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/digest:_one_malformed_entry,_NO_trailing_newline
        	Messages:   	the diagnostic must identify the entry position
    verify_c6_usersfile_probe_test.go:54: RESULT control basic: one malformed entry WITH trailing newline: file content "malformedentry_verify_hash\n" -> error "error parsing BasicUser: invalid user entry at line 1 of file <file>" | names file = true | position numbers outside the path = [1]
    verify_c6_usersfile_probe_test.go:54: RESULT control digest: one malformed entry WITH trailing newline: file content "malformedentry:verify_hash\n" -> error "error parsing DigestUser: invalid user entry at line 1 of file <file>" | names file = true | position numbers outside the path = [1]
    verify_c6_usersfile_probe_test.go:54: RESULT control basic: valid entry then malformed entry, no trailing newline: file content "alice:hashalpha\nmalformedentry_verify_hash" -> error "error parsing BasicUser: invalid user entry at line 2 of file <file>" | names file = true | position numbers outside the path = [2]
--- FAIL: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline (0.00s)
    --- FAIL: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/basic:_one_malformed_entry,_NO_trailing_newline (0.00s)
    --- FAIL: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/digest:_one_malformed_entry,_NO_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/control_basic:_one_malformed_entry_WITH_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/control_digest:_one_malformed_entry_WITH_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/control_basic:_valid_entry_then_malformed_entry,_no_trailing_newline (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.022s
FAIL
```

### Control `perfect`

```console
### branch HEAD: 84614aa0ce1af141b9fbdeae2980b79e1f53ba73
$ go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/middlewares/auth
    verify_c6_usersfile_probe_test.go:54: RESULT basic: one malformed entry, NO trailing newline: file content "malformedentry_verify_hash" -> error "parsing user 1 of <file>: expected user:hash format" | names file = true | position numbers outside the path = [1]
    verify_c6_usersfile_probe_test.go:54: RESULT digest: one malformed entry, NO trailing newline: file content "malformedentry:verify_hash" -> error "parsing user 1 of <file>: expected user:realm:hash format" | names file = true | position numbers outside the path = [1]
    verify_c6_usersfile_probe_test.go:54: RESULT control basic: one malformed entry WITH trailing newline: file content "malformedentry_verify_hash\n" -> error "parsing user 1 of <file>: expected user:hash format" | names file = true | position numbers outside the path = [1]
    verify_c6_usersfile_probe_test.go:54: RESULT control digest: one malformed entry WITH trailing newline: file content "malformedentry:verify_hash\n" -> error "parsing user 1 of <file>: expected user:realm:hash format" | names file = true | position numbers outside the path = [1]
    verify_c6_usersfile_probe_test.go:54: RESULT control basic: valid entry then malformed entry, no trailing newline: file content "alice:hashalpha\nmalformedentry_verify_hash" -> error "parsing user 2 of <file>: expected user:hash format" | names file = true | position numbers outside the path = [2]
--- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/basic:_one_malformed_entry,_NO_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/digest:_one_malformed_entry,_NO_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/control_basic:_one_malformed_entry_WITH_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/control_digest:_one_malformed_entry_WITH_trailing_newline (0.00s)
    --- PASS: TestVerify_C6_SingleEntryUsersFileNoTrailingNewline/control_basic:_valid_entry_then_malformed_entry,_no_trailing_newline (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.022s
```

### Production code on `e9366632` (`pkg/middlewares/auth/auth.go`, `parseUsers`)

```go
rawLines := strings.Split(content, "\n")
for i, rawLine := range rawLines {
	...
	userName, userHash, err := parser(line)
	if err != nil {
		if len(rawLines) == 1 {
			return fmt.Errorf("%w: invalid user entry %s", err, source)
		}
		return fmt.Errorf("%w: invalid user entry at line %d of %s", err, i+1, source)
	}
```

### Archived auth fixtures on `e9366632`

```console
### branch HEAD: 49ebf053d3e6e8819d11983d0a390dde01285ee6
$ go test -count=1 -v -run '^TestEval_' ./pkg/middlewares/auth   # provided auth fixtures (eval_auth_sanitation_test.go, byte-exact)
--- PASS: TestEval_BasicAuthParser_OmitCredentialsFromError (0.00s)
    --- PASS: TestEval_BasicAuthParser_OmitCredentialsFromError/extra_field_with_sensitive_hash (0.00s)
    --- PASS: TestEval_BasicAuthParser_OmitCredentialsFromError/single_segment_without_colon (0.00s)
    --- PASS: TestEval_BasicAuthParser_OmitCredentialsFromError/sensitive_prefix_in_first_segment (0.00s)
--- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError (0.00s)
    --- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError/sensitive_prefix_before_first_colon (0.00s)
    --- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError/credential_containing_embedded_URN_substring (0.00s)
    --- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError/missing_hash_segment (0.00s)
    --- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError/four_segments_with_sensitive_values_including_sensitive_first_field (0.00s)
    --- PASS: TestEval_DigestAuthParser_OmitCredentialsFromError/single_segment_without_colon (0.00s)
--- PASS: TestEval_GetUsers_DiagnosticsDistinguishSources (0.00s)
--- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/file (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/inline (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/digest_file (0.00s)
    --- PASS: TestEval_GetUsers_ErrorReportsPositionWithoutLeakingCredentials/digest_inline (0.00s)
PASS
ok  	github.com/traefik/traefik/v3/pkg/middlewares/auth	0.024s
```

```console
$ grep -n 'os.WriteFile(htFile' ~/evalfx/tests/eval_auth_sanitation_test.go
226:			err := os.WriteFile(htFile, []byte(strings.Join(users, "\n")+"\n"), 0o600)
275:			err := os.WriteFile(htFile, []byte(strings.Join(users, "\n")+"\n"), 0o600)
319:	err := os.WriteFile(htFile, []byte("baduser_without_colon_secret\n"), 0o600)
```

### Summary of what was observed and impact reasoning

On `e9366632`, a UsersFile holding one malformed entry and no trailing newline makes BasicAuth and DigestAuth construction fail with `error parsing BasicUser: invalid user entry file <file>` / `error parsing DigestUser: invalid user entry file <file>`: the file is named, no entry or line number appears (`position numbers outside the path = []`). The same content with a trailing newline reports `at line 1 of file <file>`, a two-entry file without trailing newline reports `at line 2`, and the control reports `parsing user 1 of <file>` in every case. The cause is the `len(rawLines) == 1` branch above. The credential text is not disclosed in any variant.

Impact: the task requires that "auth errors should still say which user entry is invalid". A one-line htpasswd file written without a final newline (`printf 'user:hash' > file`, `echo -n`, many Secret/ConfigMap-mounted files) is an everyday input. The operator still gets the file path and there is only one entry, so the entry can be found; the defect is an inconsistent, position-less diagnostic rather than lost information, and it also reads oddly ("invalid user entry file …"). The archived auth fixtures all pass on this branch because they always end the file with a newline, so the eval's checks do not observe it. Workaround: add a trailing newline. Fix: drop the `len(rawLines) == 1` special case.

## C7

Claim: the added or modified auth-map tests cannot detect incorrectly ordered production output because they normalize actual results before comparison and contain no assertion on the original order. Branch: `evalon/traefik-se-3efc4f96`; control: `traefik-secret-references-in-middlewares-perfect`.

### Setup and commands

```sh
# one-time: fixtures (byte-exact) and the claim-branch remote
mkdir -p ~/evalfx && unzip -o eval_tests.zip -d ~/evalfx        # sha256 fd25bf4a6a70d813f0568590af43976508548ac36a05177a4b3d24771cd55a08, 13 files under tests/
cd ~/repos/traefik && git remote add evals https://github.com/kaitranntt-evals/traefik-secret-references-in-middlewares
git worktree add --detach ~/wt/perfect origin/traefik-secret-references-in-middlewares-perfect   # control, 84614aa0c
git fetch evals evalon/traefik-se-3efc4f96 && git worktree add --detach ~/wt/3efc4f96 FETCH_HEAD   # 6e6f6614d
# verify/repro/install.sh <worktree> copies the archived fixtures to the paths the eval provisions them at, plus the audit probes/plugins
```

```sh
verify/repro/install.sh ~/wt/3efc4f96 && verify/repro/c7_probe.sh ~/wt/3efc4f96 > verify/logs/c7-3efc4f96.log 2>&1
for b in 3efc4f96 perfect; do   # archived deterministic-order fixture, 30 runs
  (cd ~/wt/$b && echo "### HEAD: $(git rev-parse HEAD)" && go test -count=30 -v -run '^TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation$' ./pkg/provider/kubernetes/ingress-nginx 2>&1 | grep -E '^--- |Messages:|^(ok|FAIL|PASS)' | sed -E 's/ \([0-9.]+s\)//' | sort | uniq -c) > verify/logs/c7-determinism-$b.log
done
```

`c7_probe.sh` prints the ordering-related lines of the branch's added test file, runs the branch's own added/modified tests and the unsorted-order audit probe (`verify/repro/c7_order_probe_test.go`: calls production `basicAuthUsers` 200 times on a three-user `auth-map` Secret and compares the returned slice without sorting), then repeats everything under mutation O1 and reverts it. Result lines are passed through `sort | uniq -c` (count prefix).

### Assertion text and baseline

```console
### branch HEAD: 6e6f6614d8ed45c143f5814472c5e84267273b02
### ordering-related lines of the added auth-map test (pkg/provider/kubernetes/ingress-nginx/build_test.go)
70:			users, secrets, err := basicAuthUsers(secret, test.authSecretType)
77:			slices.Sort(users)
78:			assert.Equal(t, test.expectedUsers, users)
79:			assert.Equal(t, test.expectedSecrets, secrets)
98:			slices.Sort(expectedUsers)
99:			assert.Equal(t, dynamic.Users(expectedUsers), resolved.BasicAuth.Users)
### every auth-map mention in the package's test files
pkg/provider/kubernetes/ingress-nginx/build_test.go:41:			desc:           "auth-map references the password of each user",
pkg/provider/kubernetes/ingress-nginx/build_test.go:42:			authSecretType: "auth-map",
pkg/provider/kubernetes/ingress-nginx/build_test.go:54:			desc:           "auth-map without users",
pkg/provider/kubernetes/ingress-nginx/build_test.go:55:			authSecretType: "auth-map",
pkg/provider/kubernetes/ingress-nginx/build_test.go:92:				if test.authSecretType == "auth-map" {

### BASELINE (unmodified production)
$ go test -count=5 -run '^(TestBasicAuthUsers|TestLoadIngresses)$' -v ./pkg/provider/kubernetes/ingress-nginx   # branch's own added/modified tests, eval fixtures excluded
      5     --- PASS: TestBasicAuthUsers/auth-file_references_the_auth_key
      5     --- PASS: TestBasicAuthUsers/auth-file_without_auth_key
      5     --- PASS: TestBasicAuthUsers/auth-map_references_the_password_of_each_user
      5     --- PASS: TestBasicAuthUsers/auth-map_without_users
      5 --- PASS: TestBasicAuthUsers
      5 --- PASS: TestLoadIngresses
      1 PASS
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.432s
$ go test -count=1 ./pkg/provider/kubernetes/ingress-nginx   # whole package, branch's own tests + archived eval fixtures
      1 --- FAIL: TestEval_IngressNGINX_AuthMapCredentialBoundaries (0.01s)
      1 --- FAIL: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation (0.03s)
      2 FAIL
      1 FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.394s
$ go test -tags verify -count=1 -v -run '^TestVerify_C7' ./pkg/provider/kubernetes/ingress-nginx   # audit probe: order compared without sorting
    verify_c7_order_probe_test.go:47: RESULT order seen 137/200: alice:urn:k8s:secret:basic-auth:alice | bob:urn:k8s:secret:basic-auth:bob | carol:urn:k8s:secret:basic-auth:carol
    verify_c7_order_probe_test.go:47: RESULT order seen  36/200: bob:urn:k8s:secret:basic-auth:bob | carol:urn:k8s:secret:basic-auth:carol | alice:urn:k8s:secret:basic-auth:alice
    verify_c7_order_probe_test.go:47: RESULT order seen  27/200: carol:urn:k8s:secret:basic-auth:carol | alice:urn:k8s:secret:basic-auth:alice | bob:urn:k8s:secret:basic-auth:bob
    verify_c7_order_probe_test.go:49: RESULT distinct orders over 200 calls = 3; calls returning sorted order = 137/200
        	Error:      	"map[alice:urn:k8s:secret:basic-auth:alice | bob:urn:k8s:secret:basic-auth:bob | carol:urn:k8s:secret:basic-auth:carol:137 bob:urn:k8s:secret:basic-auth:bob | carol:urn:k8s:secret:basic-auth:carol | alice:urn:k8s:secret:basic-auth:alice:36 carol:urn:k8s:secret:basic-auth:carol | alice:urn:k8s:secret:basic-auth:alice | bob:urn:k8s:secret:basic-auth:bob:27]" should have 1 item(s), but has 3
        	Messages:   	the same Secret must always publish the same user order
        	Error:      	Not equal:
        	Messages:   	the published user order must be the sorted one
--- FAIL: TestVerify_C7_AuthMapOrder (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.024s
FAIL

```

`pkg/provider/kubernetes/ingress-nginx/build_test.go` is new on this branch (102 added lines, one test function `TestBasicAuthUsers`; the claim's `Test_basicAuthUsers` / `secretUser` names do not exist there). Line 70 takes the production result, line 77 sorts it (`slices.Sort(users)`), line 78 compares it. The second ordering-related assertion (lines 92-99) resolves the references and compares against a sorted *expected* slice; under O1 it stays green as well (see below). No other `auth-map` mention exists in the package's own test files.

Production on this branch builds the slice by ranging over the Secret's data map (`pkg/provider/kubernetes/ingress-nginx/build.go`):

```go
for user, pass := range secret.Data {
	urn := dynamic.SecretURN(secret.Name, user)
	secrets[urn] = string(pass)
	users = append(users, user+":"+urn)
}

return users, secrets, nil
```

### Mutation O1 — production returns the same entries in reverse-sorted order

```console
### MUTATION O1 — production: basicAuthUsers returns the auth-map entries in REVERSE-sorted order (same contents)
+		slices.Sort(users)
+		slices.Reverse(users)
$ go test -count=5 -run '^(TestBasicAuthUsers|TestLoadIngresses)$' -v ./pkg/provider/kubernetes/ingress-nginx   # branch's own added/modified tests, eval fixtures excluded
      5     --- PASS: TestBasicAuthUsers/auth-file_references_the_auth_key
      5     --- PASS: TestBasicAuthUsers/auth-file_without_auth_key
      5     --- PASS: TestBasicAuthUsers/auth-map_references_the_password_of_each_user
      5     --- PASS: TestBasicAuthUsers/auth-map_without_users
      5 --- PASS: TestBasicAuthUsers
      5 --- PASS: TestLoadIngresses
      1 PASS
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.390s
$ go test -count=1 ./pkg/provider/kubernetes/ingress-nginx   # whole package, branch's own tests + archived eval fixtures
      1 --- FAIL: TestEval_IngressNGINX_AuthMapCredentialBoundaries (0.01s)
      2 FAIL
      1 FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.397s
$ go test -tags verify -count=1 -v -run '^TestVerify_C7' ./pkg/provider/kubernetes/ingress-nginx   # audit probe: order compared without sorting
    verify_c7_order_probe_test.go:47: RESULT order seen 200/200: carol:urn:k8s:secret:basic-auth:carol | bob:urn:k8s:secret:basic-auth:bob | alice:urn:k8s:secret:basic-auth:alice
    verify_c7_order_probe_test.go:49: RESULT distinct orders over 200 calls = 1; calls returning sorted order = 0/200
        	Error:      	Not equal:
        	Messages:   	the published user order must be the sorted one
--- FAIL: TestVerify_C7_AuthMapOrder (0.00s)
FAIL
FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.025s
FAIL

```

### Archived deterministic-order fixture, 30 runs

```console
### HEAD: 6e6f6614d8ed45c143f5814472c5e84267273b02
$ go test -count=30 -v -run '^TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation$' ./pkg/provider/kubernetes/ingress-nginx   # provided fixture, 30 runs; result and assertion-message lines counted
     55         	Messages:   	auth-map reload on unchanged secret must produce identical configuration hash
     55         	Messages:   	auth-map reload on unchanged secret must produce identical slice order
     21         	Messages:   	rotation must not change public Secret references
     55         	Messages:   	unchanged reload must be deep-equal so the watcher skips it
     29 --- FAIL: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation
      1 --- PASS: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation
      2 FAIL
      1 FAIL	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.848s
### HEAD: 84614aa0ce1af141b9fbdeae2980b79e1f53ba73
$ go test -count=30 -v -run '^TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation$' ./pkg/provider/kubernetes/ingress-nginx   # provided fixture, 30 runs; result and assertion-message lines counted
     30 --- PASS: TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation
      1 PASS
      1 ok  	github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx	0.854s
```

### Summary of what was observed and impact reasoning

- The only added auth-map test sorts the actual production result before `assert.Equal` (`build_test.go:77-78`); nothing asserts the order production returned.
- At baseline the unsorted probe saw 3 distinct orders in 200 calls on the same Secret (sorted order in 137/200), i.e. production order follows Go map iteration.
- With production forced to return a wrong (reverse-sorted) order for every call (`0/200` sorted), the branch's own `TestBasicAuthUsers` (4 subtests) and `TestLoadIngresses` pass 5/5 runs, identical to baseline. The permutation is invisible to the branch's tests.
- The archived fixture `TestEval_IngressNGINX_AuthMapDeterministicOrderAndRotation` fails 29 of 30 runs on the branch (`auth-map reload on unchanged secret must produce identical slice order` / `identical configuration hash` / `unchanged reload must be deep-equal so the watcher skips it`) and passes 30 of 30 on the control. (Under O1 that fixture passes, because a reversed order is still a stable order; the other failing fixture, `AuthMapCredentialBoundaries`, fails at baseline and under O1 alike.)

Impact: the task states "an unchanged Secret should not produce a different configuration". On this branch an `auth-secret` of type `auth-map` with more than one user yields a differently ordered `basicAuth.users` list from one provider load to the next, so the configuration hash changes and the watcher republishes a configuration although nothing changed (router/middleware rebuilds on every resync). Multi-user `auth-map` Secrets are the ordinary use of that annotation. Authentication itself keeps working, which is why the state looks correct; the branch's test suite is green because the one test that could have caught it normalizes the order first. Fix: sort the keys in `basicAuthUsers` (as the control does) and assert the unsorted production slice in `TestBasicAuthUsers`.
