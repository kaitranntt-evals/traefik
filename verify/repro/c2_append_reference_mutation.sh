#!/bin/bash
# Run: ./verify/repro/c2_append_reference_mutation.sh <af980b0a|1d04f11a|8624f8f2|413d5677>  (from a checkout of evalon/traefik-se-<hash>; appends the URN to the branch's generic value-caused plugin error and re-runs the branch's own plugin-error test — a test that REQUIRES the URN to be absent turns red)
set -u
b=${1:?branch hash}
case "$b" in
  af980b0a) F=pkg/server/middleware/middlewares.go; S='s|errors.New("invalid configuration: the plugin rejects the values of the referenced secrets")|errors.New("invalid configuration: the plugin rejects the values of the referenced secrets urn:k8s:secret:creds:token")|'; T=TestBuilder_buildConstructor_pluginSecrets ;;
  1d04f11a) F=pkg/server/middleware/middlewares.go; S='s|fmt.Errorf("invalid value read from a secret in the configuration of plugin %s", pluginType)|fmt.Errorf("invalid value read from a secret (urn:k8s:secret:name:token) in the configuration of plugin %s", pluginType)|'; T=TestBuilder_BuildChainPluginSecrets ;;
  8624f8f2) F=pkg/server/middleware/plugins.go; S='s|errors.New("invalid configuration: a value read from a secret is rejected")|errors.New("invalid configuration: a value read from a secret (urn:k8s:secret:plugin:token) is rejected")|'; T=TestBuilder_BuildChain_pluginErrorShowsSecretReference ;;
  413d5677) F=pkg/server/middleware/secrets.go; S='s|errors.New("failed to build the plugin with the referenced secret values")|errors.New("failed to build the plugin with the referenced secret values urn:k8s:secret:name:key, urn:k8s:secret:name:token")|'; T=TestBuilder_PluginSecretReferenceError ;;
  *) echo "unknown branch $b"; exit 2 ;;
esac
cp "$F" "/tmp/c2_$b.bak"
sed -i "$S" "$F"
echo "=== [$b] mutation diff:"; diff "/tmp/c2_$b.bak" "$F" | grep '^[<>]'
echo "=== [$b] $T after mutation:"
go test -count=1 -run "$T" -v ./pkg/server/middleware 2>&1 | grep -E '^(ok|FAIL|--- |\s+Error:|\s+expected|\s+actual)'
cp "/tmp/c2_$b.bak" "$F"
