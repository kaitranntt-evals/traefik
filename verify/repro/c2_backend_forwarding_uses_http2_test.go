// Run (on evalon/traefik-en-2d7182b0): cp verify/repro/c2_backend_forwarding_uses_http2_test.go pkg/server/service/ && go test -count=1 -v -run '^TestVerifyC2_BackendForwardingUsesHTTP2$' ./pkg/server/service
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
	"github.com/traefik/traefik/v2/pkg/config/runtime"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// TestVerifyC2_BackendForwardingUsesHTTP2 forwards through the production service manager
// (default ServersTransport, DisableHTTP2=false) and records the protocol the backend sees.
func TestVerifyC2_BackendForwardingUsesHTTP2(t *testing.T) {
	record := func(seen *atomic.Value) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			seen.Store(req.Proto)
			rw.WriteHeader(http.StatusOK)
		})
	}

	var tlsProto, h2cProto, h1Proto atomic.Value
	h2srv := httptest.NewUnstartedServer(record(&tlsProto))
	h2srv.EnableHTTP2 = true
	h2srv.StartTLS()
	t.Cleanup(h2srv.Close)

	h2cSrv := httptest.NewServer(h2c.NewHandler(record(&h2cProto), &http2.Server{}))
	t.Cleanup(h2cSrv.Close)
	h2cURL := "h2c://" + strings.TrimPrefix(h2cSrv.URL, "http://")

	h1srv := httptest.NewServer(record(&h1Proto))
	t.Cleanup(h1srv.Close)

	rtManager := NewRoundTripperManager()
	rtManager.Update(map[string]*dynamic.ServersTransport{"default@internal": {InsecureSkipVerify: true}})
	t.Cleanup(func() {
		rt, _ := rtManager.Get("default@internal")
		if c, ok := rt.(*KerberosRoundTripper).OriginalRoundTripper.(*smartRoundTripper); ok {
			c.http.CloseIdleConnections()
			c.http2.CloseIdleConnections()
		}
	})

	rtConfig := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
		Services: map[string]*dynamic.Service{
			"h2":  {LoadBalancer: &dynamic.ServersLoadBalancer{Servers: []dynamic.Server{{URL: h2srv.URL}}}},
			"h2c": {LoadBalancer: &dynamic.ServersLoadBalancer{Servers: []dynamic.Server{{URL: h2cURL}}}},
			"h1":  {LoadBalancer: &dynamic.ServersLoadBalancer{Servers: []dynamic.Server{{URL: h1srv.URL}}}},
		},
	}})
	manager := NewManager(rtConfig.Services, nil, nil, rtManager)

	for _, test := range []struct {
		service string
		url     string
		seen    *atomic.Value
		want    string
	}{
		{"h2", h2srv.URL, &tlsProto, "HTTP/2.0"},
		{"h2c", h2cURL, &h2cProto, "HTTP/2.0"},
		{"h1", h1srv.URL, &h1Proto, "HTTP/1.1"},
	} {
		handler, err := manager.BuildHTTP(context.Background(), test.service)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
		require.Equal(t, http.StatusOK, rw.Code, test.service)
		t.Logf("service %q (server URL %s): backend received request over %v", test.service, test.url, test.seen.Load())
		assert.Equal(t, test.want, test.seen.Load(), test.service)
	}
}
