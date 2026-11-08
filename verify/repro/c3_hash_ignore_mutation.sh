#!/bin/bash
# Run: cp verify/repro/c3_secret_rotation_hash_probe_test.go pkg/provider/kubernetes/crd/verify_probe_c3_test.go && ./verify/repro/c3_hash_ignore_mutation.sh  (from a checkout of evalon/traefik-se-c89bfec6 or evalon/traefik-se-2839e93a; excludes Middleware.Secrets from hashstructure so Secret rotation no longer changes the provider's change-detection hash, then shows the branch's Secret-change tests stay green while the probe goes red)
set -u
F=pkg/config/dynamic/middlewares.go
cp "$F" /tmp/c3_mw.bak
TESTS='TestLoadConfigurationFromCRD_SecretValues$|secretValuesAreHidden|TestVerifyC3_'
echo "=== BASELINE"
go test -tags verify -count=1 -run "$TESTS" -v ./pkg/provider/kubernetes/crd 2>&1 | grep -E '^(--- |ok|FAIL|\s+verify_probe)'
sed -i 's|Secrets Secrets `json:"-" toml:"-" yaml:"-" label:"-" file:"-" kv:"-"`|Secrets Secrets `json:"-" toml:"-" yaml:"-" label:"-" file:"-" kv:"-" hash:"ignore"`|' "$F"
echo "=== MUTATED (diff):"; diff /tmp/c3_mw.bak "$F" | grep '^[<>]'
go test -tags verify -count=1 -run "$TESTS" -v ./pkg/provider/kubernetes/crd 2>&1 | grep -E '^(--- |ok|FAIL|\s+verify_probe)'
cp /tmp/c3_mw.bak "$F"
