#!/bin/bash
# Run: verify/repro/install.sh <worktree> [eval_tests_dir] — copies the eval fixtures (byte-exact, default ~/evalfx/tests) and the verify probes/plugin into a claim-branch worktree.
set -e
W=$1; T=${2:-$HOME/evalfx/tests}; R=$(cd "$(dirname "$0")" && pwd)
C=$W/pkg/provider/kubernetes/crd; N=$W/pkg/provider/kubernetes/ingress-nginx
for f in eval_crd_behavior_test.go export_eval_test.go eval_provider_to_builder_runtime_test.go eval_redis_propagation_test.go eval_namespace_isolation_test.go eval_deepcopy_isolation_test.go eval_lifecycle_binding_test.go; do cp "$T/$f" "$C/$f"; done
mkdir -p "$C/plugins-local/src/evalplugin" "$C/plugins-local/src/verifyplugin" "$C/plugins-local/src/verifyretain"
cp "$T/eval_local_plugin_manifest.yml" "$C/plugins-local/src/evalplugin/.traefik.yml"
cp "$T/eval_local_plugin.go" "$C/plugins-local/src/evalplugin/evalplugin.go"
cp "$T/eval_ingress_nginx_test.go" "$T/export_eval_ingress_nginx_test.go" "$N/"
cp "$T/eval_auth_sanitation_test.go" "$W/pkg/middlewares/auth/"
cp "$T/eval_logconfiguration_test.go" "$W/pkg/server/"
cp "$R/verifyplugin/.traefik.yml" "$R/verifyplugin/verifyplugin.go" "$C/plugins-local/src/verifyplugin/"
cp "$R/c1_c2_plugin_probe_test.go" "$C/verify_c1_c2_plugin_probe_test.go"
cp "$R/verifyretain/.traefik.yml" "$R/verifyretain/verifyretain.go" "$C/plugins-local/src/verifyretain/"
cp "$R/c1_c5_retain_probe_test.go" "$C/verify_c1_c5_retain_probe_test.go"
