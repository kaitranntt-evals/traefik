#!/bin/sh
# Run from the repo root of a C1 branch worktree: sh verify/repro/c1_mutate.sh   (undo with: git checkout -- pkg/provider/kubernetes/gateway/kubernetes.go)
# Mutation: at the end of loadConfigurationFromGateways, swap the Rule of the broader-listener (*.example.com) HTTPRoute router
# with the Rule of the abc.foo.example.com HTTPRoute router on the plain-HTTP "web" entryPoint. The set of rules is unchanged;
# only which backend route carries which rule changes, so a request for abc.foo.example.com now reaches the broader route.
set -e
f=pkg/provider/kubernetes/gateway/kubernetes.go
awk '
/^func \(p \*Provider\) loadConfigurationFromGateways\(/ { infn=1 }
infn && /^\treturn conf, statusReport, nil$/ && !done {
  print "\t// VERIFY C1 MUTATION: swap broader/narrower route rules on web."
  print "\t{"
  print "\t\tvar broad, narrow string"
  print "\t\tfor name, r := range conf.HTTP.Routers {"
  print "\t\t\tif r.TLS != nil || len(r.EntryPoints) != 1 || r.EntryPoints[0] != \"web\" {"
  print "\t\t\t\tcontinue"
  print "\t\t\t}"
  print "\t\t\tif strings.HasPrefix(r.Rule, `Host(\"**.example.com\") &&`) {"
  print "\t\t\t\tbroad = name"
  print "\t\t\t}"
  print "\t\t\tif strings.HasPrefix(r.Rule, `Host(\"abc.foo.example.com\") &&`) {"
  print "\t\t\t\tnarrow = name"
  print "\t\t\t}"
  print "\t\t}"
  print "\t\tif broad != \"\" && narrow != \"\" {"
  print "\t\t\tconf.HTTP.Routers[broad].Rule, conf.HTTP.Routers[narrow].Rule = conf.HTTP.Routers[narrow].Rule, conf.HTTP.Routers[broad].Rule"
  print "\t\t}"
  print "\t}"
  done=1
}
{ print }
' "$f" > "$f.tmp" && mv "$f.tmp" "$f"
grep -n "VERIFY C1 MUTATION" "$f"
