//go:build verify

// Shared harness for the verify probes. Run: see verify/repro/run.sh (copies the eval fixtures and these files into pkg/provider/kubernetes/gateway and runs `go test -tags verify`).
package gateway_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

type vProvider struct{ gw, file *dynamic.Configuration }

func (p *vProvider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	ch <- dynamic.Message{ProviderName: "kubernetesgateway", Configuration: p.gw}
	if p.file != nil {
		ch <- dynamic.Message{ProviderName: "file", Configuration: p.file}
	}
	return nil
}
func (p *vProvider) Init() error { return nil }

type vRT struct {
	t       *testing.T
	Agg     dynamic.Configuration
	Routers map[string]*runtime.RouterInfo
	sel     func(sni string) (*tls.Config, string, error)
	handler http.Handler
}

// vRuntime runs the production ConfigurationWatcher (aggregation + resolveHTTPTLSOptions) and
// RouterFactory over the Gateway provider output plus an optional "file" provider configuration.
func vRuntime(t *testing.T, gw, file *dynamic.Configuration, strict bool) *vRT {
	t.Helper()
	const ep = "websecure"
	ctx := t.Context()
	has := func(c dynamic.Configuration, p string) bool {
		if c.HTTP == nil {
			return false
		}
		for n := range c.HTTP.Routers {
			if strings.HasSuffix(n, "@"+p) {
				return true
			}
		}
		return false
	}
	merged := make(chan dynamic.Configuration, 8)
	pool := safe.NewPool(ctx)
	t.Cleanup(pool.Stop)
	w := server.NewConfigurationWatcher(pool, &vProvider{gw: gw, file: file}, []string{ep}, "", strict)
	w.AddListener(func(c dynamic.Configuration) {
		select {
		case merged <- c:
		default:
		}
	})
	w.Start()
	var agg dynamic.Configuration
	timeout := time.NewTimer(10 * time.Second)
	defer timeout.Stop()
	for ready := false; !ready; {
		select {
		case c := <-merged:
			agg = c
			ready = has(c, "kubernetesgateway") && (file == nil || has(c, "file"))
		case <-timeout.C:
			t.Fatal("timed out waiting for aggregated configuration")
		}
	}
	tlsManager := traefiktls.NewManager(nil)
	tlsManager.UpdateConfigs(ctx, agg.TLS.Stores, agg.TLS.Options, agg.TLS.Certificates)
	staticConfig := static.Configuration{EntryPoints: map[string]*static.EntryPoint{ep: {}}}
	tm := service.NewTransportManager(nil)
	tm.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})
	mf := service.NewManagerFactory(staticConfig, nil, nil, tm, evalTagProxy{}, nil, nil)
	dm := tcp.NewDialerManager(nil)
	dm.Update(map[string]*dynamic.TCPServersTransport{"default@internal": {}})
	factory, err := server.NewRouterFactory(staticConfig, mf, tlsManager, nil, nil, dm)
	require.NoError(t, err)
	rtConf := runtime.NewConfig(agg)
	handlers, _ := factory.CreateRouters(rtConf)
	router := handlers[ep]
	require.NotNil(t, router)
	router.SetHTTPSForwarder(evalNoopTCP{})
	selTLS := router.HTTP3TLSConfigMatcherFunc()
	sel := func(sni string) (*tls.Config, string, error) {
		cd, err := tcpmuxer.NewConnData(sni, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4242}, nil)
		if err != nil {
			return nil, "", err
		}
		return selTLS(cd)
	}
	return &vRT{t: t, Agg: agg, Routers: rtConf.Routers, sel: sel, handler: router.GetHTTPSHandler()}
}

// Req dispatches a decrypted HTTPS request exactly as the eval fixtures do (SNI-selected TLS
// options name in the context, then the production HTTPS handler chain incl. SNICheck) and logs it.
func (r *vRT) Req(sni, host, path string) int {
	r.t.Helper()
	cfg, opt, err := r.sel(sni)
	require.NoError(r.t, err)
	req := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
	req.TLS = &tls.ConnectionState{ServerName: sni}
	req = req.WithContext(tcp.AddTLSOptionsNameInContext(context.WithoutCancel(req.Context()), opt))
	rec := httptest.NewRecorder()
	requestdecorator.New(nil).ServeHTTP(rec, req, r.handler.ServeHTTP)
	minv := uint16(0)
	if cfg != nil {
		minv = cfg.MinVersion
	}
	r.t.Logf("REQ sni=%-22s host=%-22s path=%-7s -> %d body=%q | SNI-selected tlsOptions=%q minVersion=%#x", sni, host, path, rec.Code, strings.TrimSpace(rec.Body.String()), opt, minv)
	return rec.Code
}

// Dump logs the aggregated routers (rule, parents, TLS options as declared and as resolved) and their runtime status.
func (r *vRT) Dump() {
	r.t.Helper()
	var names []string
	for n := range r.Agg.HTTP.Routers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rt := r.Agg.HTTP.Routers[n]
		tlsS := "-"
		if rt.TLS != nil {
			tlsS = fmt.Sprintf("options=%q resolved=%q", rt.TLS.Options, rt.TLS.ResolvedOptions)
		}
		st, errs := "", []string(nil)
		if info, ok := r.Routers[n]; ok {
			st, errs = info.Status, info.Err
		}
		r.t.Logf("ROUTER %s\n        rule=%s\n        parents=%v eps=%v tls[%s] status=%s err=%v", n, rt.Rule, rt.ParentRefs, rt.EntryPoints, tlsS, st, errs)
	}
}

func vLoad(t *testing.T, backends string, docs ...string) *dynamic.Configuration {
	t.Helper()
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
	all := append([]string{backends, gateway.EvalIsoClassYAML}, docs...)
	return gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret}, all...)
}

func vFileLeaf(host string) *dynamic.Configuration {
	return &dynamic.Configuration{
		TLS: &dynamic.TLSConfiguration{Options: map[string]traefiktls.Options{"default": {MinVersion: "VersionTLS13"}}},
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"ext-router": {Rule: `Host("` + host + `") && Path("/other")`, EntryPoints: []string{"websecure"}, Service: "ext-svc", TLS: &dynamic.RouterTLSConfig{}},
			},
			Services: map[string]*dynamic.Service{"ext-svc": {LoadBalancer: &dynamic.ServersLoadBalancer{Servers: []dynamic.Server{{URL: "http://ext-backend:8080"}}}}},
		},
	}
}

// vFileParent is an external exact-host parent router (default TLS options) with one child.
func vFileParent(host string, priority int) *dynamic.Configuration {
	return &dynamic.Configuration{
		TLS: &dynamic.TLSConfiguration{Options: map[string]traefiktls.Options{"default": {MinVersion: "VersionTLS13"}}},
		HTTP: &dynamic.HTTPConfiguration{
			Routers: map[string]*dynamic.Router{
				"ext-parent": {Rule: `Host("` + host + `")`, Priority: priority, EntryPoints: []string{"websecure"}, TLS: &dynamic.RouterTLSConfig{}},
				"ext-child":  {Rule: `Path("/other")`, ParentRefs: []string{"ext-parent"}, Service: "ext-svc"},
			},
			Services: map[string]*dynamic.Service{"ext-svc": {LoadBalancer: &dynamic.ServersLoadBalancer{Servers: []dynamic.Server{{URL: "http://ext-backend:8080"}}}}},
		},
	}
}
