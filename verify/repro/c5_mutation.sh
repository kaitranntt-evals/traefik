#!/bin/bash
# Run: verify/repro/c5_mutation.sh <worktree-7af818be>   (baseline, then SecretStore.Digest made value-independent, then restore)
W=$1; F=$W/pkg/config/dynamic/secrets.go
cp $(dirname $0)/c5_reload_probe_test.go $W/pkg/provider/kubernetes/crd/
t() { (cd $W && go test -tags verify -count=1 -v -run 'TestVerify_C5|TestLoadConfigurationFromCRD_secretUpdate|TestLoadConfigurationFromCRD_authSecretUpdate|TestLoadConfiguration_authSecretUpdate|TestSecretStore_Digest' ./pkg/config/dynamic/ ./pkg/provider/kubernetes/crd/ ./pkg/provider/kubernetes/ingress-nginx/ 2>&1 | grep -E '^(--- |ok|FAIL)|c5_reload_probe' ); }
echo "=== baseline"; t
cp $F /tmp/c5.bak
python3 - $F <<'PY'
import sys; p=sys.argv[1]; s=open(p).read(); o=s
s=s.replace('fmt.Fprintf(mac, "%d:%s%d:%s", len(urn), urn, len(values[urn]), values[urn])','fmt.Fprintf(mac, "%d:%s", len(urn), urn)')
assert s!=o; open(p,'w').write(s)
PY
echo "=== mutation: digest no longer depends on the Secret values"; t
cp /tmp/c5.bak $F; rm $W/pkg/provider/kubernetes/crd/c5_reload_probe_test.go
