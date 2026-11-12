//go:build verify

// Run: cp verify/repro/verify_c1c2_tls_test.go pkg/provider/kubernetes/gateway/ (next to the eval fixtures eval_isolation_test.go, eval_https_test.go, export_eval_test.go) && go test -tags verify -count=1 -v -run '^TestVerifyC1C2_' ./pkg/provider/kubernetes/gateway

package gateway_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/config/static"
	tcpmuxer "github.com/traefik/traefik/v3/pkg/muxer/tcp"
	"github.com/traefik/traefik/v3/pkg/observability/metrics"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	tcprouter "github.com/traefik/traefik/v3/pkg/server/router/tcp"
	"github.com/traefik/traefik/v3/pkg/server/service"
	"github.com/traefik/traefik/v3/pkg/tcp"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
	"golang.org/x/net/http2"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

type verifySyncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *verifySyncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *verifySyncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type verifyEnv struct {
	agg        dynamic.Configuration
	tlsManager *traefiktls.Manager
	router     *tcprouter.Router
	addr       string
	logs       string
}

// verifyStart runs the provider output through the production pipeline, exactly as the eval fixture
// evalServeHTTPSFull does (server.NewConfigurationWatcher -> tls.Manager.UpdateConfigs -> server.NewRouterFactory),
// and then, instead of calling the handlers in-process, serves the resulting TCP router on a real
// TCP entry point (server.NewTCPEntryPoints + Switch + Start) so real TLS handshakes can be made.
func verifyStart(t *testing.T, conf *dynamic.Configuration, ep string, strict bool) *verifyEnv {
	t.Helper()
	ctx := t.Context()

	logBuf := &verifySyncBuffer{}
	prevLogger := log.Logger
	log.Logger = zerolog.New(logBuf).Level(zerolog.DebugLevel)
	t.Cleanup(func() { log.Logger = prevLogger })

	merged := make(chan dynamic.Configuration, 1)
	pool := safe.NewPool(ctx)
	t.Cleanup(pool.Stop)
	watcher := server.NewConfigurationWatcher(pool, &evalOneShotProvider{conf: conf}, []string{ep}, "", strict)
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

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	epConf := &static.EntryPoint{}
	epConf.SetDefaults()
	epConf.Address = addr

	staticConfig := static.Configuration{EntryPoints: map[string]*static.EntryPoint{ep: epConf}}
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

	eps, err := server.NewTCPEntryPoints(staticConfig.EntryPoints, nil, metrics.NewVoidRegistry())
	require.NoError(t, err)
	eps.Switch(map[string]*tcprouter.Router{ep: router})
	eps.Start()
	t.Cleanup(eps.Stop)

	require.Eventually(t, func() bool {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return false
		}
		_ = c.Close()
		return true
	}, 5*time.Second, 20*time.Millisecond)

	return &verifyEnv{agg: agg, tlsManager: tlsManager, router: router, addr: addr, logs: logBuf.String()}
}

// handshake performs a real TLS handshake against the entry point, offering h2 then http/1.1.
func (e *verifyEnv) handshake(t *testing.T, sni string) string {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", e.addr, &tls.Config{
		ServerName:         sni,
		InsecureSkipVerify: true,
		NextProtos:         []string{"h2", "http/1.1"},
	})
	require.NoError(t, err)
	defer conn.Close()
	return fmt.Sprintf("%q", conn.ConnectionState().NegotiatedProtocol)
}

// request sends a real HTTPS request with Go's standard client (HTTP/2 enabled, as any browser/curl/gRPC client).
func (e *verifyEnv) request(t *testing.T, sni, host, path string) string {
	t.Helper()
	tr := &http.Transport{
		ForceAttemptHTTP2: true,
		TLSClientConfig:   &tls.Config{ServerName: sni, InsecureSkipVerify: true},
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, e.addr)
		},
	}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequest(http.MethodGet, "https://"+host+path, nil)
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("proto=%s status=%d body=%q", resp.Proto, resp.StatusCode, strings.TrimSpace(string(body)))
}

// requestH2Only sends a real HTTPS request with an HTTP/2-only client (what a gRPC client is).
func (e *verifyEnv) requestH2Only(t *testing.T, sni, host, path string) string {
	t.Helper()
	tr := &http2.Transport{
		TLSClientConfig: &tls.Config{ServerName: sni, InsecureSkipVerify: true},
		DialTLSContext: func(ctx context.Context, network, _ string, cfg *tls.Config) (net.Conn, error) {
			return (&tls.Dialer{NetDialer: &net.Dialer{Timeout: 5 * time.Second}, Config: cfg}).DialContext(ctx, network, e.addr)
		},
	}
	defer tr.CloseIdleConnections()
	req, err := http.NewRequest(http.MethodGet, "https://"+host+path, nil)
	require.NoError(t, err)
	resp, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return "error: " + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return fmt.Sprintf("proto=%s status=%d body=%q", resp.Proto, resp.StatusCode, strings.TrimSpace(string(body)))
}

