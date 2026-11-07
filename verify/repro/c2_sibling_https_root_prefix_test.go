//go:build verify

// Run (on the claim branch, with the eval fixtures copied into the package dir): cp verify/repro/c2_sibling_https_root_prefix_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -race -count=1 -run '^TestVerifyC2_' -v ./pkg/provider/kubernetes/gateway

package gateway_test

import (
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

func verifyC2RootPrefixRoute(name, gw, section string) string {
	return `
kind: HTTPRoute
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: ` + name + `
  namespace: default
spec:
  parentRefs:
    - name: ` + gw + `
      sectionName: ` + section + `
      kind: Gateway
      group: gateway.networking.k8s.io
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: whoami
          port: 80
          kind: Service
          group: ""
`
}

// Sibling HTTPS Gateways on one entry point: gw-exact owns abc.example.com with an Exact /precise route,
// gw-catchall has a hostname-less listener with a PathPrefix("/") route to the whoami backend.
func TestVerifyC2_SiblingHTTPS_RootPrefixSiblingVsExactHostFallback(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run("strict="+map[bool]string{false: "false", true: "true"}[strict], func(t *testing.T) {
			secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
			eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("gw-exact", "supersecret", "precise=abc.example.com"),
				gateway.EvalIsoHTTPSGatewayExported("gw-catchall", "supersecret", "catchall="),
				gateway.EvalIsoRouteExported("http-app-precise", "gw-exact", "precise", "/precise"),
				verifyC2RootPrefixRoute("http-app-root", "gw-catchall", "catchall"),
			)

			names := make([]string, 0, len(conf.HTTP.Routers))
			for n := range conf.HTTP.Routers {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				r := conf.HTTP.Routers[n]
				t.Logf("router %s: rule=%s priority=%d service=%s", n, r.Rule, r.Priority, r.Service)
			}

			serve := evalServeHTTPS(t, conf, "websecure", strict)

			code, _ := serve("abc.example.com", "abc.example.com", "/precise")
			t.Logf("abc.example.com /precise -> %d", code)
			assert.Equal(t, http.StatusOK, code, "exact route on exact-host listener")

			code, _ = serve("xyz.example.com", "xyz.example.com", "/outside")
			t.Logf("xyz.example.com /outside -> %d", code)
			assert.Equal(t, http.StatusOK, code, "unreserved host reaches sibling PathPrefix(/) route")

			code, body := serve("abc.example.com", "abc.example.com", "/outside")
			t.Logf("abc.example.com /outside -> %d body=%q", code, body)
			assert.Equal(t, http.StatusOK, code, "claim C2: expected to reach sibling catch-all PathPrefix(/) backend; 404 means the exact-host fallback won")
		})
	}
}

// Contrast: same topology but the sibling catch-all route is an Exact path (as in the eval fixture).
func TestVerifyC2_SiblingHTTPS_ExactSiblingReachable(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
		gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("gw-exact", "supersecret", "precise=abc.example.com"),
		gateway.EvalIsoHTTPSGatewayExported("gw-catchall", "supersecret", "catchall="),
		gateway.EvalIsoRouteExported("http-app-precise", "gw-exact", "precise", "/precise"),
		gateway.EvalIsoRouteExported("http-app-other", "gw-catchall", "catchall", "/other"),
	)
	serve := evalServeHTTPS(t, conf, "websecure", false)
	code, _ := serve("abc.example.com", "abc.example.com", "/other")
	t.Logf("abc.example.com /other (Exact sibling route) -> %d", code)
	assert.Equal(t, http.StatusOK, code)
}
