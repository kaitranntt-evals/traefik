// Run (on evalon/traefik-en-d6261075): cp verify/repro/c1_backend_transport_never_closed_test.go pkg/server/ && go test -count=1 -v -run '^TestVerifyC1_BackendTransportNeverClosed$' ./pkg/server
package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/config/dynamic"
	"github.com/traefik/traefik/v2/pkg/config/runtime"
	"github.com/traefik/traefik/v2/pkg/metrics"
	"github.com/traefik/traefik/v2/pkg/middlewares/requestdecorator"
	"github.com/traefik/traefik/v2/pkg/server/middleware"
	httprouter "github.com/traefik/traefik/v2/pkg/server/router"
	"github.com/traefik/traefik/v2/pkg/server/service"
	traefiktls "github.com/traefik/traefik/v2/pkg/tls"
)

// TestVerifyC1_BackendTransportNeverClosed mirrors the backend wiring of TestHTTPConnect
// (service.NewRoundTripperManager + Update("default@internal")) and shows that the backend
// client transport it constructs keeps an idle keep-alive connection to the backend open,
// and exposes no CloseIdleConnections/Close that a teardown could register.
func TestVerifyC1_BackendTransportNeverClosed(t *testing.T) {
	var mu sync.Mutex
	states := map[net.Conn]http.ConnState{}
	idleConns := func() int {
		mu.Lock()
		defer mu.Unlock()
		n := 0
		for _, st := range states {
			if st == http.StateIdle {
				n++
			}
		}
		return n
	}
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
	backend.Config.ConnState = func(c net.Conn, st http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		switch st {
		case http.StateClosed, http.StateHijacked:
			delete(states, c)
		default:
			states[c] = st
		}
	}
	backend.Start()
	t.Cleanup(backend.Close)

	// Same construction as TestHTTPConnect's routing subtests.
	rtConfig := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
		Routers: map[string]*dynamic.Router{"test": {
			EntryPoints: []string{"web"}, Rule: "Host(`matched.example`)", Service: "backend",
		}},
		Services: map[string]*dynamic.Service{"backend": {LoadBalancer: &dynamic.ServersLoadBalancer{
			Servers: []dynamic.Server{{URL: backend.URL}},
		}}},
	}})
	transports := service.NewRoundTripperManager()
	transports.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})
	services := service.NewManager(rtConfig.Services, nil, nil, transports)
	middlewares := middleware.NewBuilder(rtConfig.Middlewares, services, nil)
	chain := middleware.NewChainBuilder(nil, nil, nil)
	manager := httprouter.NewManager(rtConfig, services, middlewares, chain, metrics.NewVoidRegistry(), traefiktls.NewManager())
	routerHandler := manager.BuildHandlers(context.Background(), []string{"web"}, false)["web"]
	require.NotNil(t, routerHandler)
	// The entrypoint wraps the router handler with the request decorator (host canonization) in production.
	decorator := requestdecorator.New(nil)
	handler := http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		decorator.ServeHTTP(rw, req, routerHandler.ServeHTTP)
	})

	req := httptest.NewRequest(http.MethodGet, "http://matched.example/", http.NoBody)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)
	require.Equal(t, http.StatusOK, rw.Code)

	require.Eventually(t, func() bool { return idleConns() == 1 }, 2*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond)
	t.Logf("idle keep-alive connections held open on the backend after the proxied request: %d", idleConns())
	assert.Equal(t, 1, idleConns())

	rt, err := transports.Get("default@internal")
	require.NoError(t, err)
	_, rtCloses := rt.(interface{ CloseIdleConnections() })
	_, mgrCloses := any(transports).(interface{ CloseIdleConnections() })
	_, mgrClose := any(transports).(interface{ Close() error })
	inner := rt.(*service.KerberosRoundTripper).OriginalRoundTripper
	_, innerCloses := inner.(interface{ CloseIdleConnections() })
	t.Logf("RoundTripperManager has CloseIdleConnections=%v Close=%v; round tripper %T has CloseIdleConnections=%v; inner %T has CloseIdleConnections=%v",
		mgrCloses, mgrClose, rt, rtCloses, inner, innerCloses)
	assert.False(t, rtCloses)
	assert.False(t, mgrCloses)
	assert.False(t, mgrClose)
	assert.False(t, innerCloses)
}
