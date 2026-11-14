#!/usr/bin/env bash
# Run from the repository root: bash verify/repro/c2_cycle_coverage.sh
set -u
sub='^Test_resolveHTTPTLSOptions_descendantAndWildcardChildConflicts$/^cyclic_parent_refs_terminate_safely$'
echo "## 1. cycle subtest, function coverage"
go test -count=1 -v -run "$sub" -coverprofile=/tmp/c2.cover ./pkg/server | grep -E '^(=== RUN|\s*--- |PASS|FAIL|ok)'
go tool cover -func=/tmp/c2.cover | grep -E 'aggregator.go:.*(resolveHTTPTLSOptions|resolveParentChildDomains|findConflictingRouters)'
echo "## 1b. statement blocks of resolveParentChildDomains/rootsOf executed (last column = hit count)"
start=$(grep -n '^func resolveParentChildDomains' pkg/server/aggregator.go | cut -d: -f1)
end=$(awk -v s="$start" 'NR>s && /^}/ {print NR; exit}' pkg/server/aggregator.go)
echo "resolveParentChildDomains spans aggregator.go lines $start-$end"
awk -F'[:, ]' -v s="$start" -v e="$end" '/aggregator.go/ { split($2,a,"."); if (a[1]>=s && a[1]<=e) { n++; hit+=($NF>0) } } END { print "blocks in range: " n ", blocks executed: " hit }' /tmp/c2.cover
echo "## 2. same subtest with the cycle guard (if seen[ref] { continue }) removed from rootsOf"
cp pkg/server/aggregator.go /tmp/aggregator.go.orig
python3 - <<'P'
import re
p='pkg/server/aggregator.go'
s=open(p).read()
n=re.subn(r'\n\t\t\tif seen\[ref\] \{\n\t\t\t\tcontinue\n\t\t\t\}\n','\n',s,count=1)
assert n[1]==1
open(p,'w').write(n[0])
P
git diff --stat -- pkg/server/aggregator.go | tail -1
go test -count=1 -v -run "$sub" ./pkg/server | grep -E '^(=== RUN|\s*--- |PASS|FAIL|ok)'
echo "## 3. guard still removed: same cyclic routers plus one TLS router with an entry point"
cp verify/repro/verify_c2_cycle_test.go pkg/server/verify_c2_cycle_test.go
go test -count=1 -run '^TestVerifyC2_CycleReachesTraversal$' ./pkg/server 2>&1 | grep -E 'stack overflow|goroutine stack exceeds|resolveParentChildDomains\.func|^(FAIL|ok)' | sort | uniq -c | head -8
cp /tmp/aggregator.go.orig pkg/server/aggregator.go
echo "## 4. guard restored: companion test, function coverage"
go test -count=1 -v -run '^TestVerifyC2_CycleReachesTraversal$' -coverprofile=/tmp/c2b.cover ./pkg/server | grep -E '^(=== RUN|\s*--- |PASS|FAIL|ok)'
go tool cover -func=/tmp/c2b.cover | grep -E 'aggregator.go:.*(resolveParentChildDomains)'
rm -f pkg/server/verify_c2_cycle_test.go
git status --short -- pkg
