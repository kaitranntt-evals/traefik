//go:build verify

// Run: copy next to the eval fixtures in pkg/provider/kubernetes/gateway/ on branch evalon/traefik-ga-715b83ff, then
//   go test -tags verify -count=1 -run 'TestVerifyC1' -v ./pkg/provider/kubernetes/gateway/
package gateway_test

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

func verifyC1Backend(ns string) string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: whoami
  namespace: %[1]s
spec:
  clusterIP: 10.10.10.1
  ports:
    - name: web
      protocol: TCP
      port: 80
      targetPort: web
---
kind: EndpointSlice
apiVersion: discovery.k8s.io/v1
metadata:
  name: whoami-slice
  namespace: %[1]s
  labels:
    kubernetes.io/service-name: whoami
addressType: IPv4
ports:
  - name: web
    port: 80
endpoints:
  - addresses:
      - 10.10.0.1
    conditions:
      ready: true
`, ns)
}

// verifyC1Gateway renders an HTTPS Gateway in ns with listeners "name=hostname".
func verifyC1Gateway(ns, name, secret string, listeners ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nkind: Gateway\napiVersion: gateway.networking.k8s.io/v1\nmetadata:\n  name: %s\n  namespace: %s\nspec:\n  gatewayClassName: my-gateway-class\n  listeners:\n", name, ns)
	for _, l := range listeners {
		parts := strings.SplitN(l, "=", 2)
		fmt.Fprintf(&b, "    - name: %s\n      protocol: HTTPS\n      port: 443\n      hostname: %q\n      tls:\n        certificateRefs:\n          - kind: Secret\n            name: %s\n            group: \"\"\n      allowedRoutes:\n        namespaces:\n          from: Same\n", parts[0], parts[1], secret)
	}
	return b.String()
}

func verifyC1Route(ns, name, gw, section, path string) string {
	return fmt.Sprintf(`
kind: HTTPRoute
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: %s
  namespace: %s
spec:
  parentRefs:
    - name: %s
      sectionName: %s
      kind: Gateway
      group: gateway.networking.k8s.io
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: %s
      backendRefs:
        - name: whoami
          port: 80
          kind: Service
          group: ""
`, name, ns, gw, section, path)
}

func TestVerifyC1(t *testing.T) {
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	for _, tc := range []struct{ gwBNs, gwBName string }{
		{"team", "a-edge"},  // claim: collides with team-a/edge
		{"team", "b-edge"},  // control: distinct identity
	} {
		t.Run(tc.gwBNs+"/"+tc.gwBName, func(t *testing.T) {
			secrets := []k8sruntime.Object{
				gateway.EvalIsoNamespacedTLSSecretExported(t, "team-a", "cert", "*.alpha.example.com", "app.alpha.example.com"),
				gateway.EvalIsoNamespacedTLSSecretExported(t, "team", "cert", "*.beta.example.com", "app.beta.example.com"),
			}
			conf := gateway.EvalIsoLoadExported(t, eps, secrets,
				gateway.EvalIsoClassYAML,
				verifyC1Backend("team-a"), verifyC1Backend("team"),
				verifyC1Gateway("team-a", "edge", "cert", "wild=*.alpha.example.com", "exact=app.alpha.example.com"),
				verifyC1Gateway(tc.gwBNs, tc.gwBName, "cert", "wild=*.beta.example.com", "exact=app.beta.example.com"),
				verifyC1Route("team-a", "wild-route", "edge", "wild", "/wild"),
				verifyC1Route("team-a", "exact-route", "edge", "exact", "/exact"),
				verifyC1Route(tc.gwBNs, "wild-route", tc.gwBName, "wild", "/wild"),
				verifyC1Route(tc.gwBNs, "exact-route", tc.gwBName, "exact", "/exact"),
			)

			var names []string
			for n := range conf.HTTP.Routers {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				r := conf.HTTP.Routers[n]
				if strings.Contains(n, "fallback") {
					t.Logf("FALLBACK ROUTER %s: rule=%s listenerHostnames=%q", n, r.Rule, r.TLS.ListenerHostnames)
				} else {
					t.Logf("ROUTER %s: rule=%s", n, r.Rule)
				}
			}

			serve := evalServeHTTPS(t, conf, "websecure", false)
			for _, rq := range []struct{ sni, host, path string }{
				{"app.alpha.example.com", "app.alpha.example.com", "/exact"},
				{"x.alpha.example.com", "x.alpha.example.com", "/wild"},
				{"app.beta.example.com", "app.beta.example.com", "/exact"},
				{"x.beta.example.com", "x.beta.example.com", "/wild"},
				// crossed SNI/Host within Gateway team-a/edge, unmatched path
				{"app.alpha.example.com", "x.alpha.example.com", "/nope"},
				{"x.alpha.example.com", "app.alpha.example.com", "/nope"},
				// crossed SNI/Host within Gateway B, unmatched path
				{"app.beta.example.com", "x.beta.example.com", "/nope"},
				{"x.beta.example.com", "app.beta.example.com", "/nope"},
				// crossed SNI/Host on matched paths
				{"app.alpha.example.com", "x.alpha.example.com", "/wild"},
				{"app.beta.example.com", "x.beta.example.com", "/wild"},
			} {
				code, body := serve(rq.sni, rq.host, rq.path)
				t.Logf("REQ sni=%-24s host=%-24s path=%-6s -> %d %s", rq.sni, rq.host, rq.path, code, strings.TrimSpace(body))
				_ = http.StatusOK
			}
		})
	}
}
