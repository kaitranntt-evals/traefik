// Run (after copying the verify_*_test.go files of verify/repro/ into pkg/provider/kubernetes/gateway/):
//
//	go test -count=1 -v -run '^TestVerifyHTTPSIdentityBesidePassthrough$' ./pkg/provider/kubernetes/gateway
package gateway_test

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

func TestVerifyHTTPSIdentityBesidePassthrough(t *testing.T) {
	for _, test := range []struct {
		desc        string
		broad       string
		broadHost   string
		narrow      string
		narrowHost  string
		passthrough string
	}{
		{desc: "wildcard vs exact, control without TLS listener", broad: "*.example.com", broadHost: "app.example.com", narrow: "foo.example.com", narrowHost: "foo.example.com"},
		{desc: "wildcard vs exact, with overlapping TLS passthrough listener", broad: "*.example.com", broadHost: "app.example.com", narrow: "foo.example.com", narrowHost: "foo.example.com", passthrough: "db.example.com"},
		{desc: "catch-all vs exact, control without TLS listener", broad: "", broadHost: "outside.test", narrow: "foo.example.com", narrowHost: "foo.example.com"},
		{desc: "catch-all vs exact, with overlapping TLS passthrough listener", broad: "", broadHost: "outside.test", narrow: "foo.example.com", narrowHost: "foo.example.com", passthrough: "db.example.com"},
		{desc: "catch-all vs wildcard, control without TLS listener", broad: "", broadHost: "outside.test", narrow: "*.example.com", narrowHost: "a.example.com"},
		{desc: "catch-all vs wildcard, with overlapping TLS passthrough listener", broad: "", broadHost: "outside.test", narrow: "*.example.com", narrowHost: "a.example.com", passthrough: "db.example.com"},
		{desc: "wildcard vs nested wildcard, control without TLS listener", broad: "*.example.com", broadHost: "app.example.com", narrow: "*.sub.example.com", narrowHost: "app.sub.example.com"},
		{desc: "wildcard vs nested wildcard, with overlapping TLS passthrough listener", broad: "*.example.com", broadHost: "app.example.com", narrow: "*.sub.example.com", narrowHost: "app.sub.example.com", passthrough: "db.sub.example.com"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			listeners := []string{verifyHTTPSListenerYAML("broad", test.broad), verifyHTTPSListenerYAML("exact", test.narrow)}
			docs := []string{
				verifyClassYAML, verifyServiceYAML("broad", "10.10.0.1", 80), verifyServiceYAML("exact", "10.10.0.2", 80), verifyServiceYAML("db", "10.10.0.3", 443),
				verifyHTTPRouteYAML("broad", "broad", "/", "broad"), verifyHTTPRouteYAML("exact", "exact", "/", "exact"),
			}
			if test.passthrough != "" {
				listeners = append(listeners, verifyTLSListenerYAML("db", test.passthrough))
				docs = append(docs, verifyTLSRouteYAML("db", "db", test.passthrough, "db", 443))
			}
			docs = append(docs, verifyGatewayYAML(listeners...))

			conf, status := gateway.VerifyLoad(t, map[string]gateway.Entrypoint{"websecure": {Address: ":443"}},
				[]k8sruntime.Object{gateway.VerifySecret(t, "gw-cert", "gateway-listener-cert", "*.example.com", "*.sub.example.com")}, docs...)
			for _, s := range status {
				t.Log("STATUS ", s)
			}

			certPEM, keyPEM := gateway.VerifyCert(t, "traefik-default-fallback")
			fallback, err := tls.X509KeyPair(certPEM, keyPEM)
			require.NoError(t, err)
			rt := verifyRuntimeFor(t, conf, "websecure", fallback)
			verifyDump(t, rt.Merged)

			for _, c := range []struct {
				sni, host string
				want      int
			}{
				{test.broadHost, test.broadHost, 200},
				{test.narrowHost, test.narrowHost, 200},
				{test.broadHost, test.narrowHost, 421},
				{test.narrowHost, test.broadHost, 421},
			} {
				code, body, opts := rt.Serve(c.sni, c.host, "/")
				t.Logf("REQUEST sni=%s host=%s -> %d body=%q tlsOptions=%q (want %d)", c.sni, c.host, code, body, opts, c.want)
				assert.Equal(t, c.want, code)
			}
		})
	}
}
