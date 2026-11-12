//go:build verify

// Shared harness for the verify probes. Run: see the header of verify_c1c2_tlsconflict_test.go / verify_c3_misdirected_test.go.
// Copy next to the eval fixtures in pkg/provider/kubernetes/gateway/ and run with `-tags verify`.

package gateway_test

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	"github.com/traefik/traefik/v3/pkg/observability/metrics"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	tcprouter "github.com/traefik/traefik/v3/pkg/server/router/tcp"
	"github.com/traefik/traefik/v3/pkg/server/service"
	"github.com/traefik/traefik/v3/pkg/tcp"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
)

// verifyProvider emits one dynamic.Message per provider, as the provider aggregator does.
type verifyProvider struct{ msgs []dynamic.Message }

func (p *verifyProvider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	for _, m := range p.msgs {
		ch <- m
	}
	return nil
}

func (p *verifyProvider) Init() error { return nil }

// verifyTagProxy answers 200 with the backend IP as body, so a response names the backend that served it.
type verifyTagProxy struct{}

func (verifyTagProxy) Build(_ string, target *url.URL, _, _ bool, _ time.Duration) (http.Handler, error) {
	return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write([]byte(target.Hostname()))
	}), nil
}

func (verifyTagProxy) Update(map[string]*dynamic.ServersTransport) {}

type verifyRuntime struct {
	// Agg is what the production ConfigurationWatcher hands to its listeners
	// (mergeConfiguration + applyModel + resolveHTTPTLSOptions).
	Agg dynamic.Configuration
	// RT is the runtime configuration after RouterFactory.CreateRouters (ParseRouterTree + BuildHandlers).
	RT     *runtime.Configuration
	Router *tcprouter.Router
	ep     string
}

// verifyBuild pushes the given provider messages through the production server.ConfigurationWatcher
// and server.RouterFactory, exactly as cmd/traefik wires them.
func verifyBuild(t *testing.T, msgs []dynamic.Message, ep string, strict bool) *verifyRuntime {
	t.Helper()
	ctx := t.Context()

	merged := make(chan dynamic.Configuration, 16)
	pool := safe.NewPool(ctx)
	t.Cleanup(pool.Stop)
	watcher := server.NewConfigurationWatcher(pool, &verifyProvider{msgs: msgs}, []string{ep}, "", strict)
	watcher.AddListener(func(c dynamic.Configuration) {
		select {
		case merged <- c:
		default:
		}
	})
	watcher.Start()

	hasAll := func(c dynamic.Configuration) bool {
		for _, m := range msgs {
			found := false
			for name := range c.HTTP.Routers {
				if strings.HasSuffix(name, "@"+m.ProviderName) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	}

	var agg dynamic.Configuration
	deadline := time.After(15 * time.Second)
wait:
	for {
		select {
		case c := <-merged:
			if hasAll(c) {
				agg = c
				break wait
			}
		case <-deadline:
			t.Fatal("timed out waiting for the aggregated configuration of all providers")
		}
	}

	tlsManager := traefiktls.NewManager(nil)
	tlsManager.UpdateConfigs(ctx, agg.TLS.Stores, agg.TLS.Options, agg.TLS.Certificates)

	staticConfig := static.Configuration{EntryPoints: map[string]*static.EntryPoint{ep: {}}}
	transportManager := service.NewTransportManager(nil)
	transportManager.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})
	managerFactory := service.NewManagerFactory(staticConfig, nil, nil, transportManager, verifyTagProxy{}, nil, nil)
	dialerManager := tcp.NewDialerManager(nil)
	dialerManager.Update(map[string]*dynamic.TCPServersTransport{"default@internal": {}})
	factory, err := server.NewRouterFactory(staticConfig, managerFactory, tlsManager, nil, nil, dialerManager)
	require.NoError(t, err)

	rtConf := runtime.NewConfig(agg)
	handlers, _ := factory.CreateRouters(rtConf)
	require.NotNil(t, handlers[ep])

	return &verifyRuntime{Agg: agg, RT: rtConf, Router: handlers[ep], ep: ep}
}

