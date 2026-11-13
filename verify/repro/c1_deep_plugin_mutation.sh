#!/bin/bash
# Run: WT=<worktree of evalon/traefik-se-a2edef14> bash verify/repro/c1_deep_plugin_mutation.sh
# Corrupts every Secret value bound for a *nested* plugin field (map/list depth > 0) in the CRD provider,
# then runs the branch's committed provider tests. Flat plugin fields are left intact.
set -u
HERE="$(cd "$(dirname "$0")" && pwd)"
cd "${WT:?set WT}"
F=pkg/provider/kubernetes/crd/kubernetes.go
P=pkg/provider/kubernetes/crd/zz_verify_c1_probe_test.go
cp "$HERE/c1_probe_test.go" $P
echo "--- bound values, unmodified branch ---"
go test -tags verify -count=1 -run TestVerifyC1 -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(BOUND|ok|FAIL)'
cp $F /tmp/c1_kubernetes.go.bak
trap 'cp /tmp/c1_kubernetes.go.bak $F; rm -f $P' EXIT
python3 - "$F" <<'PY'
import sys
p=sys.argv[1]; s=open(p).read()
a='''		parts := strings.Split(iv, ":")
		return secrets.Set(parts[3], parts[4], value), nil
'''
assert a in s
s=s.replace(a,'''		parts := strings.Split(iv, ":")
		if verifyNested > 0 {
			value = "CORRUPTED-BY-VERIFY"
		}
		return secrets.Set(parts[3], parts[4], value), nil
''')
a='''	case []any:
		for i := range iv {'''
assert a in s
s=s.replace(a,'''	case []any:
		verifyNested++
		defer func() { verifyNested-- }()
		for i := range iv {''')
a='''	case map[string]any:
		for k := range iv {
			if iv[k], err = loadSecretKeys('''
assert a in s
s=s.replace(a,'''	case map[string]any:
		verifyNested++
		defer func() { verifyNested-- }()
		for k := range iv {
			if iv[k], err = loadSecretKeys(''')
s+="\nvar verifyNested int\n"
open(p,'w').write(s)
PY
git diff --stat -- $F
echo "--- bound values, mutated ---"
go test -tags verify -count=1 -run TestVerifyC1 -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(BOUND|ok|FAIL)'
echo "--- committed tests, mutated ---"
go test -count=1 -timeout=10m ./pkg/config/dynamic/... ./pkg/server/middleware/... ./pkg/provider/kubernetes/crd/ ./pkg/provider/kubernetes/ingress-nginx/ 2>&1 | tail -20
echo "--- targeted (mutated) ---"
go test -count=1 -run 'TestLoadIngressRoutes/.*(Secret|secret|plugin|Plugin).*|TestKubernetesSecretReferencesAndReload' -v ./pkg/provider/kubernetes/crd/ 2>&1 | grep -E '^(=== RUN|\s*--- (PASS|FAIL)).*(ecret|lugin)|^(ok|FAIL|PASS)' | grep -v '=== RUN'
