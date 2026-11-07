//go:build verify

// Run: copy next to the eval fixtures in pkg/provider/kubernetes/gateway/ on branch evalon/traefik-ga-e500a6b0, then
//   go test -tags verify -count=1 -run 'TestVerifyC2' -v ./pkg/provider/kubernetes/gateway/
package gateway_test

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/config/static"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	tcpmuxer "github.com/traefik/traefik/v3/pkg/muxer/tcp"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/service"
	"github.com/traefik/traefik/v3/pkg/tcp"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// verifyTwoProviders emits the Gateway provider configuration and then an "internal" provider
// configuration carrying the entry point model (what static entryPoints.<ep>.http.middlewares produces).
type verifyTwoProviders struct {
	gw       *dynamic.Configuration
	internal *dynamic.Configuration
}

func (p *verifyTwoProviders) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	ch <- dynamic.Message{ProviderName: "kubernetesgateway", Configuration: p.gw}
	ch <- dynamic.Message{ProviderName: "internal", Configuration: p.internal}
	return nil
}

func (p *verifyTwoProviders) Init() error { return nil }

func verifyServeHTTPSWithModel(t *testing.T, conf, internal *dynamic.Configuration, ep string) (func(sni, host, path string, hdr map[string]string) (int, string), *runtime.Configuration) {
	t.Helper()
	ctx := t.Context()

	merged := make(chan dynamic.Configuration, 1)
	pool := safe.NewPool(ctx)
	t.Cleanup(pool.Stop)
	watcher := server.NewConfigurationWatcher(pool, &verifyTwoProviders{gw: conf, internal: internal}, []string{ep}, "internal", false)
	watcher.AddListener(func(c dynamic.Configuration) {
		select {
		case merged <- c:
		default:
		}
	})
	watcher.Start()

	var agg dynamic.Configuration
	select {
	case agg = <-merged:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for aggregated configuration")
	}

	tlsManager := traefiktls.NewManager(nil)
	tlsManager.UpdateConfigs(ctx, agg.TLS.Stores, agg.TLS.Options, agg.TLS.Certificates)

	staticConfig := static.Configuration{EntryPoints: map[string]*static.EntryPoint{ep: {}}}
	transportManager := service.NewTransportManager(nil)
	transportManager.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})
	managerFactory := service.NewManagerFactory(staticConfig, nil, nil, transportManager, evalTagProxy{}, nil, nil)
	dialerManager := tcp.NewDialerManager(nil)
	dialerManager.Update(map[string]*dynamic.TCPServersTransport{"default@internal": {}})
	factory, err := server.NewRouterFactory(staticConfig, managerFactory, tlsManager, nil, nil, dialerManager)
	require.NoError(t, err)

	rtConf := runtime.NewConfig(agg)
	handlers, _ := factory.CreateRouters(rtConf)
	router := handlers[ep]
	require.NotNil(t, router)
	router.SetHTTPSForwarder(evalNoopTCP{})
	selectTLS := router.HTTP3TLSConfigMatcherFunc()

	return func(sni, host, path string, hdr map[string]string) (int, string) {
		cd, err := tcpmuxer.NewConnData(sni, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4242}, nil)
		require.NoError(t, err)
		_, optionsName, err := selectTLS(cd)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		req.TLS = &tls.ConnectionState{ServerName: sni}
		req = req.WithContext(tcp.AddTLSOptionsNameInContext(context.WithoutCancel(req.Context()), optionsName))
		rec := httptest.NewRecorder()
		requestdecorator.New(nil).ServeHTTP(rec, req, router.GetHTTPSHandler().ServeHTTP)
		return rec.Code, rec.Body.String()
	}, rtConf
}

func verifyBcrypt(t *testing.T, pw string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.MinCost)
	require.NoError(t, err)
	return string(h)
}

func TestVerifyC2(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
		gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "wildcard=*.example.com", "precise=abc.example.com"),
		gateway.EvalIsoRouteExported("http-app-wildcard", "my-gateway", "wildcard", "/wildcard"),
		gateway.EvalIsoRouteExported("http-app-precise", "my-gateway", "precise", "/precise"),
	)

	// Entry point authentication: static entryPoints.websecure.http.middlewares = ["epauth@internal"].
	internal := &dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{},
			Services: map[string]*dynamic.Service{},
			Middlewares: map[string]*dynamic.Middleware{
				// user "test", password "test" (htpasswd bcrypt).
				"epauth": {BasicAuth: &dynamic.BasicAuth{Users: []string{"test:" + verifyBcrypt(t, "test")}}},
			},
			Models: map[string]*dynamic.Model{
				"websecure": {Middlewares: []string{"epauth@internal"}},
			},
		},
	}

	for _, withAuth := range []bool{false, true} {
		t.Run(map[bool]string{false: "no-entrypoint-auth", true: "entrypoint-auth"}[withAuth], func(t *testing.T) {
			in := internal
			if !withAuth {
				in = &dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
					Middlewares: map[string]*dynamic.Middleware{"noop": {Headers: &dynamic.Headers{CustomRequestHeaders: map[string]string{"X-Verify": "1"}}}},
					Models:      map[string]*dynamic.Model{"websecure": {}},
				}}
			}
			serve, rt := verifyServeHTTPSWithModel(t, conf, in, "websecure")

			var names []string
			for n := range rt.Routers {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				r := rt.Routers[n]
				t.Logf("RUNTIME ROUTER %s: prio=%d middlewares=%v service=%s rule=%s", n, r.Priority, r.Middlewares, r.Service, r.Rule)
			}

			for _, rq := range []struct{ sni, host, path string }{
				{"abc.example.com", "abc.example.com", "/precise"},
				{"xyz.example.com", "xyz.example.com", "/wildcard"},
				{"xyz.example.com", "abc.example.com", "/precise"}, // SNI wildcard listener, Host exact listener
				{"abc.example.com", "xyz.example.com", "/wildcard"}, // SNI exact listener, Host wildcard listener
				{"xyz.example.com", "abc.example.com", "/nope"},
			} {
				code, body := serve(rq.sni, rq.host, rq.path, nil)
				t.Logf("UNAUTH REQ sni=%-16s host=%-16s path=%-9s -> %d %s", rq.sni, rq.host, rq.path, code, strings.TrimSpace(body))
				code, body = serve(rq.sni, rq.host, rq.path, map[string]string{"Authorization": "Basic dGVzdDp0ZXN0"})
				t.Logf("AUTHED REQ sni=%-16s host=%-16s path=%-9s -> %d %s", rq.sni, rq.host, rq.path, code, strings.TrimSpace(body))
			}
		})
	}
}
