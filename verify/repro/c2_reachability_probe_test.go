// Run: on evalon/traefik-ga-968ccc91, copy this file plus the eval fixtures (eval_https_test.go, eval_isolation_test.go, export_eval_test.go) to pkg/provider/kubernetes/gateway/, then `go test -tags verify -count=1 -v -run TestVerifyC2Reachability ./pkg/provider/kubernetes/gateway/`.

//go:build verify

package gateway_test

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
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
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	tcprouter "github.com/traefik/traefik/v3/pkg/server/router/tcp"
	"github.com/traefik/traefik/v3/pkg/server/service"
	"github.com/traefik/traefik/v3/pkg/tcp"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

const verifyEP = "websecure"

// verifyBackends maps the body written by the fixture's evalTagProxy (the backend pod IP of
// evalIsoMultiBackendYAML) to the listener owning that backend.
var verifyBackends = map[string]string{"10.10.1.1": "EXACT-listener-backend", "10.10.2.1": "catchall-listener-backend"}

func verifyBackend(body string) string {
	body = strings.TrimSpace(body)
	if name, ok := verifyBackends[body]; ok {
		return name
	}
	return fmt.Sprintf("-(%q)", body)
}

// verifyDistinctCertGateway declares a hostname-less listener and an `abc.example.com` listener on
// port 443, each with its own certificate Secret.
const verifyDistinctCertGateway = `
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: my-gateway
  namespace: default
spec:
  gatewayClassName: my-gateway-class
  listeners:
    - name: catchall
      protocol: HTTPS
      port: 443
      tls:
        certificateRefs:
          - kind: Secret
            name: cert-catchall
            group: ""
      allowedRoutes:
        namespaces:
          from: Same
    - name: precise
      protocol: HTTPS
      port: 443
      hostname: "abc.example.com"
      tls:
        certificateRefs:
          - kind: Secret
            name: cert-precise
            group: ""
      allowedRoutes:
        namespaces:
          from: Same
`

// verifyBuildRouters aggregates the provider output through pkg/server exactly like the fixture's
// evalServeHTTPSFull, and returns the aggregated configuration and the entry point TCP router.
func verifyBuildRouters(t *testing.T, conf *dynamic.Configuration, strict bool, staticEP *static.EntryPoint) (dynamic.Configuration, *tcprouter.Router) {
	t.Helper()
	ctx := t.Context()

	merged := make(chan dynamic.Configuration, 1)
	pool := safe.NewPool(ctx)
	t.Cleanup(pool.Stop)
	watcher := server.NewConfigurationWatcher(pool, &evalOneShotProvider{conf: conf}, []string{verifyEP}, "", strict)
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

	staticConfig := static.Configuration{EntryPoints: map[string]*static.EntryPoint{verifyEP: staticEP}}
	transportManager := service.NewTransportManager(nil)
	transportManager.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})
	managerFactory := service.NewManagerFactory(staticConfig, nil, nil, transportManager, evalTagProxy{}, nil, nil)
	dialerManager := tcp.NewDialerManager(nil)
	dialerManager.Update(map[string]*dynamic.TCPServersTransport{"default@internal": {}})
	factory, err := server.NewRouterFactory(staticConfig, managerFactory, tlsManager, nil, nil, dialerManager)
	require.NoError(t, err)

	handlers, _ := factory.CreateRouters(runtime.NewConfig(agg))
	require.NotNil(t, handlers[verifyEP])

	return agg, handlers[verifyEP]
}

func verifyDumpRouters(t *testing.T, agg dynamic.Configuration) {
	t.Helper()

	names := make([]string, 0, len(agg.HTTP.Routers))
	for name := range agg.HTTP.Routers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rt := agg.HTTP.Routers[name]
		opts, resolved, conflicting := "-", "-", false
		if rt.TLS != nil {
			opts, resolved, conflicting = rt.TLS.Options, rt.TLS.ResolvedOptions, rt.TLS.ConflictingOptions
		}
		t.Logf("VERIFY-C2 ROUTER %s rule=%s prio=%d middlewares=%v service=%s tls.options=%q resolvedOptions=%q conflictingOptions=%t",
			name, rt.Rule, rt.Priority, rt.Middlewares, rt.Service, opts, resolved, conflicting)
	}
	// Marshalled so that the probe also builds on branches without the MisdirectedRequest middleware.
	for name, mw := range agg.HTTP.Middlewares {
		if raw, err := json.Marshal(mw); err == nil && strings.Contains(string(raw), "misdirectedRequest") {
			t.Logf("VERIFY-C2 MIDDLEWARE %s %s", name, raw)
		}
	}
}

