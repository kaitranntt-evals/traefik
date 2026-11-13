// Run (in a worktree of evalon/traefik-ga-42029a8c): cp verify/repro/c7_multihost_fallback_probe_test.go <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -v -run 'TestVerify_MultiHostFallback' ./pkg/provider/kubernetes/gateway
//go:build verify

package gateway_test

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// TestVerify_MultiHostFallback loads a Gateway HTTPRoute for foo+bar.example.com, a second HTTPRoute for
// bar.example.com only (same listener, hence same dedicated TLS options), and a file-provider router on
// foo.example.com, through the production ConfigurationWatcher (strict mode off) and router factory.
func TestVerify_MultiHostFallback(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	for _, opts := range []string{"", "custom"} {
		for _, withFile := range []bool{false, true} {
			name := fmt.Sprintf("fileRouter=%v/fileOptions=%s", withFile, map[string]string{"": "default", "custom": "custom"}[opts])
			t.Run(name, func(t *testing.T) {
				gatewayConf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
					evalIsoMultiBackendYAML, gateway.EvalIsoClassYAML,
					gateway.EvalIsoHTTPSGatewayExported("verify-gw", "supersecret", "wild=*.example.com", "other=other.test"),
					gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteWithServiceExported("http-app-mixed", "verify-gw", "wild", "/mixed", "whoami-precise"), "foo.example.com", "bar.example.com"),
					gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteWithServiceExported("http-app-baronly", "verify-gw", "wild", "/baronly", "whoami-catchall"), "bar.example.com"),
				)
				host := "foo.example.com"
				if !withFile {
					host = "unrelated.test"
				}
				fileConf := &dynamic.Configuration{
					TLS: &dynamic.TLSConfiguration{Options: map[string]traefiktls.Options{
						"default": {MinVersion: "VersionTLS13"},
						"custom":  {MinVersion: "VersionTLS12", MaxVersion: "VersionTLS12"},
					}},
					HTTP: &dynamic.HTTPConfiguration{
						Routers: map[string]*dynamic.Router{
							"ext": {Rule: "Host(`" + host + "`) && Path(`/other`)", EntryPoints: []string{"websecure"}, Service: "aeval-file-service", TLS: &dynamic.RouterTLSConfig{Options: opts}},
						},
						Services: map[string]*dynamic.Service{"aeval-file-service": {LoadBalancer: &dynamic.ServersLoadBalancer{
							Servers: []dynamic.Server{{URL: "http://" + evalTLSConflictFileBackend + ":8080"}},
						}}},
					},
				}
				rt := evalTLSConflictRuntimeFor(t, gatewayConf, nil, fileConf, "websecure", "foo.example.com", false)

				var names []string
				for n := range rt.Routers {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, n := range names {
					r := rt.Routers[n]
					if r.TLS == nil || !(strings.Contains(n, "http-app-") || strings.Contains(n, "ext@")) {
						continue
					}
					fmt.Printf("ROUTER %s configured=%q resolved=%q status=%s err=%v rule=%s\n", n, r.TLS.Options, r.TLS.ResolvedOptions, r.Status, r.Err, r.Rule)
				}
				for _, sni := range []string{"foo.example.com", "bar.example.com", "baz.example.com"} {
					cfg, optName, err := rt.TLSConfig(sni)
					minV := uint16(0)
					if cfg != nil {
						minV = cfg.MinVersion
					}
					fmt.Printf("SNI %s -> options=%q minVersion=%#x err=%v\n", sni, optName, minV, err)
				}
				for _, rq := range [][2]string{{"foo.example.com", "/mixed"}, {"foo.example.com", "/other"}, {"bar.example.com", "/mixed"}, {"bar.example.com", "/baronly"}} {
					res := rt.Serve(rq[0], rq[0], rq[1])
					fmt.Printf("RESULT %s GET https://%s%s -> %d (%s)\n", name, rq[0], rq[1], res.Code, strings.TrimSpace(res.Body))
				}
			})
		}
	}
}
