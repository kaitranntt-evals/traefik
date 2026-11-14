#!/bin/bash
# Run: bash verify/bridge/install.sh <worktree> <dir with extracted eval_tests.zip tests/> [bridge]  -- provisions the eval fixtures byte-exact; with "bridge", also routes their loader helpers through the publication steps of Provider.Provide.
set -e
W=$1; T=$2; C=$W/pkg/provider/kubernetes/crd; N=$W/pkg/provider/kubernetes/ingress-nginx; R=$(cd "$(dirname "$0")" && pwd)
for f in eval_crd_behavior_test.go export_eval_test.go eval_provider_to_builder_runtime_test.go eval_redis_propagation_test.go eval_namespace_isolation_test.go eval_deepcopy_isolation_test.go eval_lifecycle_binding_test.go; do cp $T/$f $C/; done
cp $T/eval_ingress_nginx_test.go $T/export_eval_ingress_nginx_test.go $N/
for p in evalplugin:eval_local_plugin evalretain:eval_retain_plugin; do d=$C/plugins-local/src/${p%%:*}; mkdir -p $d; cp $T/${p##*:}.go $d/${p%%:*}.go; cp $T/${p##*:}_manifest.yml $d/.traefik.yml; done
mkdir -p $C/plugins-local/src/evalwasm; cp $T/eval_wasm_plugin_manifest.yml $C/plugins-local/src/evalwasm/.traefik.yml
cp $T/eval_auth_sanitation_test.go $W/pkg/middlewares/auth/
cp $T/eval_logconfiguration_test.go $W/pkg/server/
if [ "${3:-}" = bridge ]; then
  cp $R/bridge_crd_test.go $C/; cp $R/bridge_nginx_test.go $N/
  sed -i -E 's/p\.loadConfigurationFromCRD\(t\.Context\(\), (client|rotatingClient)\)/evalPublishLikeProvide(t.Context(), \&p, \1)/' $C/export_eval_test.go
  sed -i 's/p\.loadConfiguration(t\.Context())/evalPublishLikeProvide(t.Context(), \&p)/' $N/export_eval_ingress_nginx_test.go
fi
