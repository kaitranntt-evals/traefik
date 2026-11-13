//go:build verify

// C8 repro (branch evalon/traefik-ga-b6839099). Run from a checkout of that branch: <path>/verify/repro/run.sh 'TestVerify_C8' <checkout root>
package gateway_test

import (
	"sort"
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

// Two HTTPS listeners of one Gateway with distinct hostnames: names that differ after
// provider.Normalize (control) and names that normalize identically (edge-a / edge--a).
func TestVerify_C8_NormalizedListenerNames(t *testing.T) {
	for _, tc := range []struct{ name, l1, l2 string }{
		{"control-edge-a-edge-b", "edge-a", "edge-b"},
		{"edge-a-edge--a", "edge-a", "edge--a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gw := vLoad(t, evalIsoMultiBackendYAML,
				gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", tc.l1+"=a.example.com", tc.l2+"=b.example.com"),
				gateway.EvalIsoRouteWithServiceExported("r-a", "gw", tc.l1, "/a", "whoami-precise"),
				gateway.EvalIsoRouteWithServiceExported("r-b", "gw", tc.l2, "/b", "whoami-catchall"))
			var opts []string
			for n := range gw.TLS.Options {
				opts = append(opts, n)
			}
			sort.Strings(opts)
			t.Logf("PROVIDER TLS OPTIONS (%d): %v", len(opts), opts)
			rt := vRuntime(t, gw, nil, false)
			rt.Dump()
			t.Log("-- equal SNI/Host")
			rt.Req("a.example.com", "a.example.com", "/a")
			rt.Req("b.example.com", "b.example.com", "/b")
			t.Log("-- crossed SNI/Host (must be 421)")
			rt.Req("a.example.com", "b.example.com", "/b")
			rt.Req("b.example.com", "a.example.com", "/a")
		})
	}
}
