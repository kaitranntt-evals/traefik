# Run: sh verify/repro/c7_secret_only_change_mutant.sh  (from a checkout of evalon/traefik-se-3e8db7d3; needs the eval fixtures eval_crd_behavior_test.go + export_eval_test.go in $FIXTURES)
# Demonstrates that no added/modified test guards "a Secret-only change must change the configuration hash":
# a mutant that excludes Middleware.Secrets from hashstructure keeps every authored test green, while the eval fixture catches it.
set -eu
FIXTURES=${FIXTURES:-$HOME/attachments/b184ba04-9152-4ce5-80e8-60e84d99d556/eval_tests/tests}
FILE=pkg/config/dynamic/middlewares.go
cp "$FILE" /tmp/c7_middlewares.go.orig
trap 'cp /tmp/c7_middlewares.go.orig "$FILE"; rm -f pkg/provider/kubernetes/crd/eval_crd_behavior_test.go pkg/provider/kubernetes/crd/export_eval_test.go' EXIT
echo "-- added/modified tests mentioning hash/reload:"
git diff af6d927c51191ae1df104aaf907793386b816c28 HEAD -- '*_test.go' | grep -c -i 'hashstructure\|Hash(\|reload' || true
echo "-- applying mutant: Secrets excluded from hashing"
sed -i 's/Secrets map\[string\]string `json:"-" toml:"-" yaml:"-" label:"-" file:"-" kv:"-"`/Secrets map[string]string `json:"-" toml:"-" yaml:"-" label:"-" file:"-" kv:"-" hash:"ignore"`/' "$FILE"
git diff --stat
echo "-- authored tests under the mutant:"
go test -count=1 ./pkg/config/dynamic/ ./pkg/middlewares/auth/ ./pkg/server/middleware/ ./pkg/provider/kubernetes/crd/ ./pkg/provider/kubernetes/ingress-nginx/
echo "-- eval fixture under the mutant:"
cp "$FIXTURES/eval_crd_behavior_test.go" "$FIXTURES/export_eval_test.go" pkg/provider/kubernetes/crd/
go test -count=1 -run '^TestEval_CRD_SecretRotationAndHashStability$' ./pkg/provider/kubernetes/crd/ || true
