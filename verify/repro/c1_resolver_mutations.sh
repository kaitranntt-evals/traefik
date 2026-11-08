#!/bin/bash
# Run: ./verify/repro/c1_resolver_mutations.sh <3a48b648|3a9c96a8>  (from a checkout of evalon/traefik-se-<hash>; applies four single mutations to pkg/config/dynamic/secrets.go — drop a non-secret ForwardAuth field, leave a ForwardAuth Secret unresolved, blank non-secret plugin strings, leave nested plugin Secrets unresolved — and shows the branch's Secret tests catch each one)
set -u
b=${1:?branch hash}
F=pkg/config/dynamic/secrets.go
run() { go test -count=1 -run 'Secret' ./pkg/config/dynamic ./pkg/server/middleware 2>&1 | grep -E '^(ok|FAIL|--- FAIL|\s+Error Trace|\s+Test:)' | head -20; }
mut() { cp "$F" /tmp/c1_sec.bak; sed -i "$2" "$F"; echo "--- mutation: $1"; diff /tmp/c1_sec.bak "$F" | grep '^[<>]'; run; cp /tmp/c1_sec.bak "$F"; }
echo "=== BASELINE"; run
case "$b" in
  3a48b648)
    mut "M1 drop non-secret ForwardAuth.Address in resolved copy" 's|tls := config.ForwardAuth.TLS|config.ForwardAuth.Address = ""; tls := config.ForwardAuth.TLS|'
    mut "M2 leave ForwardAuth.TLS.Key unresolved" 's|tls.Key = m.Secrets.resolveString(tls.Key)|tls.Key = tls.Key|'
    mut "M3 blank non-secret plugin strings" 's|return s.resolveString(value)|if _, ok := s[value]; !ok { return "" }; return s.resolveString(value)|'
    mut "M4 leave nested plugin map values unresolved" 's|value\[key\] = s.resolvePlugin(v)|value[key] = v|' ;;
  3a9c96a8)
    mut "M1 drop non-secret ForwardAuth.Address in resolved copy" 's|config.ForwardAuth.TLS.CA = resolve(config.ForwardAuth.TLS.CA)|config.ForwardAuth.Address = ""; config.ForwardAuth.TLS.CA = resolve(config.ForwardAuth.TLS.CA)|'
    mut "M2 leave ForwardAuth.TLS.Key unresolved" 's|config.ForwardAuth.TLS.Key = resolve(config.ForwardAuth.TLS.Key)|config.ForwardAuth.TLS.Key = config.ForwardAuth.TLS.Key|'
    mut "M3 blank non-secret plugin strings" 's|\t\treturn resolve(value)$|\t\tif resolved := resolve(value); resolved == value { return "" } else { return resolved }|'
    mut "M4 leave nested plugin map values unresolved" 's|value\[key\] = resolvePluginSecrets(v, resolve)|value[key] = v|' ;;
  *) echo "unknown branch $b"; exit 2 ;;
esac
