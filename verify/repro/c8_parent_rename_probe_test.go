// Run (in a worktree of evalon/traefik-ga-ef258f77): cp verify/repro/c8_parent_rename_probe_test.go <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -v -run 'TestVerify_ParentRename' ./pkg/provider/kubernetes/gateway
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

// TestVerify_ParentRename loads a file-provider TLS parent (named options) with a child referring to it,
// alongside a Gateway HTTPS route for the same host and entry point, through the production
// ConfigurationWatcher (strict mode off) and router factory (which runs ParseRouterTree).
// The control keeps the external tree on another host, so no fallback is triggered.
func TestVerify_ParentRename(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	for _, extHost := range []string{"unrelated.test", "foo.example.com"} {
		name := "control_no_overlap"
		if extHost == "foo.example.com" {
			name = "overlap_triggers_fallback"
		}
		t.Run(name, func(t *testing.T) {
			gatewayConf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				evalIsoMultiBackendYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("verify-gw", "supersecret", "exact=foo.example.com", "other=other.test"),
				gateway.EvalIsoRouteWithServiceExported("http-app-gw", "verify-gw", "exact", "/gw", "whoami-precise"),
			)
			fileConf := &dynamic.Configuration{
				TLS: &dynamic.TLSConfiguration{Options: map[string]traefiktls.Options{
					"custom": {MinVersion: "VersionTLS12", MaxVersion: "VersionTLS12"},
				}},
				HTTP: &dynamic.HTTPConfiguration{
					Routers: map[string]*dynamic.Router{
						"ext-parent": {Rule: "Host(`" + extHost + "`)", EntryPoints: []string{"websecure"}, TLS: &dynamic.RouterTLSConfig{Options: "custom"}},
						"ext-child":  {Rule: "Path(`/other`)", ParentRefs: []string{"ext-parent"}, Service: "aeval-file-service"},
					},
					Services: map[string]*dynamic.Service{"aeval-file-service": {LoadBalancer: &dynamic.ServersLoadBalancer{
						Servers: []dynamic.Server{{URL: "http://" + evalTLSConflictFileBackend + ":8080"}},
					}}},
				},
			}
			rt := evalTLSConflictRuntimeFor(t, gatewayConf, nil, fileConf, "websecure", extHost, false)

			var names []string
			for n := range rt.Routers {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				r := rt.Routers[n]
				if !(strings.Contains(n, "http-app-") || strings.Contains(n, "@file")) {
					continue
				}
				tlsOpt := "-"
				if r.TLS != nil {
					tlsOpt = r.TLS.ResolvedOptions
				}
				fmt.Printf("ROUTER %s resolvedTLS=%s parentRefs=%v childRefs=%v status=%s err=%v rule=%s\n", n, tlsOpt, r.ParentRefs, r.ChildRefs, r.Status, r.Err, r.Rule)
			}
			_, optName, err := rt.TLSConfig(extHost)
			ext := rt.Serve(extHost, extHost, "/other")
			gw := rt.Serve("foo.example.com", "foo.example.com", "/gw")
			fmt.Printf("RESULT %s sniOptions(%s)=%q err=%v GET https://%s/other=%d(%s) GET https://foo.example.com/gw=%d(%s)\n",
				name, extHost, optName, err, extHost, ext.Code, strings.TrimSpace(ext.Body), gw.Code, strings.TrimSpace(gw.Body))
		})
	}
}
