//go:build verify

// C1 repro (per branch). Run from this branch: verify/repro/run.sh 'TestVerify_C1' <checkout root of the evalon/traefik-ga-* branch>
package gateway_test

import (
	"sort"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

func vOpts(t *testing.T, gw *dynamic.Configuration) {
	t.Helper()
	var opts []string
	if gw.TLS != nil {
		for n := range gw.TLS.Options {
			opts = append(opts, n)
		}
	}
	sort.Strings(opts)
	t.Logf("PROVIDER TLS OPTIONS (%d): %v", len(opts), opts)
}

// Compares the TLS options documented for Gateway HTTPS listeners with the ones actually declared,
// resolved, and selected from the SNI, in normal operation.
func TestVerify_C1_DocumentedTLSOptions(t *testing.T) {
	// One Gateway, HTTPS listeners "*.example.com" and "foo.example.com", hostname-free routes.
	t.Run("wildcard-and-exact-listeners", func(t *testing.T) {
		gw := vLoad(t, evalIsoMultiBackendYAML,
			gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "wild=*.example.com", "exact=foo.example.com"),
			gateway.EvalIsoRouteWithServiceExported("r-wild", "gw", "wild", "/w", "whoami-catchall"),
			gateway.EvalIsoRouteWithServiceExported("r-exact", "gw", "exact", "/e", "whoami-precise"))
		vOpts(t, gw)
		rt := vRuntime(t, gw, nil, false)
		rt.Dump()
		rt.Req("bar.example.com", "bar.example.com", "/w")
		rt.Req("foo.example.com", "foo.example.com", "/e")
		rt.Req("bar.example.com", "foo.example.com", "/e")
		rt.Req("foo.example.com", "bar.example.com", "/w")
	})
	// Same listeners, the wildcard listener's route restricted to a single hostname.
	t.Run("wildcard-listener-route-with-hostname", func(t *testing.T) {
		gw := vLoad(t, evalIsoMultiBackendYAML,
			gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "wild=*.example.com", "exact=foo.example.com"),
			gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteWithServiceExported("r-wild", "gw", "wild", "/w", "whoami-catchall"), "bar.example.com"),
			gateway.EvalIsoRouteWithServiceExported("r-exact", "gw", "exact", "/e", "whoami-precise"))
		vOpts(t, gw)
		rt := vRuntime(t, gw, nil, false)
		rt.Dump()
		rt.Req("bar.example.com", "bar.example.com", "/w")
		rt.Req("baz.example.com", "baz.example.com", "/w")
		rt.Req("baz.example.com", "foo.example.com", "/e")
		rt.Req("foo.example.com", "baz.example.com", "/w")
	})
	// A single HTTPS listener with a hostname.
	t.Run("single-exact-listener", func(t *testing.T) {
		gw := vLoad(t, evalIsoMultiBackendYAML,
			gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "exact=foo.example.com"),
			gateway.EvalIsoRouteWithServiceExported("r-exact", "gw", "exact", "/e", "whoami-precise"))
		vOpts(t, gw)
		rt := vRuntime(t, gw, nil, false)
		rt.Dump()
		rt.Req("foo.example.com", "foo.example.com", "/e")
	})
	// A single wildcard HTTPS listener.
	t.Run("single-wildcard-listener", func(t *testing.T) {
		gw := vLoad(t, evalIsoMultiBackendYAML,
			gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "wild=*.example.com"),
			gateway.EvalIsoRouteWithServiceExported("r-wild", "gw", "wild", "/w", "whoami-catchall"))
		vOpts(t, gw)
		rt := vRuntime(t, gw, nil, false)
		rt.Dump()
		rt.Req("bar.example.com", "bar.example.com", "/w")
	})
	// Two wildcard HTTPS listeners, hostname-free routes.
	t.Run("two-wildcard-listeners", func(t *testing.T) {
		gw := vLoad(t, evalIsoMultiBackendYAML,
			gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "wild=*.example.com", "sub=*.foo.example.com"),
			gateway.EvalIsoRouteWithServiceExported("r-wild", "gw", "wild", "/w", "whoami-catchall"),
			gateway.EvalIsoRouteWithServiceExported("r-sub", "gw", "sub", "/s", "whoami-precise"))
		vOpts(t, gw)
		rt := vRuntime(t, gw, nil, false)
		rt.Dump()
		rt.Req("bar.example.com", "bar.example.com", "/w")
		rt.Req("a.foo.example.com", "a.foo.example.com", "/s")
		rt.Req("bar.example.com", "a.foo.example.com", "/s")
		rt.Req("a.foo.example.com", "bar.example.com", "/w")
	})
	// Cross-provider fallback: wildcard + exact Gateway listeners beside a file-provider router
	// on a host of the wildcard listener, using the default TLS options.
	for _, strict := range []bool{false, true} {
		name := "fallback-external-router-on-wildcard-host"
		if strict {
			name += "-strict"
		}
		t.Run(name, func(t *testing.T) {
			gw := vLoad(t, evalIsoMultiBackendYAML,
				gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "wild=*.example.com", "exact=foo.example.com"),
				gateway.EvalIsoRouteWithServiceExported("r-wild", "gw", "wild", "/w", "whoami-catchall"),
				gateway.EvalIsoRouteWithServiceExported("r-exact", "gw", "exact", "/e", "whoami-precise"))
			rt := vRuntime(t, gw, vFileLeaf("bar.example.com"), strict)
			rt.Dump()
			rt.Req("bar.example.com", "bar.example.com", "/w")
			rt.Req("bar.example.com", "bar.example.com", "/other")
			rt.Req("baz.example.com", "baz.example.com", "/w")
			rt.Req("foo.example.com", "foo.example.com", "/e")
		})
	}
	// Cross-provider fallback on the exact listener host.
	t.Run("fallback-external-router-on-exact-host", func(t *testing.T) {
		gw := vLoad(t, evalIsoMultiBackendYAML,
			gateway.EvalIsoHTTPSGatewayExported("gw", "supersecret", "wild=*.example.com", "exact=foo.example.com"),
			gateway.EvalIsoRouteWithServiceExported("r-wild", "gw", "wild", "/w", "whoami-catchall"),
			gateway.EvalIsoRouteWithServiceExported("r-exact", "gw", "exact", "/e", "whoami-precise"))
		rt := vRuntime(t, gw, vFileLeaf("foo.example.com"), false)
		rt.Dump()
		rt.Req("foo.example.com", "foo.example.com", "/e")
		rt.Req("foo.example.com", "foo.example.com", "/other")
		rt.Req("bar.example.com", "bar.example.com", "/w")
	})
}
