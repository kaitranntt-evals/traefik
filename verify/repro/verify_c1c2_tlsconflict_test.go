//go:build verify

// C1/C2 repro. Run (eval fixtures + verify_harness_test.go copied to pkg/provider/kubernetes/gateway/):
//   go test -tags verify -count=1 -v -run '^TestVerifyC1C2_' ./pkg/provider/kubernetes/gateway

package gateway_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// verifyOtherProviderConf is what a second provider (here named "file") would emit: one HTTPS
// router on the same entry point, for the given host, with the given TLS options ("" = default).
func verifyOtherProviderConf(host, tlsOptions string) *dynamic.Configuration {
	conf := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"other-app": {
					EntryPoints: []string{"websecure"},
					Rule:        `Host("` + host + `") && PathPrefix("/file")`,
					Service:     "other-svc",
					TLS:         &dynamic.RouterTLSConfig{Options: tlsOptions},
				},
			},
			Services: map[string]*dynamic.Service{
				"other-svc": {LoadBalancer: &dynamic.ServersLoadBalancer{Servers: []dynamic.Server{{URL: "http://10.99.0.1:80"}}}},
			},
		},
	}
	if tlsOptions != "" {
		conf.TLS = &dynamic.TLSConfiguration{Options: map[string]traefiktls.Options{tlsOptions: {MinVersion: "VersionTLS12"}}}
	}
	return conf
}

// verifyGatewayConf loads, through the provider, one Gateway with two HTTPS listeners
// (listenerHostname, abc.example.com by default, and xyz.example.com), an HTTPRoute and a GRPCRoute on the first, an HTTPRoute on the second.
func verifyGatewayConf(t *testing.T, listenerHostname string) *dynamic.Configuration {
	t.Helper()
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
		evalIsoMultiBackendYAML, gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "precise="+listenerHostname, "sibling=xyz.example.com"),
		gateway.EvalIsoRouteWithServiceExported("http-app-precise", "my-gateway", "precise", "/gw", "whoami-precise"),
		gateway.EvalIsoRouteWithServiceExported("http-app-sibling", "my-gateway", "sibling", "/gw", "whoami-catchall"),
		gateway.EvalIsoGRPCRouteExported("grpc-app-precise", "my-gateway", "precise", "SayHello"),
	)

	names := make([]string, 0, len(conf.HTTP.Routers))
	for name := range conf.HTTP.Routers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rt := conf.HTTP.Routers[name]
		tlsOpts := "tls=nil"
		if rt.TLS != nil {
			tlsOpts = `tls.options="` + rt.TLS.Options + `"`
		}
		t.Logf("PROVIDER(kubernetesgateway) router %q\n      rule=%s eps=%v service=%q parentRefs=%v %s", name, rt.Rule, rt.EntryPoints, rt.Service, rt.ParentRefs, tlsOpts)
	}
	return conf
}

type verifyC1Case struct {
	name string
	// other is the second provider configuration; nil means the Gateway provider is alone.
	other  *dynamic.Configuration
	strict bool
	// listenerHostname is the hostname of the listener the probed routes attach to (default abc.example.com).
	listenerHostname string
}

