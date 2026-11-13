//go:build verify

// C3/C6 repro. Run: verify/repro/run.sh 'TestVerify_C3C6'
package gateway_test

import (
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

// Routes constrained to an exact hostname beneath wildcard listeners, attached without
// sectionName or through sibling Gateways sharing the websecure entry point, in both TLS modes.
func TestVerify_C3C6_ChildHostnamePropagation(t *testing.T) {
	type scenario struct {
		name string
		docs []string
		reqs [][3]string
	}
	scenarios := []scenario{
		{
			// Control: every route names its listener; nothing crosses listener parents.
			name: "control-sectioned",
			docs: []string{
				gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "outer=*.example.com", "inner=*.foo.example.com"),
				gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteWithServiceExported("r-inner", "gw", "inner", "/inner", "whoami-precise"), "a.foo.example.com"),
				gateway.EvalIsoRouteWithServiceExported("r-outer", "gw", "outer", "/outer", "whoami-catchall"),
			},
		},
		{
			// One Gateway, the constrained route attaches without sectionName to both listeners.
			name: "unsectioned",
			docs: []string{
				gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "outer=*.example.com", "inner=*.foo.example.com"),
				gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteWithServiceExported("r-inner", "gw", "", "/inner", "whoami-precise"), "a.foo.example.com"),
				gateway.EvalIsoRouteWithServiceExported("r-outer", "gw", "outer", "/outer", "whoami-catchall"),
			},
		},
		{
			// Sibling Gateways sharing the entry point; gw-inner's route names its listener.
			name: "sibling-gateways",
			docs: []string{
				gateway.EvalIsoHTTPSGatewayExported("gw-outer", "supersecret", "outer=*.example.com"),
				gateway.EvalIsoHTTPSGatewayExported("gw-inner", "supersecret", "inner=*.foo.example.com"),
				gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteWithServiceExported("r-inner", "gw-outer", "outer", "/inner", "whoami-precise"), "a.foo.example.com"),
				gateway.EvalIsoRouteWithServiceExported("r-inner2", "gw-inner", "inner", "/inner2", "whoami-precise"),
				gateway.EvalIsoRouteWithServiceExported("r-outer", "gw-outer", "outer", "/outer", "whoami-catchall"),
			},
		},
	}
	for _, sc := range scenarios {
		for _, strict := range []bool{false, true} {
			name := sc.name + "/non-strict"
			if strict {
				name = sc.name + "/strict"
			}
			t.Run(name, func(t *testing.T) {
				rt := vRuntime(t, vLoad(t, evalIsoMultiBackendYAML, sc.docs...), nil, strict)
				rt.Dump()
				t.Log("-- equal SNI/Host (must be served)")
				rt.Req("a.foo.example.com", "a.foo.example.com", "/inner")
				rt.Req("a.foo.example.com", "a.foo.example.com", "/inner2")
				rt.Req("b.example.com", "b.example.com", "/outer")
				t.Log("-- crossed SNI/Host between the two listeners (must be 421)")
				rt.Req("b.example.com", "a.foo.example.com", "/inner")
				rt.Req("b.example.com", "a.foo.example.com", "/inner2")
				rt.Req("a.foo.example.com", "b.example.com", "/outer")
			})
		}
	}
}
