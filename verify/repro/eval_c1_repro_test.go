// Repro for C1 (branch evalon/traefik-ga-f4e84e3e of kaitranntt-evals/traefik-gateway-listener-isolation).
// Run: copy this file plus the eval fixtures (eval_https_test.go, eval_isolation_test.go, export_eval_test.go) into
// pkg/provider/kubernetes/gateway/ on that branch, then:
//
//	go test -v -race -count=1 -timeout=5m -run '^TestC1_' ./pkg/provider/kubernetes/gateway
//
// Expected on a conforming implementation: PASS. Observed on the claim branch: crossed SNI/Host returns 500 (HTTPRoute) / 200 (GRPCRoute), not 421.
package gateway_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// unresolvedExtensionRef is an ExtensionRef filter whose group/kind the provider does not know,
// which makes loadMiddlewares / loadGRPCMiddlewares return an error.
const unresolvedExtensionRef = `
      filters:
        - type: ExtensionRef
          extensionRef:
            group: example.com
            kind: Unknown
            name: does-not-exist
`

func c1HTTPRouteWithBadFilter(name, gw, section, path string) string {
	doc := gateway.EvalIsoRouteExported(name, gw, section, path)
	return strings.Replace(doc, "      backendRefs:", unresolvedExtensionRef+"      backendRefs:", 1)
}

func c1GRPCRouteWithBadFilter(name, gw, section, method string) string {
	doc := gateway.EvalIsoGRPCRouteExported(name, gw, section, method)
	return strings.Replace(doc, "      backendRefs:", unresolvedExtensionRef+"      backendRefs:", 1)
}

func TestC1_HTTPRoute_UnresolvedExtensionRef_CrossedSNIHost(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run("strictTLSOptions="+map[bool]string{false: "false", true: "true"}[strict], func(t *testing.T) {
			secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
			eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "catchall=", "precise=abc.example.com"),
				c1HTTPRouteWithBadFilter("http-app-catchall", "my-gateway", "catchall", "/catchall"),
				c1HTTPRouteWithBadFilter("http-app-precise", "my-gateway", "precise", "/precise"),
			)

			// Error-service protection: inspect the generated routers/services.
			for name, r := range conf.HTTP.Routers {
				t.Logf("ROUTER %s rule=%q middlewares=%v service=%s tls=%+v", name, r.Rule, r.Middlewares, r.Service, r.TLS)
				if strings.HasPrefix(name, "httproute-") {
					require.True(t, strings.HasSuffix(r.Service, "-err-wrr"), "filter error must produce the err-wrr service")
					svc := conf.HTTP.Services[r.Service]
					require.NotNil(t, svc)
					t.Logf("SERVICE %s status=%d", r.Service, *svc.Weighted.Services[0].Status)
					assert.NotContains(t, strings.Join(r.Middlewares, ","), "misdirected-request",
						"filter-error router carries no MisdirectedRequest middleware")
				}
			}

			serve := evalServeHTTPS(t, conf, "websecure", strict)

			// Same-listener requests reach the error service (500), as designed.
			code, body := serve("abc.example.com", "abc.example.com", "/precise")
			t.Logf("same listener exact: code=%d body=%q", code, body)
			assert.Equal(t, http.StatusInternalServerError, code)
			code, body = serve("xyz.example.com", "xyz.example.com", "/catchall")
			t.Logf("same listener catchall: code=%d body=%q", code, body)
			assert.Equal(t, http.StatusInternalServerError, code)

			// Crossed SNI/Host: the Gateway API requires 421 Misdirected Request.
			code, body = serve("abc.example.com", "xyz.example.com", "/catchall")
			t.Logf("CROSSED SNI=abc.example.com Host=xyz.example.com /catchall: code=%d body=%q", code, body)
			assert.Equal(t, http.StatusMisdirectedRequest, code, "SNI exact, Host catchall")
			code, body = serve("xyz.example.com", "abc.example.com", "/precise")
			t.Logf("CROSSED SNI=xyz.example.com Host=abc.example.com /precise: code=%d body=%q", code, body)
			assert.Equal(t, http.StatusMisdirectedRequest, code, "SNI catchall, Host exact")
		})
	}
}

func TestC1_GRPCRoute_UnresolvedExtensionRef_CrossedSNIHost(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
		gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "catchall=", "precise=abc.example.com"),
		c1GRPCRouteWithBadFilter("grpc-app-catchall", "my-gateway", "catchall", "SayHello"),
		c1GRPCRouteWithBadFilter("grpc-app-precise", "my-gateway", "precise", "SayGoodbye"),
	)
	for name, r := range conf.HTTP.Routers {
		t.Logf("ROUTER %s rule=%q middlewares=%v service=%s", name, r.Rule, r.Middlewares, r.Service)
		if strings.HasPrefix(name, "grpcroute-") {
			require.True(t, strings.HasSuffix(r.Service, "-err-wrr"))
			assert.NotContains(t, strings.Join(r.Middlewares, ","), "misdirected-request")
		}
	}

	serve := evalServeHTTPS(t, conf, "websecure", false)
	grpcHdr := map[string]string{":method": http.MethodPost, "Content-Type": "application/grpc"}
	code, body := serve("abc.example.com", "xyz.example.com", "/helloworld.Greeter/SayHello", grpcHdr)
	t.Logf("CROSSED SNI=abc.example.com Host=xyz.example.com SayHello: code=%d body=%q", code, body)
	assert.Equal(t, http.StatusMisdirectedRequest, code)
	code, body = serve("xyz.example.com", "abc.example.com", "/helloworld.Greeter/SayGoodbye", grpcHdr)
	t.Logf("CROSSED SNI=xyz.example.com Host=abc.example.com SayGoodbye: code=%d body=%q", code, body)
	assert.Equal(t, http.StatusMisdirectedRequest, code)
}
