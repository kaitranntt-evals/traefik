//go:build verify

// C3 repro. Run (in a checkout of the claim branch, eval fixtures + verify_harness_test.go copied to pkg/provider/kubernetes/gateway/):
//   go test -tags verify -count=1 -v -run '^TestVerifyC3_' ./pkg/provider/kubernetes/gateway

package gateway_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

type verifyC3Req struct {
	sni, host, path string
	want            int
	wantBody        string
	note            string
}

func verifyC3Run(t *testing.T, listeners []string, routes []string, reqs []verifyC3Req) {
	t.Helper()
	verifyC3RunWith(t, false, listeners, routes, reqs)
}

// verifyC3DedicateTLSOptions gives every HTTPS router that carries no TLS options one TLS options set per host name it
// serves, for a revision whose Gateway provider cannot express per-listener TLS options itself.
func verifyC3DedicateTLSOptions(t *testing.T, conf *dynamic.Configuration) {
	t.Helper()
	if conf.TLS == nil {
		conf.TLS = &dynamic.TLSConfiguration{}
	}
	if conf.TLS.Options == nil {
		conf.TLS.Options = map[string]traefiktls.Options{}
	}
	for _, rt := range conf.HTTP.Routers {
		if rt.TLS == nil || rt.TLS.Options != "" || !strings.HasPrefix(rt.Rule, `Host("`) {
			continue
		}
		host := strings.SplitN(strings.TrimPrefix(rt.Rule, `Host("`), `"`, 2)[0]
		name := "dedicated-" + strings.NewReplacer("*", "wildcard", ".", "-").Replace(host)
		rt.TLS.Options = name
		conf.TLS.Options[name] = traefiktls.Options{MinVersion: "VersionTLS12"}
	}
}

func verifyC3RunWith(t *testing.T, dedicatedTLSOptions bool, listeners []string, routes []string, reqs []verifyC3Req) {
	t.Helper()

	secret := gateway.EvalIsoNamespacedTLSSecretExported(t, "default", "supersecret",
		"example.com", "*.example.com", "*.foo.example.com", "abc.foo.example.com")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	docs := []string{evalIsoMultiBackendYAML, gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", listeners...)}
	docs = append(docs, routes...)

	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret}, docs...)
	if dedicatedTLSOptions {
		verifyC3DedicateTLSOptions(t, conf)
	}

	names := make([]string, 0, len(conf.HTTP.Routers))
	for name := range conf.HTTP.Routers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rt := conf.HTTP.Routers[name]
		tlsOpts := "<no tls>"
		if rt.TLS != nil {
			tlsOpts = "tls.options=" + `"` + rt.TLS.Options + `"`
		}
		t.Logf("PROVIDER router %q\n      rule=%s\n      priority=%d service=%q parentRefs=%v %s", name, rt.Rule, rt.Priority, rt.Service, rt.ParentRefs, tlsOpts)
	}
	for name := range conf.TLS.Options {
		t.Logf("PROVIDER tls.options %q", name)
	}

	rt := verifyBuild(t, []dynamic.Message{{ProviderName: "kubernetesgateway", Configuration: conf}}, "websecure", false)
	realTLS := rt.StartRealTLS(t)

	for _, req := range reqs {
		code, body := rt.ServeInProcess(t, req.sni, req.host, req.path)
		rcode, rbody, err := realTLS(req.sni, req.host, req.path)
		require.NoError(t, err)
		t.Logf("REQUEST SNI=%-22s Host=%-22s path=%-7s -> in-process %d %q | real TLS %d %q   (%s; spec-expected %d)",
			req.sni, req.host, req.path, code, body, rcode, rbody, req.note, req.want)
		assert.Equal(t, req.want, code, "in-process: %s", req.note)
		assert.Equal(t, req.want, rcode, "real TLS: %s", req.note)
		if req.wantBody != "" {
			assert.Equal(t, req.wantBody, rbody, "real TLS backend: %s", req.note)
		}
	}
}

func verifyC3Routes() []string {
	return []string{
		gateway.EvalIsoRouteWithServiceExported("route-wild", "my-gateway", "wild", "/wild", "whoami-catchall"),
		gateway.EvalIsoRouteWithServiceExported("route-foo", "my-gateway", "foo", "/foo", "whoami"),
		gateway.EvalIsoRouteWithServiceExported("route-exact", "my-gateway", "exact", "/exact", "whoami-precise"),
	}
}

