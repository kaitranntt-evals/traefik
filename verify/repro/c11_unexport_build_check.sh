#!/usr/bin/env bash
# Run: in a checkout of evalon/traefik-se-d6c9b55f: bash verify/repro/c11_unexport_build_check.sh  (renames ParseSecretReference, builds all production code, vets the dynamic package, restores the file)
set -uo pipefail
sed -i 's/^func ParseSecretReference(/func parseSecretReferenceRenamed(/' pkg/config/dynamic/secrets.go
go build ./... && echo "BUILD OK with ParseSecretReference unexported"
go vet ./pkg/config/dynamic/ 2>&1 | head -3
git checkout pkg/config/dynamic/secrets.go
