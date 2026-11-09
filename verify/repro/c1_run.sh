#!/usr/bin/env bash
# Run: bash verify/repro/c1_run.sh <worktree of evalon/traefik-ga-c8563c30|b52f8b88|12fa8560>  (replays the dispatch probe and every exchange against the authored tests; restores the worktree afterwards)
set -u
WT=$(cd "$1" && pwd)
HERE=$(cd "$(dirname "$0")" && pwd)
PKG=pkg/provider/kubernetes/gateway
AUTHORED='^TestLoad(Routes|HTTPRoutes|GRPCRoutes)_(listenerIsolation|httpsListenerIsolation)$'
cd "$WT" || exit 1

restore() { git checkout -q -- "$PKG" && rm -f "$PKG/c1_route_identity_probe_test.go"; }
trap restore EXIT

# Keeps the verdict lines of go test -v plus the testify failure blocks, drops the provider logs.
filter() { grep -E '^\s*(=== RUN|--- (PASS|FAIL)|PASS$|FAIL|ok\s)|VERIFY hook|Error Trace|Error:|elements differ|extra elements|listA|listB|Not equal|expected:|actual  :|^\s+[-+]\s+\(string\)|^\s+[-+](Rule|\s+Rule)|^\s+[-+] *\(gateway\.routerSummary\)|^\s+\(string\) \(len|\(\[\]string\) \(len=|\(gateway.routerSummary\)|Rule: \(string\)' | cut -c1-330; }

authored() { go test -count=1 -v "$@" -run "$AUTHORED" ./$PKG 2>&1 | filter; }

# swap <fixture> <parents|rules> <listenerA> <listenerB>: exchanges, between the routes attached to the two listeners,
# either the parent binding (parentRefs sectionName) or the complete spec.rules block (matches + backendRefs).
swap() {
python3 - "$PKG/fixtures/$1" "$2" "$3" "$4" <<'PY'
import re, sys
path, mode, a, b = sys.argv[1:]
docs = re.split(r'(?m)^---[ \t]*\n', open(path).read())
def find(listener):
    c = [i for i, d in enumerate(docs) if re.search(r'kind: (HTTP|GRPC)Route', d) and re.search(r'sectionName: %s\s*\n' % re.escape(listener), d)]
    if len(c) > 1:
        c = [i for i in c if re.search(r'name: attache[sd]-to-%s\s*\n' % re.escape(listener), docs[i])]
    assert len(c) == 1, (listener, c)
    return c[0]
ia, ib = find(a), find(b)
name = lambda i: re.search(r'metadata:\s*\n\s+name: (\S+)', docs[i]).group(1)
if mode == 'parents':
    docs[ia] = re.sub(r'sectionName: %s(\s*\n)' % re.escape(a), r'sectionName: %s\1' % b, docs[ia])
    docs[ib] = re.sub(r'sectionName: %s(\s*\n)' % re.escape(b), r'sectionName: %s\1' % a, docs[ib])
else:
    cut = lambda i: re.search(r'(?m)^  rules:', docs[i]).start()
    ta, tb = docs[ia][cut(ia):], docs[ib][cut(ib):]
    docs[ia], docs[ib] = docs[ia][:cut(ia)] + tb, docs[ib][:cut(ib)] + ta
open(path, 'w').write('---\n'.join(docs))
print('FIXTURE-EXCHANGE %s: %s between route %s (listener %s) and route %s (listener %s)' % (path, mode, name(ia), a, name(ib), b))
PY
}

echo "##### branch HEAD $(git rev-parse HEAD)"
echo "##### 1. authored tests, untouched"
authored

echo "##### 2. dispatch probe on the configurations the authored tests load"
cp "$HERE/c1_route_identity_probe_test.go" $PKG/
go test -tags verify -count=1 -v -run 'TestVerifyC1' ./$PKG 2>&1 | grep -E '^(CONFIG|ROUTER|DISPATCH|---|ok|FAIL|PASS)' | cut -c1-420

echo "##### 3. authored tests after exchanging rule/priority/TLS between the two router objects in the loaded configuration (hook injected after loadConfigurationFromGateways)"
python3 - $PKG/kubernetes_test.go <<'PY'
import re, sys
p = sys.argv[1]
lines = open(p).read().split('\n')
out, infn, n = [], False, 0
for i, line in enumerate(lines):
    if line.startswith('func '):
        infn = re.match(r'func TestLoad(Routes|HTTPRoutes|GRPCRoutes)_(listenerIsolation|httpsListenerIsolation)\(', line) is not None
    out.append(line)
    if infn and 'require.NoError(t, err)' in line and 'p.loadConfigurationFromGateways(t.Context())' in lines[i - 1]:
        out.append(line[:len(line) - len(line.lstrip())] + 'verifyC1Hook(t, conf)')
        n += 1
open(p, 'w').write('\n'.join(out))
print('HOOK-INJECTED into %d authored test bodies' % n)
PY
VERIFY_C1_SWAP_CONF='empty-hostname,abc-foo-example-com;empty-hostname,wildcard-example-com' authored -tags verify
git checkout -q -- $PKG/kubernetes_test.go
rm -f $PKG/c1_route_identity_probe_test.go

HTTPS_FIXTURE=$(cd $PKG/fixtures && ls httproute/with_https_listeners_misdirected_requests.yml httproute/with_https_listener_isolation.yml 2>/dev/null | head -1)
n=4
for spec in \
  "httproute/with_listener_isolation.yml parents empty-hostname abc-foo-example-com" \
  "httproute/with_listener_isolation.yml rules empty-hostname abc-foo-example-com" \
  "httproute/with_listener_isolation.yml parents empty-hostname wildcard-example-com" \
  "grpcroute/with_listener_isolation.yml parents empty-hostname wildcard-example-com" \
  "$HTTPS_FIXTURE parents https-with-wildcard-hostname https-with-hostname-matching-wildcard"; do
  echo "##### $n. authored tests, untouched test code, after a fixture exchange"
  # shellcheck disable=SC2086
  swap $spec && authored
  git checkout -q -- $PKG/fixtures
  n=$((n + 1))
done
echo "##### worktree after restore: $(git status --short | wc -l) modified files"
