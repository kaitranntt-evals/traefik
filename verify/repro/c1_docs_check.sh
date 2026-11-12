#!/bin/sh
# Run from the repo root: sh verify/repro/c1_docs_check.sh [<rev>]   (default rev: evalrepo/evalon/traefik-ga-fadc98fd, fetched from kaitranntt-evals/traefik-gateway-listener-isolation)
REV="${1:-evalrepo/evalon/traefik-ga-fadc98fd}"
BASE=35c7530ac0906b5407a3733de173d58d9c427949
GW=docs/content/reference/routing-configuration/kubernetes/gateway-api.md

echo "## docs files changed by the branch"
git diff --stat "$BASE" "$REV" -- docs | cat

echo "## the promise: Listener Isolation section of $GW"
git show "$REV:$GW" | awk '/^### Listener Isolation/{p=1} /^## Exposing a Route/{p=0} p'

echo "## qualifications inside $GW (wildcard / HostRegexp / fallback / default TLS options / strict)"
git show "$REV:$GW" | grep -n -i -E 'HostRegexp|fall ?back|strictTLSOptions|wildcard listener|no domain|`default` TLS options' || echo "(only the lines above match, if any)"

echo "## every docs/ mention of Gateway listeners together with TLS options"
git grep -n -i -E 'listener' "$REV" -- docs/content | grep -i -E 'TLS option' || echo "(none outside the section above)"

echo "## generic (non-Gateway) statements about HostRegexp and TLS options under docs/"
git grep -n -E 'HostRegexp' "$REV" -- docs/content | grep -i -E 'TLS ?option'