type verifyRequest struct{ sni, host string }

var verifyRequests = []verifyRequest{
	{"abc.example.com", "abc.example.com"},      // Control: exact listener, same SNI and Host.
	{"xyz.example.com", "xyz.example.com"},      // Control: catch-all listener, same SNI and Host.
	{"xyz.example.com", "abc.example.com"},      // Control: crossed selection without trailing dot.
	{"xyz.example.com", "abc.example.com."},     // Claim: catch-all SNI, trailing-dot exact Host.
	{"xyz.example.com", "ABC.Example.COM.:443"}, // Claim variant: mixed case, port.
	{"", "abc.example.com."},                    // Claim variant: no SNI.
	{"abc.example.com", "xyz.example.com."},     // Reverse crossing with trailing dot.
}

func TestVerifyC2Reachability(t *testing.T) {
	eps := map[string]gateway.Entrypoint{verifyEP: {Address: ":443"}}

	routes := []string{
		gateway.EvalIsoRouteWithServiceExported("http-app-catchall", "my-gateway", "catchall", "/app", "whoami-catchall"),
		gateway.EvalIsoRouteWithServiceExported("http-app-precise", "my-gateway", "precise", "/app", "whoami-precise"),
	}

	loadDistinctCerts := func(t *testing.T) *dynamic.Configuration {
		t.Helper()
		docs := append([]string{evalIsoMultiBackendYAML, gateway.EvalIsoClassYAML, verifyDistinctCertGateway}, routes...)
		return gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{
			gateway.EvalIsoNamespacedTLSSecretExported(t, "default", "cert-catchall", "catchall-listener.test", "*.example.com"),
			gateway.EvalIsoNamespacedTLSSecretExported(t, "default", "cert-precise", "abc.example.com"),
		}, docs...)
	}

	// setTLSOptions attaches the named TLS options to the routers generated for the given HTTPRoute.
	setTLSOptions := func(t *testing.T, conf *dynamic.Configuration, routeName, optionsName string, options traefiktls.Options) {
		t.Helper()
		if conf.TLS.Options == nil {
			conf.TLS.Options = map[string]traefiktls.Options{}
		}
		conf.TLS.Options[optionsName] = options
		var found int
		for name, rt := range conf.HTTP.Routers {
			if strings.Contains(name, routeName) && rt.TLS != nil {
				rt.TLS.Options = optionsName
				found++
			}
		}
		require.Positive(t, found)
	}

	scenarios := []struct {
		name string
		load func(t *testing.T) *dynamic.Configuration
	}{
		{
			// The provider output for the fixture's own exact + catch-all HTTPS Gateway (one shared certificate).
			name: "A-provider-output-shared-cert",
			load: func(t *testing.T) *dynamic.Configuration {
				t.Helper()
				docs := append([]string{evalIsoMultiBackendYAML, gateway.EvalIsoClassYAML,
					gateway.EvalIsoHTTPSGatewayExported("my-gateway", "supersecret", "catchall=", "precise=abc.example.com")}, routes...)
				return gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{gateway.EvalIsoTLSSecretExported(t, "supersecret")}, docs...)
			},
		},
		{
			// The unmodified provider output when each listener references its own certificate.
			name: "B-provider-output-distinct-listener-certs",
			load: loadDistinctCerts,
		},
		{
			// Provider output of B, then the exact listener's routers are given dedicated TLS options
			// (TLS 1.3 only) while the catch-all listener keeps the default ones.
			name: "C-injected-distinct-tls-options-exact-only",
			load: func(t *testing.T) *dynamic.Configuration {
				t.Helper()
				conf := loadDistinctCerts(t)
				setTLSOptions(t, conf, "http-app-precise", "exact-listener-tls13", traefiktls.Options{MinVersion: "VersionTLS13"})
				return conf
			},
		},
		{
			// Provider output of B, then both listeners' routers are given their own TLS options.
			name: "D-injected-distinct-tls-options-both",
			load: func(t *testing.T) *dynamic.Configuration {
				t.Helper()
				conf := loadDistinctCerts(t)
				setTLSOptions(t, conf, "http-app-precise", "exact-listener-tls13", traefiktls.Options{MinVersion: "VersionTLS13"})
				setTLSOptions(t, conf, "http-app-catchall", "catchall-listener-tls12", traefiktls.Options{MinVersion: "VersionTLS12", MaxVersion: "VersionTLS12"})
				return conf
			},
		},
	}

	for _, scenario := range scenarios {
		for _, strict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/strictTLSOptions=%t", scenario.name, strict), func(t *testing.T) {
				// 1. Fixture-equivalent dispatch: SNI-selected TLS options, then the production HTTPS handler chain.
				agg, router := verifyBuildRouters(t, scenario.load(t), strict, &static.EntryPoint{})
				verifyDumpRouters(t, agg)
				router.SetHTTPSForwarder(evalNoopTCP{})
				selectTLS := router.HTTP3TLSConfigMatcherFunc()

				// The byte-exact fixture harness, as a cross-check of the copy above.
				fixtureServe := evalServeHTTPSFull(t, scenario.load(t), nil, verifyEP, strict)

				for _, r := range verifyRequests {
					cd, err := tcpmuxer.NewConnData(r.sni, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4242}, nil)
					require.NoError(t, err)
					_, optionsName, err := selectTLS(cd)
					require.NoError(t, err)

					req := httptest.NewRequest(http.MethodGet, "https://"+r.host+"/app", nil)
					req.TLS = &tls.ConnectionState{ServerName: r.sni}
					req = req.WithContext(tcp.AddTLSOptionsNameInContext(context.WithoutCancel(req.Context()), optionsName))
					rec := httptest.NewRecorder()
					requestdecorator.New(nil).ServeHTTP(rec, req, router.GetHTTPSHandler().ServeHTTP)

					fx := fixtureServe(r.sni, r.host, "/app")

					t.Logf("VERIFY-C2 REQ mode=handler-chain SNI=%-16q Host=%-22q sniSelectedTLSOptions=%-40q -> %d %-26s | fixture evalServeHTTPSFull -> %d %s",
						r.sni, r.host, optionsName, rec.Code, verifyBackend(rec.Body.String()), fx.Code, verifyBackend(fx.Body))
				}

				// 2. Real TLS connections to a production TCP entry point serving the same routers.
				verifyRealTLS(t, scenario.load(t), strict)
			})
		}
	}
}

