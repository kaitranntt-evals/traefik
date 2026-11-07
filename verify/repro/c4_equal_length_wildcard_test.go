//go:build verify

// Run: copy next to the eval fixtures in pkg/provider/kubernetes/gateway/ on branch evalon/traefik-ga-963e6173, then
//   go test -tags verify -count=1 -run 'TestVerifyC4' -v ./pkg/provider/kubernetes/gateway/
package gateway_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

func TestVerifyC4(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	for _, tc := range []struct {
		name  string
		exact string
	}{
		{"equal-length a.example.com vs *.example.com", "a.example.com"},
		{"longer abc.example.com vs *.example.com", "abc.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "exact="+tc.exact, "wildcard=*.example.com"),
				gateway.EvalIsoRouteExported("http-app-exact", "my-gateway", "exact", "/exact"),
				gateway.EvalIsoRouteExported("http-app-wildcard", "my-gateway", "wildcard", "/wildcard"),
			)

			var names []string
			for n := range conf.HTTP.Routers {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				r := conf.HTTP.Routers[n]
				t.Logf("ROUTER %s: prio=%d middlewares=%v rule=%s", n, r.Priority, r.Middlewares, r.Rule)
			}
			for n, m := range conf.HTTP.Middlewares {
				if m.ListenerIsolation != nil {
					t.Logf("MIDDLEWARE %s: hostname=%q siblings=%q", n, m.ListenerIsolation.Hostname, m.ListenerIsolation.SiblingHostnames)
				}
			}

			serve := evalServeHTTPS(t, conf, "websecure", false)
			for _, rq := range []struct{ sni, host, path string }{
				{tc.exact, tc.exact, "/exact"},        // valid request on the exact listener
				{"other.example.com", "other.example.com", "/wildcard"}, // valid request on the wildcard listener
				{"other.example.com", tc.exact, "/exact"}, // crossed: should be 421
				{tc.exact, "other.example.com", "/wildcard"}, // crossed: should be 421
				{tc.exact, tc.exact, "/nope"},
			} {
				code, body := serve(rq.sni, rq.host, rq.path)
				t.Logf("REQ sni=%-18s host=%-18s path=%-9s -> %d %s", rq.sni, rq.host, rq.path, code, strings.TrimSpace(body))
			}
		})
	}
}