// The claim topology: *.example.com, *.foo.example.com, abc.foo.example.com, no catch-all listener.
func TestVerifyC3_ThreeListenersNoCatchAll(t *testing.T) {
	verifyC3Run(t,
		[]string{"wild=*.example.com", "foo=*.foo.example.com", "exact=abc.foo.example.com"},
		verifyC3Routes(),
		[]verifyC3Req{
			{"abc.foo.example.com", "abc.foo.example.com", "/exact", http.StatusOK, "10.10.1.1", "CLAIM: SNI == Host == exact listener hostname"},
			{"abc.foo.example.com", "abc.foo.example.com", "/nope", http.StatusNotFound, "", "same listener, unmatched path"},
			{"xyz.foo.example.com", "xyz.foo.example.com", "/foo", http.StatusOK, "10.10.0.1", "SNI == Host on *.foo.example.com listener"},
			{"xyz.example.com", "xyz.example.com", "/wild", http.StatusOK, "10.10.2.1", "SNI == Host on *.example.com listener"},
			{"xyz.example.com", "abc.foo.example.com", "/exact", http.StatusMisdirectedRequest, "", "crossed: SNI wild listener, Host exact listener"},
			{"abc.foo.example.com", "xyz.example.com", "/wild", http.StatusMisdirectedRequest, "", "crossed: SNI exact listener, Host wild listener"},
		})
}

// The claim topology again, with one dedicated TLS options set per listener host name injected into the provider
// output (the claim branch generates none; a no-op on a revision that already generates per-listener TLS options).
func TestVerifyC3_ThreeListenersNoCatchAll_DedicatedTLSOptions(t *testing.T) {
	verifyC3RunWith(t, true,
		[]string{"wild=*.example.com", "foo=*.foo.example.com", "exact=abc.foo.example.com"},
		verifyC3Routes(),
		[]verifyC3Req{
			{"abc.foo.example.com", "abc.foo.example.com", "/exact", http.StatusOK, "10.10.1.1", "CLAIM: SNI == Host == exact listener hostname"},
			{"xyz.foo.example.com", "xyz.foo.example.com", "/foo", http.StatusOK, "10.10.0.1", "SNI == Host on *.foo.example.com listener"},
			{"xyz.example.com", "xyz.example.com", "/wild", http.StatusOK, "10.10.2.1", "SNI == Host on *.example.com listener"},
		})
}

// Control: same exact listener and route, with only one wildcard sibling.
func TestVerifyC3_Control_TwoListeners_FooWildcard(t *testing.T) {
	verifyC3Run(t,
		[]string{"foo=*.foo.example.com", "exact=abc.foo.example.com"},
		verifyC3Routes()[1:],
		[]verifyC3Req{
			{"abc.foo.example.com", "abc.foo.example.com", "/exact", http.StatusOK, "10.10.1.1", "SNI == Host == exact listener hostname"},
			{"xyz.foo.example.com", "abc.foo.example.com", "/exact", http.StatusMisdirectedRequest, "", "crossed"},
		})
}

// Control: same exact listener and route, with only the outer wildcard sibling.
func TestVerifyC3_Control_TwoListeners_OuterWildcard(t *testing.T) {
	routes := verifyC3Routes()
	verifyC3Run(t,
		[]string{"wild=*.example.com", "exact=abc.foo.example.com"},
		[]string{routes[0], routes[2]},
		[]verifyC3Req{
			{"abc.foo.example.com", "abc.foo.example.com", "/exact", http.StatusOK, "10.10.1.1", "SNI == Host == exact listener hostname"},
			{"xyz.example.com", "abc.foo.example.com", "/exact", http.StatusMisdirectedRequest, "", "crossed"},
		})
}

// Control: the claim topology plus a catch-all listener.
func TestVerifyC3_Control_ThreeListenersPlusCatchAll(t *testing.T) {
	verifyC3Run(t,
		[]string{"wild=*.example.com", "foo=*.foo.example.com", "exact=abc.foo.example.com", "all="},
		verifyC3Routes(),
		[]verifyC3Req{
			{"abc.foo.example.com", "abc.foo.example.com", "/exact", http.StatusOK, "10.10.1.1", "SNI == Host == exact listener hostname"},
		})
}
