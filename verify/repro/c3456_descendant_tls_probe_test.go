// Run: cp verify/repro/c3456_descendant_tls_probe_test.go pkg/provider/kubernetes/gateway/ && cp <eval_tests>/tests/*.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -v -run 'TestVerify_DescendantTLS' ./pkg/provider/kubernetes/gateway
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

// TestVerify_DescendantTLS drives the Gateway provider output plus a file-provider router tree
// through the production ConfigurationWatcher (strict mode off) and prints the resolved routers,
// the TLS options selected per SNI and the HTTPS responses.
func TestVerify_DescendantTLS(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	fileConfWith := func(routers map[string]*dynamic.Router) *dynamic.Configuration {
		return &dynamic.Configuration{
			TLS: &dynamic.TLSConfiguration{Options: map[string]traefiktls.Options{
				"default": {MinVersion: "VersionTLS13"},
				"custom":  {MinVersion: "VersionTLS12", MaxVersion: "VersionTLS12"},
			}},
			HTTP: &dynamic.HTTPConfiguration{
				Routers: routers,
				Services: map[string]*dynamic.Service{"aeval-file-service": {LoadBalancer: &dynamic.ServersLoadBalancer{
					Servers: []dynamic.Server{{URL: "http://" + evalTLSConflictFileBackend + ":8080"}},
				}}},
			},
		}
	}

	trees := []struct {
		name    string
		routers func(opts string) map[string]*dynamic.Router
	}{
		{"control_child_exact_host", func(opts string) map[string]*dynamic.Router {
			return map[string]*dynamic.Router{
				"ext-root":  {Rule: "PathPrefix(`/other`)", Priority: 100, EntryPoints: []string{"websecure"}, TLS: &dynamic.RouterTLSConfig{Options: opts}},
				"ext-child": {Rule: "Host(`foo.example.com`)", ParentRefs: []string{"ext-root"}, Service: "aeval-file-service"},
			}
		}},
		{"grandchild_exact_host", func(opts string) map[string]*dynamic.Router {
			return map[string]*dynamic.Router{
				"ext-root":       {Rule: "PathPrefix(`/other`)", Priority: 100, EntryPoints: []string{"websecure"}, TLS: &dynamic.RouterTLSConfig{Options: opts}},
				"ext-mid":        {Rule: "Method(`GET`)", ParentRefs: []string{"ext-root"}},
				"ext-grandchild": {Rule: "Host(`foo.example.com`)", ParentRefs: []string{"ext-mid"}, Service: "aeval-file-service"},
			}
		}},
		{"child_wildcard_host", func(opts string) map[string]*dynamic.Router {
			return map[string]*dynamic.Router{
				"ext-root":  {Rule: "PathPrefix(`/other`)", Priority: 100, EntryPoints: []string{"websecure"}, TLS: &dynamic.RouterTLSConfig{Options: opts}},
				"ext-child": {Rule: "Host(`*.example.com`)", ParentRefs: []string{"ext-root"}, Service: "aeval-file-service"},
			}
		}},
	}

	listeners := []struct{ desc, spec string }{
		{"exact", "broad=foo.example.com"},
		{"wildcard", "broad=*.example.com"},
		{"catchall", "broad="},
		{"disjoint_control", "broad=other.test"},
	}

	for _, tree := range trees {
		for _, opts := range []string{"", "custom"} {
			for _, l := range listeners {
				name := fmt.Sprintf("%s/extopts=%s/listener=%s", tree.name, map[string]string{"": "default", "custom": "custom"}[opts], l.desc)
				t.Run(name, func(t *testing.T) {
					gwDoc := gateway.EvalIsoHTTPSGatewayExported("verify-gw", "supersecret", l.spec)
					gatewayConf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
						evalIsoMultiBackendYAML, gateway.EvalIsoClassYAML, gwDoc,
						gateway.EvalIsoRouteWithServiceExported("http-app-gw", "verify-gw", "broad", "/gw", "whoami-precise"),
					)
					rt := evalTLSConflictRuntimeFor(t, gatewayConf, nil, fileConfWith(tree.routers(opts)), "websecure", "foo.example.com", false)

					var names []string
					for n := range rt.Routers {
						names = append(names, n)
					}
					sort.Strings(names)
					for _, n := range names {
						r := rt.Routers[n]
						tlsOpt := "-"
						if r.TLS != nil {
							tlsOpt = r.TLS.ResolvedOptions
						}
						fmt.Printf("ROUTER %-62s resolvedTLS=%-34s parents=%v status=%s err=%v rule=%s\n", n, tlsOpt, r.ParentRefs, r.Status, r.Err, r.Rule)
					}
					fallback := false
					for n, r := range rt.Routers {
						if strings.HasSuffix(n, "@kubernetesgateway") && r.TLS != nil && strings.Contains(n, "-conflicted-") && r.TLS.ResolvedOptions == "default" {
							fallback = true
						}
					}
					cfg, optName, err := rt.TLSConfig("foo.example.com")
					minV := uint16(0)
					if cfg != nil {
						minV = cfg.MinVersion
					}
					gw := rt.Serve("foo.example.com", "foo.example.com", "/gw")
					ext := rt.Serve("foo.example.com", "foo.example.com", "/other")
					fmt.Printf("RESULT %s gatewayFallback=%v sniOptions=%q minVersion=%#x err=%v GET/gw=%d(%s) GET/other=%d(%s)\n",
						name, fallback, optName, minV, err, gw.Code, strings.TrimSpace(gw.Body), ext.Code, strings.TrimSpace(ext.Body))
				})
			}
		}
	}
}