func verifyC1C2Run(t *testing.T, tc verifyC1Case) {
	t.Helper()

	if tc.listenerHostname == "" {
		tc.listenerHostname = "abc.example.com"
	}
	msgs := []dynamic.Message{{ProviderName: "kubernetesgateway", Configuration: verifyGatewayConf(t, tc.listenerHostname)}}
	if tc.other != nil {
		msgs = append(msgs, dynamic.Message{ProviderName: "file", Configuration: tc.other})
	}

	rt := verifyBuild(t, msgs, "websecure", tc.strict)

	dangling, resolved := rt.DumpAgg(t)
	t.Logf("REFERENCE-INTEGRITY child ParentRefs after aggregation: resolved=%d dangling=%d", resolved, dangling)
	disabled := rt.DumpRuntime(t)
	t.Logf("ROUTE-AVAILABILITY child routers disabled by ParseRouterTree: %d", disabled)

	realTLS := rt.StartRealTLS(t)
	grpc := map[string]string{":method": http.MethodPost, "Content-Type": "application/grpc"}

	reqs := []struct {
		sni, host, path string
		extra           map[string]string
		want            int
		wantBody        string
		note            string
	}{
		{"abc.example.com", "abc.example.com", "/gw", nil, http.StatusOK, "10.10.1.1", "Gateway HTTPRoute on the conflicting listener"},
		{"abc.example.com", "abc.example.com", "/helloworld.Greeter/SayHello", grpc, http.StatusOK, "10.10.0.1", "Gateway GRPCRoute on the conflicting listener"},
		{"xyz.example.com", "xyz.example.com", "/gw", nil, http.StatusOK, "10.10.2.1", "Gateway HTTPRoute on the sibling listener (no conflict)"},
	}
	for _, req := range reqs {
		code, body := rt.ServeInProcess(t, req.sni, req.host, req.path, req.extra)
		rcode, rbody, err := realTLS(req.sni, req.host, req.path, req.extra)
		errStr := ""
		if err != nil {
			errStr = " err=" + err.Error()
		}
		t.Logf("REQUEST SNI=%s Host=%s path=%-28s -> in-process %d %q | real TLS %d %q%s   (%s)",
			req.sni, req.host, req.path, code, strings.TrimSpace(body), rcode, rbody, errStr, req.note)
		assert.Equal(t, req.want, code, "in-process: %s", req.note)
		assert.Equal(t, req.want, rcode, "real TLS: %s", req.note)
		assert.Equal(t, req.wantBody, rbody, "real TLS backend: %s", req.note)
	}

	if tc.other != nil {
		host := "abc.example.com"
		if strings.Contains(tc.other.HTTP.Routers["other-app"].Rule, "unrelated") {
			host = "unrelated.example.com"
		}
		code, body := rt.ServeInProcess(t, host, host, "/file")
		rcode, rbody, _ := realTLS(host, host, "/file")
		t.Logf("REQUEST SNI=%s Host=%s path=%-28s -> in-process %d %q | real TLS %d %q   (other provider's router)",
			host, host, "/file", code, strings.TrimSpace(body), rcode, rbody)
	}

	assert.Zero(t, dangling, "child ParentRefs must resolve after aggregation")
	assert.Zero(t, disabled, "ParseRouterTree must not disable Gateway child routers")
}

// Controls: no second provider, and a second provider that serves another host (no TLS options conflict).
func TestVerifyC1C2_Control_GatewayAlone(t *testing.T) {
	verifyC1C2Run(t, verifyC1Case{name: "gateway alone"})
}

func TestVerifyC1C2_Control_OtherProviderDifferentHost(t *testing.T) {
	verifyC1C2Run(t, verifyC1Case{other: verifyOtherProviderConf("unrelated.example.com", "")})
}

// The claimed situation: another provider serves the listener's host on the same entry point,
// strictTLSOptions disabled (the default).
func TestVerifyC1C2_NonStrict_OtherProviderSameHost_DefaultTLSOptions(t *testing.T) {
	verifyC1C2Run(t, verifyC1Case{other: verifyOtherProviderConf("abc.example.com", "")})
}

func TestVerifyC1C2_NonStrict_OtherProviderSameHost_CustomTLSOptions(t *testing.T) {
	verifyC1C2Run(t, verifyC1Case{other: verifyOtherProviderConf("abc.example.com", "modern")})
}

// Adjacent observation, outside the claims: a wildcard listener and another provider serving a host under that wildcard
// (no conflict is detected by resolveHTTPTLSOptions, no ParentRef dangles, the failure mode is different).
func TestVerifyC1C2_Adjacent_WildcardListener_OtherProviderHostUnderWildcard(t *testing.T) {
	verifyC1C2Run(t, verifyC1Case{listenerHostname: "*.example.com", other: verifyOtherProviderConf("abc.example.com", "")})
}

// Scope: the other provider's router points at the TLS options the Gateway provider generated for the listener.
func TestVerifyC1C2_Scope_OtherProviderReusesListenerTLSOptions(t *testing.T) {
	gw := verifyGatewayConf(t, "abc.example.com")
	var listenerOptions string
	for name, rt := range gw.HTTP.Routers {
		if rt.TLS != nil && strings.Contains(name, "abc-example-com") {
			listenerOptions = rt.TLS.Options + "@kubernetesgateway"
		}
	}
	if listenerOptions == "" {
		t.Skip("this revision generates no per-listener TLS options")
	}
	other := verifyOtherProviderConf("abc.example.com", "")
	other.HTTP.Routers["other-app"].TLS.Options = listenerOptions
	verifyC1C2Run(t, verifyC1Case{other: other})
}
