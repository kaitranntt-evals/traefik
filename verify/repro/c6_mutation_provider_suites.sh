#!/bin/bash
# Run: WT=<worktree of evalon/traefik-se-8ca00183> bash verify/repro/c6_mutation_provider_suites.sh   (applies c6_mutation.diff, runs both provider suites, reverts)
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
cd "${WT:?set WT to a checkout of evalon/traefik-se-8ca00183}"
git apply "$HERE/c6_mutation.diff" || exit 1
go test -count=1 ./pkg/provider/kubernetes/crd ./pkg/provider/kubernetes/ingress-nginx
echo "exit=$? (0 means both suites stay green with mutated nested-plugin and default-backend credentials)"
git checkout -- pkg/provider/kubernetes/crd/kubernetes.go pkg/provider/kubernetes/ingress-nginx/build.go
