// Shared production-stack harness for the verify repros. Copy next to the provider sources (pkg/provider/kubernetes/gateway/).
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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/config/static"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	tcpmuxer "github.com/traefik/traefik/v3/pkg/muxer/tcp"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/service"
	"github.com/traefik/traefik/v3/pkg/tcp"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
)

type verifyProvider struct{ conf *dynamic.Configuration }

func (p *verifyProvider) Init() error { return nil }

func (p *verifyProvider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	ch <- dynamic.Message{ProviderName: "kubernetesgateway", Configuration: p.conf}
	return nil
}

// verifyProxy answers in place of the HTTP backends, naming the backend the request reached.
type verifyProxy struct{}

func (verifyProxy) Build(_ string, target *url.URL, _, _ bool, _ time.Duration) (http.Handler, error) {
	return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		_, _ = rw.Write([]byte("backend:" + target.Host))
	}), nil
}

func (verifyProxy) Update(map[string]*dynamic.ServersTransport) {}

// verifyForwarder stands for the entry point HTTPS server: it completes the TLS handshake and answers a marker.
type verifyForwarder struct{ fallback *tls.Config }

func (f verifyForwarder) ServeTCP(conn tcp.WriteCloser) {
	defer conn.Close()
	var c net.Conn = conn
	if _, ok := conn.(*tls.Conn); !ok {
		c = tls.Server(conn, f.fallback)
	}
	_, _ = c.Write([]byte("HTTPS-TERMINATED-BY-TRAEFIK"))
}

type verifyRuntime struct {
	Merged dynamic.Configuration
	// Serve dispatches an HTTPS request through the production SNI selection and HTTPS handler.
	Serve func(sni, host, path string) (code int, body, tlsOptions string)
	// DialTLS opens a real TLS connection served by the production Router.ServeTCP.
	DialTLS func(sni string) (peerCN, payload string, err error)
}

func verifyRuntimeFor(t *testing.T, conf *dynamic.Configuration, ep string, fallbackCert tls.Certificate) verifyRuntime {
	t.Helper()
	ctx := t.Context()

	merged := make(chan dynamic.Configuration, 1)
	pool := safe.NewPool(ctx)
	t.Cleanup(pool.Stop)
	watcher := server.NewConfigurationWatcher(pool, &verifyProvider{conf: conf}, []string{ep}, "", false)
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
	managerFactory := service.NewManagerFactory(staticConfig, nil, nil, transportManager, verifyProxy{}, nil, nil)
	dialerManager := tcp.NewDialerManager(nil)
	dialerManager.Update(map[string]*dynamic.TCPServersTransport{"default@internal": {}})
	factory, err := server.NewRouterFactory(staticConfig, managerFactory, tlsManager, nil, nil, dialerManager)
	require.NoError(t, err)

	handlers, _ := factory.CreateRouters(runtime.NewConfig(agg))
	router := handlers[ep]
	require.NotNil(t, router)
	router.SetHTTPSForwarder(verifyForwarder{fallback: &tls.Config{Certificates: []tls.Certificate{fallbackCert}}})
	selectTLS := router.HTTP3TLSConfigMatcherFunc()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go router.ServeTCP(c.(*net.TCPConn))
		}
	}()

	return verifyRuntime{
		Merged: agg,
		Serve: func(sni, host, path string) (int, string, string) {
			cd, err := tcpmuxer.NewConnData(sni, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 4242}, nil)
			require.NoError(t, err)
			_, optionsName, err := selectTLS(cd)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodGet, "https://"+host+path, nil)
			req.TLS = &tls.ConnectionState{ServerName: sni}
			req = req.WithContext(tcp.AddTLSOptionsNameInContext(context.WithoutCancel(req.Context()), optionsName))
			rec := httptest.NewRecorder()
			requestdecorator.New(nil).ServeHTTP(rec, req, router.GetHTTPSHandler().ServeHTTP)
			return rec.Code, rec.Body.String(), optionsName
		},
		DialTLS: func(sni string) (string, string, error) {
			d := &net.Dialer{Timeout: 5 * time.Second}
			c, err := tls.DialWithDialer(d, "tcp", ln.Addr().String(), &tls.Config{ServerName: sni, InsecureSkipVerify: true})
			if err != nil {
				return "", "", err
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(5 * time.Second))
			payload, _ := io.ReadAll(c)
			return c.ConnectionState().PeerCertificates[0].Subject.CommonName, string(payload), nil
		},
	}
}

// verifyDump prints the generated routers that take part in the SNI dispatch.
func verifyDump(t *testing.T, agg dynamic.Configuration) {
	t.Helper()
	var lines []string
	for name, r := range agg.HTTP.Routers {
		if r.TLS == nil {
			continue
		}
		lines = append(lines, fmt.Sprintf("HTTPS router %s rule=%s entryPoints=%v tls.options=%q resolvedOptions=%q", name, r.Rule, r.EntryPoints, r.TLS.Options, r.TLS.ResolvedOptions))
	}
	if agg.TCP != nil {
		for name, r := range agg.TCP.Routers {
			passthrough := r.TLS != nil && r.TLS.Passthrough
			lines = append(lines, fmt.Sprintf("TCP router %s rule=%s entryPoints=%v passthrough=%v service=%s", name, r.Rule, r.EntryPoints, passthrough, r.Service))
		}
	}
	sort.Strings(lines)
	for _, l := range lines {
		t.Log(l)
	}
}
