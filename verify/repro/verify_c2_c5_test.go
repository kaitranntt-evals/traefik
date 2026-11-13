//go:build verify

// C2/C5 repro. Run: verify/repro/run.sh 'TestVerify_C2C5'
package gateway_test

import (
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

// A hostname-free HTTPRoute under a broad (catch-all or wildcard) HTTPS listener, next to a
// file-provider exact-host router using the default TLS options.
func TestVerify_C2C5_BroadRouteBesideExternalExactHost(t *testing.T) {
	for _, tc := range []struct{ name, listener string }{{"catchall", "broad="}, {"wildcard", "broad=*.example.com"}} {
		for _, withFile := range []bool{false, true} {
			name := tc.name + "/control-gateway-only"
			if withFile {
				name = tc.name + "/with-external-router"
			}
			t.Run(name, func(t *testing.T) {
				gw := vLoad(t, gateway.EvalIsoWhoamiYAML,
					gateway.EvalIsoHTTPSGatewayExported("broad-gw", "supersecret", tc.listener),
					gateway.EvalIsoRouteExported("gw-route", "broad-gw", "broad", "/gw"))
				var rt *vRT
				if withFile {
					rt = vRuntime(t, gw, vFileLeaf("foo.example.com"), false)
				} else {
					rt = vRuntime(t, gw, nil, false)
				}
				rt.Dump()
				rt.Req("foo.example.com", "foo.example.com", "/gw")
				rt.Req("foo.example.com", "foo.example.com", "/other")
				rt.Req("bar.example.com", "bar.example.com", "/gw")
			})
		}
	}
}
