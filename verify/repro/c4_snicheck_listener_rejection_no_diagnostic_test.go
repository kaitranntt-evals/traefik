//go:build verify

// Run on branch evalon/traefik-ga-7b8831c5 (kaitranntt-evals/traefik-gateway-listener-isolation), after copying the eval
// fixtures from eval_tests.zip into pkg/provider/kubernetes/gateway/:
//   cp verify/repro/c4_snicheck_listener_rejection_no_diagnostic_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -race -v -run '^TestVerify_C4' ./pkg/provider/kubernetes/gateway

package gateway_test

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// One Gateway with a catch-all HTTPS listener and an exact abc.example.com HTTPS listener.
// A request whose SNI selects the exact listener and whose Host selects the catch-all listener
// is a listener misdirection. The probe sends it through the production HTTPS handler chain
// (evalServeHTTPS) with the global zerolog logger captured at debug level, and prints every log
// line emitted while the request was served.
func TestVerify_C4_ListenerMisdirectionDiagnostic(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprintf("strictTLSOptions=%t", strict), func(t *testing.T) {
			secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
			eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "catchall=", "precise=abc.example.com"),
				gateway.EvalIsoRouteExported("http-app-catchall", "my-gateway", "catchall", "/catchall"),
				gateway.EvalIsoRouteExported("http-app-precise", "my-gateway", "precise", "/precise"),
			)
			for name, r := range conf.HTTP.Routers {
				tlsInfo := "<nil>"
				if r.TLS != nil {
					tlsInfo = fmt.Sprintf("%+v", *r.TLS)
				}
				t.Logf("ROUTER %s rule=%s tls=%s", name, r.Rule, tlsInfo)
			}

			var buf bytes.Buffer
			oldLogger, oldLevel := log.Logger, zerolog.GlobalLevel()
			log.Logger = zerolog.New(&buf)
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
			t.Cleanup(func() {
				log.Logger = oldLogger
				zerolog.SetGlobalLevel(oldLevel)
			})

			serve := evalServeHTTPS(t, conf, "websecure", strict)

			type probe struct {
				sni, host, path string
			}
			for _, p := range []probe{
				{"abc.example.com", "abc.example.com", "/precise"},  // control: same listener
				{"abc.example.com", "xyz.example.com", "/catchall"}, // misdirection: SNI exact, Host catch-all
				{"xyz.example.com", "abc.example.com", "/precise"},  // misdirection: SNI catch-all, Host exact
				{"xyz.example.com", "abc.example.com", "/nope"},     // misdirection on unmatched path (fallback router)
			} {
				buf.Reset()
				code, body := serve(p.sni, p.host, p.path)
				logged := strings.TrimSpace(buf.String())
				hasAll := strings.Contains(logged, "routerName") && strings.Contains(logged, "req.Host") && strings.Contains(logged, "req.TLS.ServerName")
				t.Logf("REQUEST SNI=%s Host=%s path=%s -> %d body=%q\n    log-lines=%d diagnostic-with-routerName+req.Host+req.TLS.ServerName=%t\n    log=%q", p.sni, p.host, p.path, code, body, strings.Count(logged, "\n")+boolToInt(logged != ""), hasAll, logged)
				if code == http.StatusMisdirectedRequest && !hasAll {
					t.Logf("CLAIM OBSERVED: 421 without a diagnostic carrying routerName, req.Host and req.TLS.ServerName")
				}
			}
		})
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