// verifyRealTLS starts a real Traefik TCP entry point on a loopback port, switches it to the routers
// built from conf, and sends each request over a real TLS connection carrying the given SNI.
func verifyRealTLS(t *testing.T, conf *dynamic.Configuration, strict bool) {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())

	epConfig := &static.EntryPoint{}
	epConfig.SetDefaults()
	epConfig.Address = addr

	_, router := verifyBuildRouters(t, conf, strict, epConfig)

	entryPoints, err := server.NewTCPEntryPoints(static.EntryPoints{verifyEP: epConfig}, nil, metrics.NewVoidRegistry())
	require.NoError(t, err)
	entryPoints.Switch(map[string]*tcprouter.Router{verifyEP: router})
	entryPoints.Start()
	t.Cleanup(entryPoints.Stop)

	for _, maxVersion := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
		for _, r := range verifyRequests {
			tlsConf := &tls.Config{ServerName: r.sni, InsecureSkipVerify: true, MaxVersion: maxVersion, NextProtos: []string{"h2", "http/1.1"}}

			var state tls.ConnectionState
			client := &http.Client{
				Timeout: 5 * time.Second,
				Transport: &http.Transport{
					ForceAttemptHTTP2: true,
					DialTLSContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
						var conn net.Conn
						var dialErr error
						// The entry point goroutine may not be accepting yet.
						for range 50 {
							conn, dialErr = (&tls.Dialer{Config: tlsConf}).DialContext(ctx, "tcp", addr)
							if dialErr == nil || !strings.Contains(dialErr.Error(), "connection refused") {
								break
							}
							time.Sleep(20 * time.Millisecond)
						}
						if dialErr != nil {
							return nil, dialErr
						}
						state = conn.(*tls.Conn).ConnectionState()
						return conn, nil
					},
				},
			}

			req, err := http.NewRequest(http.MethodGet, "https://"+addr+"/app", nil)
			require.NoError(t, err)
			req.Host = r.host

			prefix := fmt.Sprintf("VERIFY-C2 REQ mode=real-tls-entrypoint clientMaxTLS=%s SNI=%-16q Host=%-22q", tls.VersionName(maxVersion), r.sni, r.host)

			resp, err := client.Do(req)
			if err != nil {
				t.Logf("%s -> ERROR %v", prefix, err)
				continue
			}
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			client.CloseIdleConnections()

			var certNames []string
			if len(state.PeerCertificates) > 0 {
				certNames = slices.Clone(state.PeerCertificates[0].DNSNames)
			}
			t.Logf("%s negotiated=%s/%s servedCertDNSNames=%v -> %d %s",
				prefix, tls.VersionName(state.Version), resp.Proto, certNames, resp.StatusCode, verifyBackend(string(body)))
		}
	}
}
