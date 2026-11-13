#!/bin/bash
# Run: verify/repro/install_fixtures.sh <dir-with-extracted-eval_tests.zip>/tests <worktree>   (installs the eval fixtures byte-exact at the paths the audit brief lists)
S=$1; W=$2; C=$W/pkg/provider/kubernetes/crd; N=$W/pkg/provider/kubernetes/ingress-nginx
for f in eval_crd_behavior_test.go export_eval_test.go eval_provider_to_builder_runtime_test.go eval_redis_propagation_test.go eval_namespace_isolation_test.go eval_deepcopy_isolation_test.go eval_lifecycle_binding_test.go; do cp $S/$f $C/$f; done
cp $S/eval_ingress_nginx_test.go $S/export_eval_ingress_nginx_test.go $N/
mkdir -p $C/plugins-local/src/evalplugin $C/plugins-local/src/evalretain $C/plugins-local/src/evalwasm
cp $S/eval_local_plugin_manifest.yml $C/plugins-local/src/evalplugin/.traefik.yml
cp $S/eval_local_plugin.go $C/plugins-local/src/evalplugin/evalplugin.go
cp $S/eval_retain_plugin_manifest.yml $C/plugins-local/src/evalretain/.traefik.yml
cp $S/eval_retain_plugin.go $C/plugins-local/src/evalretain/evalretain.go
cp $S/eval_wasm_plugin_manifest.yml $C/plugins-local/src/evalwasm/.traefik.yml
cp $S/eval_auth_sanitation_test.go $W/pkg/middlewares/auth/
cp $S/eval_logconfiguration_test.go $W/pkg/server/
