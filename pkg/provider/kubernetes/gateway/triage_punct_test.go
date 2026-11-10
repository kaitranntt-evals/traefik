package gateway_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// Probe for the proposed criterion: two HTTPS listeners of one Gateway whose
// hostnames differ only by '.' versus '-' must stay distinct listeners.
func TestTriage_PunctuationVariantListenerHostnames(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprintf("strictTLSOptions=%t", strict), func(t *testing.T) {
			secret := gateway.EvalIsoNamespacedTLSSecretExported(t, "default", "supersecret", "a-b.example.com", "a.b.example.com")
			eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "dash=a-b.example.com", "dot=a.b.example.com"),
				gateway.EvalIsoRouteExported("http-app-dash", "my-gateway", "dash", "/dash"),
				gateway.EvalIsoRouteExported("http-app-dot", "my-gateway", "dot", "/dot"),
			)
			serve := evalServeHTTPS(t, conf, "websecure", strict)
			code, _ := serve("a-b.example.com", "a-b.example.com", "/dash")
			assert.Equal(t, http.StatusOK, code, "same listener dash")
			code, _ = serve("a.b.example.com", "a.b.example.com", "/dot")
			assert.Equal(t, http.StatusOK, code, "same listener dot")
			for _, c := range [][3]string{
				{"a-b.example.com", "a.b.example.com", "/dot"},
				{"a.b.example.com", "a-b.example.com", "/dash"},
				{"a-b.example.com", "a.b.example.com", "/nope"},
				{"a.b.example.com", "a-b.example.com", "/nope"},
			} {
				code, _ := serve(c[0], c[1], c[2])
				assert.Equal(t, http.StatusMisdirectedRequest, code, "crossed SNI=%s Host=%s path=%s", c[0], c[1], c[2])
			}
		})
	}
}
