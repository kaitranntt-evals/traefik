// Run (C2, branch evalon/traefik-ga-fe614666 of kaitranntt-evals/traefik-gateway-listener-isolation): copy the eval fixtures (tests/*.go) and this file into pkg/provider/kubernetes/gateway/, then `go test -race -count=1 -timeout=5m -run "^TestVerifyC2_" -v ./pkg/provider/kubernetes/gateway`.
package gateway_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

func TestVerifyC2_CatchAllExactHostnamesUnderWildcardListener(t *testing.T) {
	eps := map[string]gateway.Entrypoint{"web": {Address: ":80"}}

	conf := gateway.EvalIsoLoadExported(t, eps, nil,
		gateway.EvalIsoWhoamiYAML, evalIsoMultiBackendYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoGatewayExported("my-gateway", "catchall=", "wild=*.example.com"),
		gateway.EvalIsoWithHostnamesExported(
			gateway.EvalIsoRouteWithServiceExported("http-app-catchall", "my-gateway", "catchall", "/catchall", "whoami-catchall"),
			"foo.example.com", "bar.example.com"),
		gateway.EvalIsoRouteWithServiceExported("http-app-wild", "my-gateway", "wild", "/wild", "whoami-precise"),
	)

	var catchallRule string
	for name, r := range conf.HTTP.Routers {
		t.Logf("ROUTER %s rule=%s priority=%d service=%s", name, r.Rule, r.Priority, r.Service)
		if strings.Contains(name, "http-app-catchall") {
			catchallRule = r.Rule
		}
	}
	// Part 1: the catch-all route rule keeps the exact hostnames and must not exclude the wildcard listener region.
	assert.Contains(t, catchallRule, `Host("foo.example.com")`)
	assert.Contains(t, catchallRule, `Host("bar.example.com")`)
	assert.NotContains(t, catchallRule, `!Host("**.example.com")`, "claim: no exclusion of the *.example.com region")

	serve := evalServeHTTP(t, conf, "web")
	// Part 2: requests for the exact hostnames reach the catch-all route backend (10.10.2.1).
	for _, host := range []string{"foo.example.com", "bar.example.com"} {
		code, body := serve(host, "/catchall")
		t.Logf("Host=%s /catchall -> %d %q", host, code, body)
		assert.Equal(t, http.StatusOK, code, host)
		assert.Equal(t, "10.10.2.1", body, host)
	}
	code, body := serve("foo.example.com", "/wild")
	t.Logf("Host=foo.example.com /wild -> %d %q", code, body)
	assert.Equal(t, http.StatusOK, code)
	assert.Equal(t, "10.10.1.1", body)
	code, body = serve("other.org", "/catchall")
	t.Logf("Host=other.org /catchall -> %d %q", code, body)
}
