// Run (after copying the verify_*_test.go files of verify/repro/ into pkg/provider/kubernetes/gateway/):
//
//	go test -count=1 -v -run '^TestVerifyHyphenDotHostnameIdentity$' ./pkg/provider/kubernetes/gateway
package gateway_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

func TestVerifyHyphenDotHostnameIdentity(t *testing.T) {
	for _, test := range []struct {
		desc string
		a, b string
	}{
		{desc: "control: hostnames that normalize differently", a: "foo.example.com", b: "bar.example.com"},
		{desc: "hostnames differing only by hyphen and dot", a: "foo-bar.example.com", b: "foo.bar.example.com"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			docs := []string{
				verifyClassYAML, verifyServiceYAML("a", "10.10.0.1", 80), verifyServiceYAML("b", "10.10.0.2", 80),
				verifyHTTPRouteYAML("a", "a", "/", "a"), verifyHTTPRouteYAML("b", "b", "/", "b"),
				verifyGatewayYAML(verifyHTTPSListenerYAML("a", test.a), verifyHTTPSListenerYAML("b", test.b)),
			}

			conf, status := gateway.VerifyLoad(t, map[string]gateway.Entrypoint{"websecure": {Address: ":443"}},
				[]k8sruntime.Object{gateway.VerifySecret(t, "gw-cert", "gateway-listener-cert", "*.example.com")}, docs...)
			for _, s := range status {
				t.Log("STATUS ", s)
			}

			certPEM, keyPEM := gateway.VerifyCert(t, "traefik-default-fallback")
			fallback, err := tls.X509KeyPair(certPEM, keyPEM)
			require.NoError(t, err)
			rt := verifyRuntimeFor(t, conf, "websecure", fallback)
			verifyDump(t, rt.Merged)
			for name := range rt.Merged.TLS.Options {
				t.Log("TLS option ", name)
			}

			for _, c := range []struct {
				sni, host string
				want      int
			}{
				{test.a, test.a, 200},
				{test.b, test.b, 200},
				{test.a, test.b, 421},
				{test.b, test.a, 421},
			} {
				code, body, opts := rt.Serve(c.sni, c.host, "/")
				t.Logf("REQUEST sni=%s host=%s -> %d body=%q tlsOptions=%q (want %d)", c.sni, c.host, code, body, opts, c.want)
				assert.Equal(t, c.want, code)
			}
		})
	}
}
