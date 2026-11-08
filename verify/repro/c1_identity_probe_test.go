//go:build verify

// Run: cp verify/repro/c1_identity_probe_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -v -run 'TestVerifyC1' ./pkg/provider/kubernetes/gateway/
// (on branches evalon/traefik-ga-0c78c3fa and evalon/traefik-ga-6aded2e6). Loads the branch's own
// httproute/with_listener_isolation.yml fixture and reports WHICH router (HTTPRoute identity) serves
// GET http://abc.foo.example.com/ on the web entryPoint. Pair it with verify/repro/c1_mutate.sh.
package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestVerifyC1_requestIdentity(t *testing.T) {
	k8sObjects, gwObjects := readResources(t, []string{"services.yml", "httproute/with_listener_isolation.yml"})
	client := newClientImpl(kubefake.NewClientset(k8sObjects...), newGatewaySimpleClientSet(t, gwObjects...))
	eventCh, err := client.WatchAll(nil, make(chan struct{}))
	require.NoError(t, err)
	<-eventCh

	p := Provider{EntryPoints: map[string]Entrypoint{"web": {Address: ":80"}, "websecure": {Address: ":443"}}, client: client}
	conf, _, err := p.loadConfigurationFromGateways(t.Context())
	require.NoError(t, err)

	parser, err := httpmuxer.NewSyntaxParser()
	require.NoError(t, err)
	muxer := httpmuxer.NewMuxer(parser, nil)
	for name, router := range conf.HTTP.Routers {
		if router.EntryPoints[0] != "web" {
			continue
		}
		t.Logf("web router %s priority=%d rule=%s", name, router.Priority, router.Rule)
		require.NoError(t, muxer.AddRoute(router.Rule, router.RuleSyntax, router.Priority, "", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("X-Router", name)
		})))
	}

	for host, want := range map[string]string{
		"abc.foo.example.com": "abc-foo-example-com",
		"bar.foo.example.com": "wildcard-foo-example-com",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://"+host+"/", http.NoBody)
		rw := httptest.NewRecorder()
		requestdecorator.New(nil).ServeHTTP(rw, req, muxer.ServeHTTP)
		got := rw.Header().Get("X-Router")
		t.Logf("GET http://%s/ -> router %q", host, got)
		assert.Contains(t, got, want)
	}
}
