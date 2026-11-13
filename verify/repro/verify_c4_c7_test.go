//go:build verify

// C4/C7 repro. Run: verify/repro/run.sh 'TestVerify_C4C7'
package gateway_test

import (
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

// A Gateway wildcard listener whose route serves foo.example.com at /gw, next to an external
// exact-host router for foo.example.com with default TLS options: as a leaf (control, the shape
// the eval fixture covers) and as a parent router with a child.
func TestVerify_C4C7_WildcardRouteBesideExternalParent(t *testing.T) {
	for _, name := range []string{"control-external-leaf", "external-parent", "external-parent-priority1", "external-parent-short-host", "external-parent-scoped-to-other-path", "control-external-parent-other-host-priority1"} {
		t.Run(name, func(t *testing.T) {
			host := "foo.example.com"
			if name == "external-parent-short-host" {
				// Default priority is the rule length: Host("a.example.com") is shorter than Host("**.example.com").
				host = "a.example.com"
			}
			gw := vLoad(t, gateway.EvalIsoWhoamiYAML,
				gateway.EvalIsoHTTPSGatewayExported("broad-gw", "supersecret", "broad=*.example.com"),
				gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteExported("gw-route", "broad-gw", "broad", "/gw"), host))
			file := vFileLeaf(host)
			switch name {
			case "external-parent", "external-parent-short-host":
				file = vFileParent(host, 0)
			case "external-parent-priority1":
				file = vFileParent(host, 1)
			case "external-parent-scoped-to-other-path":
				// The parent only takes /other, as the eval fixture's leaf router does, so /gw reaches the Gateway parent at default priorities.
				file = vFileParent(host, 0)
				file.HTTP.Routers["ext-parent"].Rule = `Host("` + host + `") && PathPrefix("/other")`
				file.HTTP.Routers["ext-child"].Rule = `Method("GET")`
			case "control-external-parent-other-host-priority1":
				file = vFileParent("other.example.org", 1)
			}
			rt := vRuntime(t, gw, file, false)
			rt.Dump()
			rt.Req(host, host, "/gw")
			rt.Req(host, host, "/other")
		})
	}
}
