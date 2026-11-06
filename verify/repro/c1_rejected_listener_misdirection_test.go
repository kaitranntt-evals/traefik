// Run (C1, branch evalon/traefik-ga-ee75c112 of kaitranntt-evals/traefik-gateway-listener-isolation): copy the eval fixtures (tests/*.go), c1_export_test.go and this file into pkg/provider/kubernetes/gateway/, then `go test -race -count=1 -timeout=5m -run "^TestVerifyC1_" -v ./pkg/provider/kubernetes/gateway`.
package gateway_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// verifyC1Gateway renders an HTTPS Gateway on port 443 whose listeners each name their own secret ("name=hostname=secret").
func verifyC1Gateway(listeners ...string) string {
	var b strings.Builder
	b.WriteString(`
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: my-gateway
  namespace: default
spec:
  gatewayClassName: my-gateway-class
  listeners:
`)
	for _, l := range listeners {
		parts := strings.SplitN(l, "=", 3)
		b.WriteString("    - name: " + parts[0] + "\n      protocol: HTTPS\n      port: 443\n")
		if parts[1] != "" {
			b.WriteString("      hostname: \"" + parts[1] + "\"\n")
		}
		b.WriteString("      tls:\n        certificateRefs:\n          - kind: Secret\n            name: " + parts[2] + "\n            group: \"\"\n")
		b.WriteString("      allowedRoutes:\n        namespaces:\n          from: Same\n")
	}
	return b.String()
}

func TestVerifyC1_RejectedListenerMisdirection(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	load := func(t *testing.T, gw string) (map[string][]string, func(sni, host, path string, extra ...map[string]string) (int, string)) {
		t.Helper()
		conf, report := gateway.EvalIsoLoadWithStatusExported(t, eps, []k8sruntime.Object{secret},
			gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML, gw,
			gateway.EvalIsoRouteExported("http-app-catchall", "my-gateway", "catchall", "/catchall"),
		)
		for _, l := range report.ListenersExported() {
			t.Logf("LISTENER %s hostname=%q Attached=%t", l.Name, l.HostnameExported(), l.Attached)
		}
		mws := map[string][]string{}
		for name, mw := range conf.HTTP.Middlewares {
			if mw.MisdirectedRequest != nil {
				hs := append([]string(nil), mw.MisdirectedRequest.Hostnames...)
				sort.Strings(hs)
				mws[name] = hs
				t.Logf("MIDDLEWARE %s misdirectedRequest.hostnames=%q", name, hs)
			}
		}
		for name, r := range conf.HTTP.Routers {
			t.Logf("ROUTER %s rule=%s middlewares=%v service=%s", name, r.Rule, r.Middlewares, r.Service)
		}
		return mws, evalServeHTTPS(t, conf, "websecure", false)
	}

	t.Run("baseline_only_catchall", func(t *testing.T) {
		mws, serve := load(t, verifyC1Gateway("catchall==supersecret"))
		assert.Empty(t, mws, "no misdirected-request middleware with a single HTTPS listener")
		code, body := serve("abc.example.com", "xyz.example.com", "/catchall")
		t.Logf("SNI=abc.example.com Host=xyz.example.com /catchall -> %d %q", code, body)
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("rejected_exact_listener_missing_secret", func(t *testing.T) {
		mws, serve := load(t, verifyC1Gateway("catchall==supersecret", "precise=abc.example.com=missing-secret"))
		require.Len(t, mws, 1)
		for name, hs := range mws {
			// Part 1: the rejected (Attached == false) listener hostname takes part in misdirection detection.
			assert.Equal(t, []string{"", "abc.example.com"}, hs, name)
		}
		// Part 2: SNI selects the rejected listener, Host selects the attached catch-all.
		code, body := serve("abc.example.com", "xyz.example.com", "/catchall")
		t.Logf("SNI=abc.example.com Host=xyz.example.com /catchall -> %d %q", code, body)
		assert.Equal(t, http.StatusMisdirectedRequest, code)
		code, body = serve("xyz.example.com", "xyz.example.com", "/catchall")
		t.Logf("SNI=xyz.example.com Host=xyz.example.com /catchall -> %d %q", code, body)
		assert.Equal(t, http.StatusOK, code)
		code, body = serve("abc.example.com", "abc.example.com", "/catchall")
		t.Logf("SNI=abc.example.com Host=abc.example.com /catchall -> %d %q", code, body)
		code, body = serve("", "xyz.example.com", "/catchall")
		t.Logf("SNI=<empty> Host=xyz.example.com /catchall -> %d %q", code, body)
	})

	t.Run("control_accepted_sibling_without_rejected_listener", func(t *testing.T) {
		mws, serve := load(t, verifyC1Gateway("catchall==supersecret", "other=def.example.com=supersecret"))
		require.Len(t, mws, 1)
		for name, hs := range mws {
			assert.Equal(t, []string{"", "def.example.com"}, hs, name)
		}
		code, body := serve("abc.example.com", "xyz.example.com", "/catchall")
		t.Logf("SNI=abc.example.com Host=xyz.example.com /catchall -> %d %q", code, body)
		assert.Equal(t, http.StatusOK, code)
	})

	t.Run("accepted_sibling_plus_rejected_listener", func(t *testing.T) {
		mws, serve := load(t, verifyC1Gateway("catchall==supersecret", "other=def.example.com=supersecret", "precise=abc.example.com=missing-secret"))
		require.Len(t, mws, 1)
		for name, hs := range mws {
			assert.Equal(t, []string{"", "abc.example.com", "def.example.com"}, hs, name)
		}
		code, body := serve("abc.example.com", "xyz.example.com", "/catchall")
		t.Logf("SNI=abc.example.com Host=xyz.example.com /catchall -> %d %q", code, body)
		assert.Equal(t, http.StatusMisdirectedRequest, code)
	})
}
