// Run (after copying the verify_*_test.go files of verify/repro/ into pkg/provider/kubernetes/gateway/):
//
//	go test -count=1 -v -run '^TestVerifyPassthroughBesideHTTPSWildcard$' ./pkg/provider/kubernetes/gateway
package gateway_test

import (
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

const verifyClassYAML = `
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: my-gateway-class
spec:
  controllerName: traefik.io/gateway-controller
`

func verifyServiceYAML(name, addr string, port int) string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Service
metadata:
  name: %[1]s
  namespace: default
spec:
  clusterIP: 10.10.10.1
  ports:
    - name: web
      protocol: TCP
      port: %[3]d
      targetPort: web
---
kind: EndpointSlice
apiVersion: discovery.k8s.io/v1
metadata:
  name: %[1]s-slice
  namespace: default
  labels:
    kubernetes.io/service-name: %[1]s
addressType: IPv4
ports:
  - name: web
    port: %[3]d
endpoints:
  - addresses:
      - %[2]s
    conditions:
      ready: true
`, name, addr, port)
}

func verifyHTTPSListenerYAML(name, hostname string) string {
	l := "    - name: " + name + "\n      protocol: HTTPS\n      port: 443\n"
	if hostname != "" {
		l += "      hostname: \"" + hostname + "\"\n"
	}
	return l + "      tls:\n        certificateRefs:\n          - kind: Secret\n            name: gw-cert\n            group: \"\"\n      allowedRoutes:\n        namespaces:\n          from: Same\n"
}

func verifyTLSListenerYAML(name, hostname string) string {
	return "    - name: " + name + "\n      protocol: TLS\n      port: 443\n      hostname: \"" + hostname + "\"\n      tls:\n        mode: Passthrough\n      allowedRoutes:\n        namespaces:\n          from: Same\n"
}

func verifyGatewayYAML(listeners ...string) string {
	return "kind: Gateway\napiVersion: gateway.networking.k8s.io/v1\nmetadata:\n  name: gw\n  namespace: default\nspec:\n  gatewayClassName: my-gateway-class\n  listeners:\n" + strings.Join(listeners, "")
}

func verifyHTTPRouteYAML(name, section, path, svc string) string {
	return fmt.Sprintf(`
kind: HTTPRoute
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: %[1]s
  namespace: default
spec:
  parentRefs:
    - name: gw
      sectionName: %[2]s
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: %[3]s
      backendRefs:
        - name: %[4]s
          port: 80
`, name, section, path, svc)
}

func verifyTLSRouteYAML(name, section, hostname, svc string, port int) string {
	return fmt.Sprintf(`
kind: TLSRoute
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: %[1]s
  namespace: default
spec:
  parentRefs:
    - name: gw
      sectionName: %[2]s
  hostnames:
    - %[3]s
  rules:
    - backendRefs:
        - name: %[4]s
          port: %[5]d
`, name, section, hostname, svc, port)
}

// verifyPassthroughBackend is a real TLS server standing for the workload behind the TLSRoute.
func verifyPassthroughBackend(t *testing.T) int {
	t.Helper()
	certPEM, keyPEM := gateway.VerifyCert(t, "passthrough-backend", "db.example.com")
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				_, _ = c.Write([]byte("PASSTHROUGH-BACKEND"))
			}()
		}
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	p, err := strconv.Atoi(port)
	require.NoError(t, err)
	return p
}

func TestVerifyPassthroughBesideHTTPSWildcard(t *testing.T) {
	for _, test := range []struct {
		desc          string
		httpsListener string
	}{
		{desc: "control: TLS passthrough listener alone"},
		{desc: "HTTPS wildcard listener beside the TLS passthrough listener", httpsListener: "*.example.com"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			port := verifyPassthroughBackend(t)

			listeners := []string{verifyTLSListenerYAML("db", "db.example.com")}
			docs := []string{verifyClassYAML, verifyServiceYAML("db", "127.0.0.1", port), verifyServiceYAML("whoami", "10.10.0.1", 80), verifyTLSRouteYAML("db", "db", "db.example.com", "db", port)}
			if test.httpsListener != "" {
				listeners = append([]string{verifyHTTPSListenerYAML("web", test.httpsListener)}, listeners...)
				docs = append(docs, verifyHTTPRouteYAML("web", "web", "/", "whoami"))
			}
			docs = append(docs, verifyGatewayYAML(listeners...))

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

			for _, sni := range []string{"db.example.com", "app.example.com"} {
				cn, payload, err := rt.DialTLS(sni)
				t.Logf("DISPATCH sni=%s -> peerCertCN=%q payload=%q err=%v", sni, cn, payload, err)
				if sni == "db.example.com" {
					assert.Equal(t, "passthrough-backend", cn)
					assert.Equal(t, "PASSTHROUGH-BACKEND", payload)
				}
			}
		})
	}
}
