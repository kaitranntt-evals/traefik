//go:build verify

// Run (audited branch, after copying the eval fixtures from eval_tests.zip into pkg/provider/kubernetes/gateway/):
//   cp verify/repro/c2_c3_sibling_gateway_crossed_names_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -race -v -run '^TestVerify_C2C3' ./pkg/provider/kubernetes/gateway

package gateway_test

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// Gateway A owns a *.example.com HTTPS listener, sibling Gateway B owns a bar.example.com HTTPS
// listener on the same entry point. Both bar.example.com and foo.example.com select A's wildcard
// listener within A. The probe dumps the generated parent routers / TLS options and sends the
// crossed request (SNI bar.example.com, Host foo.example.com) to A's route through the production
// HTTPS handler chain built by the eval fixture helper evalServeHTTPS.
func TestVerify_C2C3_SiblingGatewayCrossedNames(t *testing.T) {
	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprintf("strictTLSOptions=%t", strict), func(t *testing.T) {
			secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
			eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("gw-a", "supersecret", "wildcard=*.example.com"),
				gateway.EvalIsoHTTPSGatewayExported("gw-b", "supersecret", "exact=bar.example.com"),
				gateway.EvalIsoRouteExported("route-a", "gw-a", "wildcard", "/a"),
				gateway.EvalIsoRouteExported("route-b", "gw-b", "exact", "/b"),
			)

			names := make([]string, 0, len(conf.HTTP.Routers))
			for name := range conf.HTTP.Routers {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				r := conf.HTTP.Routers[name]
				tlsOpts := "<nil>"
				if r.TLS != nil {
					tlsOpts = r.TLS.Options
				}
				t.Logf("ROUTER %s\n    rule=%s\n    parents=%v tls.options=%q service=%q", name, r.Rule, r.ParentRefs, tlsOpts, r.Service)
			}
			for name := range conf.TLS.Options {
				t.Logf("TLSOPTION %s", name)
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
				{"foo.example.com", "foo.example.com", "/a"}, // control: same name, A's route
				{"bar.example.com", "bar.example.com", "/b"}, // control: same name, B's route
				{"bar.example.com", "bar.example.com", "/a"}, // A's route via the bar.example.com partition
				{"bar.example.com", "foo.example.com", "/a"}, // THE CLAIM: crossed names, both select A's wildcard listener within A
				{"foo.example.com", "bar.example.com", "/a"}, // reverse crossing
			} {
				buf.Reset()
				code, body := serve(p.sni, p.host, p.path)
				var sniLines []string
				for _, line := range strings.Split(buf.String(), "\n") {
					if strings.Contains(line, "TLS options difference") {
						sniLines = append(sniLines, line)
					}
				}
				t.Logf("REQUEST SNI=%s Host=%s path=%s -> %d body=%q\n    snicheck-log=%v", p.sni, p.host, p.path, code, body, sniLines)
				if p.sni == "bar.example.com" && p.host == "foo.example.com" {
					if code == http.StatusMisdirectedRequest {
						t.Logf("CLAIM OBSERVED: crossed request rejected with 421")
					} else {
						t.Logf("CLAIM NOT OBSERVED: crossed request answered %d", code)
					}
				}
			}
		})
	}
}