// ServeInProcess selects the TLS options by SNI as the TCP router does, then dispatches the
// decrypted request through the production HTTPS handler chain (same method as the eval fixture
// evalHTTPSRuntimeFor).
func (r *verifyRuntime) ServeInProcess(t *testing.T, sni, host, path string, extra ...map[string]string) (int, string) {
	t.Helper()
	method, hdrs := verifyMethodAndHeaders(extra)
	cd, err := tcpmuxer.NewConnData(sni, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4242}, nil)
	require.NoError(t, err)
	_, optionsName, err := r.Router.HTTP3TLSConfigMatcherFunc()(cd)
	require.NoError(t, err)

	req := httptest.NewRequest(method, "https://"+host+path, nil)
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	req.TLS = &tls.ConnectionState{ServerName: sni}
	req = req.WithContext(tcp.AddTLSOptionsNameInContext(context.WithoutCancel(req.Context()), optionsName))
	rec := httptest.NewRecorder()
	requestdecorator.New(nil).ServeHTTP(rec, req, r.Router.GetHTTPSHandler().ServeHTTP)
	return rec.Code, rec.Body.String()
}

// StartRealTLS binds a real Traefik TCP entry point on a free loopback port and installs the router,
// returning a function that performs a real TLS handshake (given SNI) and HTTP/1.1 request (given Host).
func (r *verifyRuntime) StartRealTLS(t *testing.T) func(sni, host, path string, extra ...map[string]string) (int, string, error) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	epCfg := &static.EntryPoint{}
	epCfg.SetDefaults()
	epCfg.Address = addr

	eps, err := server.NewTCPEntryPoints(static.EntryPoints{r.ep: epCfg}, nil, metrics.NewVoidRegistry())
	require.NoError(t, err)
	eps.Switch(map[string]*tcprouter.Router{r.ep: r.Router})
	eps.Start()
	t.Cleanup(eps.Stop)

	return func(sni, host, path string, extra ...map[string]string) (int, string, error) {
		method, hdrs := verifyMethodAndHeaders(extra)
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{ServerName: sni, InsecureSkipVerify: true},
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, addr)
			},
			DisableKeepAlives: true,
		}
		defer tr.CloseIdleConnections()

		req, err := http.NewRequest(method, "https://"+host+path, nil)
		if err != nil {
			return 0, "", err
		}
		req.Host = host
		for k, v := range hdrs {
			req.Header.Set(k, v)
		}

		var resp *http.Response
		for i := 0; i < 50; i++ {
			resp, err = (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Do(req)
			if err == nil || !strings.Contains(err.Error(), "connection refused") {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, strings.TrimSpace(string(body)), nil
	}
}

// verifyMethodAndHeaders reads the optional ":method" pseudo entry and headers, as the eval fixtures do.
func verifyMethodAndHeaders(extra []map[string]string) (string, map[string]string) {
	method := http.MethodGet
	hdrs := map[string]string{}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			if k == ":method" {
				method = v
				continue
			}
			hdrs[k] = v
		}
	}
	return method, hdrs
}

// DumpAgg logs the routers of the aggregated configuration and, for each child router, whether
// each of its ParentRefs resolves to a router that exists after aggregation.
func (r *verifyRuntime) DumpAgg(t *testing.T) (dangling, resolved int) {
	t.Helper()
	names := make([]string, 0, len(r.Agg.HTTP.Routers))
	for name := range r.Agg.HTTP.Routers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		rt := r.Agg.HTTP.Routers[name]
		tlsInfo := "tls=nil"
		if rt.TLS != nil {
			tlsInfo = fmt.Sprintf("tls.options=%q resolved=%q conflicting=%t", rt.TLS.Options, rt.TLS.ResolvedOptions, rt.TLS.ConflictingOptions)
		}
		t.Logf("AGG router %q\n      rule=%s priority=%d eps=%v service=%q %s", name, rt.Rule, rt.Priority, rt.EntryPoints, rt.Service, tlsInfo)
		for _, ref := range rt.ParentRefs {
			_, ok := r.Agg.HTTP.Routers[ref]
			if ok {
				resolved++
			} else {
				dangling++
			}
			t.Logf("      parentRef %q existsAfterAggregation=%t", ref, ok)
		}
	}
	return dangling, resolved
}

// DumpRuntime logs the status and errors ParseRouterTree/BuildHandlers recorded on each router.
func (r *verifyRuntime) DumpRuntime(t *testing.T) (disabledChildren int) {
	t.Helper()
	names := make([]string, 0, len(r.RT.Routers))
	for name := range r.RT.Routers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		info := r.RT.Routers[name]
		t.Logf("RUNTIME router %q status=%s childRefs=%v err=%v", name, info.Status, info.ChildRefs, info.Err)
		if len(info.ParentRefs) > 0 && info.Status == runtime.StatusDisabled {
			disabledChildren++
		}
	}
	return disabledChildren
}