func verifyDump(t *testing.T, tag string, conf *dynamic.Configuration, env *verifyEnv, snis []string, reqs [][3]string) {
	t.Helper()

	var names []string
	for name := range conf.TLS.Options {
		names = append(names, name)
	}
	slices.Sort(names)
	fmt.Printf("%s | provider output: %d TLS options\n", tag, len(names))
	for _, name := range names {
		fmt.Printf("%s |   provider tls.Options[%q].ALPNProtocols=%v (len=%d)\n", tag, name, conf.TLS.Options[name].ALPNProtocols, len(conf.TLS.Options[name].ALPNProtocols))
	}

	names = nil
	for name := range env.agg.TLS.Options {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		cfg, err := env.tlsManager.Get("default", name)
		require.NoError(t, err)
		fmt.Printf("%s |   aggregated tls.Options[%q].ALPNProtocols=%v -> tlsManager.Get NextProtos=%v (len=%d)\n", tag, name, env.agg.TLS.Options[name].ALPNProtocols, cfg.NextProtos, len(cfg.NextProtos))
	}

	names = nil
	for name := range env.agg.HTTP.Routers {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		r := env.agg.HTTP.Routers[name]
		if r.TLS == nil {
			continue
		}
		short := name
		if len(short) > 46 {
			short = short[:20] + "…" + short[len(short)-25:]
		}
		fmt.Printf("%s |   aggregated router %s rule=%s configuredOptions=%q resolvedOptions=%q\n", tag, short, r.Rule, r.TLS.Options, r.TLS.ResolvedOptions)
	}

	for _, line := range strings.Split(strings.TrimSpace(env.logs), "\n") {
		if strings.Contains(line, "TLS options") || strings.Contains(line, "TLSOptions") {
			fmt.Printf("%s |   aggregator log %s\n", tag, line)
		}
	}

	selectTLS := env.router.HTTP3TLSConfigMatcherFunc()
	for _, sni := range snis {
		cd, err := tcpmuxer.NewConnData(sni, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4242}, nil)
		require.NoError(t, err)
		cfg, optionsName, err := selectTLS(cd)
		require.NoError(t, err)
		fmt.Printf("%s |   SNI %-22s -> TCP router selects TLS options %q NextProtos=%v | real handshake offering [h2 http/1.1] negotiated %s\n", tag, sni, optionsName, cfg.NextProtos, env.handshake(t, sni))
	}

	for _, r := range reqs {
		fmt.Printf("%s |   real HTTPS request SNI=%s Host=%s %s -> %s\n", tag, r[0], r[1], r[2], env.request(t, r[0], r[1], r[2]))
		if r[0] == r[1] {
			fmt.Printf("%s |   real HTTP/2-only (gRPC-like) request SNI=%s Host=%s %s -> %s\n", tag, r[0], r[1], r[2], env.requestH2Only(t, r[0], r[1], r[2]))
		}
	}
}

func TestVerifyC1C2_WildcardAndExactListeners(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	for _, strict := range []bool{false, true} {
		t.Run(fmt.Sprintf("strictTLSOptions=%t", strict), func(t *testing.T) {
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "wildcard=*.example.com", "precise=abc.example.com"),
				gateway.EvalIsoRouteExported("http-app-wildcard", "my-gateway", "wildcard", "/wildcard"),
				gateway.EvalIsoWithHostnamesExported(gateway.EvalIsoRouteExported("http-app-bar", "my-gateway", "wildcard", "/bar"), "bar.example.com"),
				gateway.EvalIsoRouteExported("http-app-precise", "my-gateway", "precise", "/precise"),
			)
			env := verifyStart(t, conf, "websecure", strict)
			verifyDump(t, fmt.Sprintf("C1C2 wildcard+exact strict=%t", strict), conf, env,
				[]string{"abc.example.com", "xyz.example.com", "bar.example.com", "other.test"},
				[][3]string{
					{"abc.example.com", "abc.example.com", "/precise"},
					{"xyz.example.com", "xyz.example.com", "/wildcard"},
					{"bar.example.com", "bar.example.com", "/bar"},
					{"abc.example.com", "xyz.example.com", "/wildcard"},
					{"xyz.example.com", "abc.example.com", "/precise"},
				})
		})
	}
}

func TestVerifyC1C2_TwoWildcardListeners(t *testing.T) {
	secret := gateway.EvalIsoNamespacedTLSSecretExported(t, "default", "supersecret", "example.com", "*.example.com", "*.foo.example.com")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
		gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "wildcard=*.example.com", "nested=*.foo.example.com"),
		gateway.EvalIsoRouteExported("http-app-wildcard", "my-gateway", "wildcard", "/wildcard"),
		gateway.EvalIsoRouteExported("http-app-nested", "my-gateway", "nested", "/nested"),
	)
	env := verifyStart(t, conf, "websecure", false)
	verifyDump(t, "C1C2 two-wildcards strict=false", conf, env,
		[]string{"xyz.example.com", "a.foo.example.com"},
		[][3]string{
			{"xyz.example.com", "xyz.example.com", "/wildcard"},
			{"a.foo.example.com", "a.foo.example.com", "/nested"},
			{"a.foo.example.com", "xyz.example.com", "/wildcard"},
			{"xyz.example.com", "a.foo.example.com", "/nested"},
		})
}

func TestVerifyC1C2_SingleListenerControl(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}

	conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
		gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "precise=abc.example.com"),
		gateway.EvalIsoRouteExported("http-app-precise", "my-gateway", "precise", "/precise"),
	)
	env := verifyStart(t, conf, "websecure", false)
	verifyDump(t, "C1C2 single-listener control", conf, env,
		[]string{"abc.example.com"},
		[][3]string{{"abc.example.com", "abc.example.com", "/precise"}})
}
