#!/usr/bin/env bash
# Run: verify/repro/c3_mutations.sh <worktree of evalon/traefik-se-6205ead6> [dir holding the eval fixtures] — mutates production code so stored diagnostics are missing or leak a Secret value, runs the branch's plugin Secret-reference test each time, then restores the files.
set -uo pipefail

WT="${1:?worktree of evalon/traefik-se-6205ead6}"
FIXTURES="${2:-}"
cd "$WT"

MW=pkg/server/middleware/middlewares.go
TEST=pkg/server/middleware/secrets_test.go
cp "$MW" /tmp/c3_middlewares.go.orig
cp "$TEST" /tmp/c3_secrets_test.go.orig
restore() { cp /tmp/c3_middlewares.go.orig "$MW"; cp /tmp/c3_secrets_test.go.orig "$TEST"; }
trap restore EXIT

run_branch_test() {
	go test -count=1 -v -run '^TestBuilder_buildConstructor_pluginSecretReference$' ./pkg/server/middleware 2>&1 |
		grep -E '^(\s+secrets_test.go:[0-9]+: C3|\s*--- |PASS|FAIL|ok)' | sed 's/^/    /'
}

run_fixtures() {
	[ -n "$FIXTURES" ] || return 0
	cp "$FIXTURES"/eval_crd_behavior_test.go "$FIXTURES"/export_eval_test.go "$FIXTURES"/eval_provider_to_builder_runtime_test.go pkg/provider/kubernetes/crd/
	go test -count=1 -v -run '^TestEval_ProviderToBuilder_Plugin(BuildErrorRedaction|CachedErrorRedaction)$' ./pkg/provider/kubernetes/crd 2>&1 |
		grep -E '^(--- |PASS|FAIL|ok)' | sed 's/^/    [eval fixture] /'
	rm -f pkg/provider/kubernetes/crd/eval_crd_behavior_test.go pkg/provider/kubernetes/crd/export_eval_test.go pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go
}

# Instrumentation only: print the Err slice right before the stored-diagnostic assertion of the branch's test.
python3 - "$TEST" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
old = '\t\t\tassert.NotContains(t, rtConf.Middlewares["plugin"].Err, test.secret)\n'
assert s.count(old) == 1
open(p, 'w').write(s.replace(old, '\t\t\tt.Logf("C3 Err inspected by the assertion: %q", rtConf.Middlewares["plugin"].Err)\n' + old))
PY

echo "== M0: unmodified production code (test instrumented with one t.Logf)"
run_branch_test
run_fixtures

echo "== M1: buildConstructor records the plugin failure with the resolved (secret) configuration in MiddlewareInfo.Err"
python3 - "$MW" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
old = '''		plug, err := buildPlugin(b.pluginBuilder, config.Middleware, middlewareName)
		if err != nil {
'''
assert s.count(old) == 1
open(p, 'w').write(s.replace(old, old + '\t\t\tconfig.AddError(fmt.Errorf("plugin: rejected configuration %v", resolved.Plugin), true)\n'))
PY
git --no-pager diff --stat -- "$MW" | sed 's/^/    /'
run_branch_test
run_fixtures
cp /tmp/c3_middlewares.go.orig "$MW"

echo "== M2: BuildMiddlewareChain no longer records any error (AddError calls removed)"
python3 - "$MW" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
old = '\t\t\t\tb.configs[middlewareName].AddError(err, true)\n'
assert s.count(old) == 3
open(p, 'w').write(s.replace(old, ''))
PY
git --no-pager diff --stat -- "$MW" | sed 's/^/    /'
run_branch_test
run_fixtures
cp /tmp/c3_middlewares.go.orig "$MW"

echo "== M3 (sensitivity control): buildConstructor records the bare Secret value as an Err entry"
python3 - "$MW" <<'PY'
import sys
p = sys.argv[1]
s = open(p).read()
old = '''		plug, err := buildPlugin(b.pluginBuilder, config.Middleware, middlewareName)
		if err != nil {
'''
assert s.count(old) == 1
new = old + '''			for _, value := range config.Secrets {
				config.AddError(errors.New(value), true)
			}
'''
open(p, 'w').write(s.replace(old, new))
PY
git --no-pager diff --stat -- "$MW" | sed 's/^/    /'
run_branch_test

restore
echo "== restored: $(git status --short -- "$MW" "$TEST" | wc -l) modified tracked files left"
