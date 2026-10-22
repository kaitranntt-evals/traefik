// Run: cp verify/repro/c1_header_policy_connect_test.go pkg/server/ && go test -count=1 -timeout=60s ./pkg/server -run '^TestAuditC1_' -v
// Live probe: starts the entrypoint HTTP stack (createHTTPServer) with a reject header policy and sends CONNECT
// requests carrying an underscore header (HTTP/1.1 raw TCP and h2c) plus a CONNECT whose target carries a fragment.
// Expected contract: every CONNECT answered 501; a 400 proves header policy / fragment validation preempted denyConnect.
package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/config/static"
	"github.com/traefik/traefik/v2/pkg/middlewares/requestdecorator"
	"golang.org/x/net/http2"
)

func auditC1Server(t *testing.T, configure func(*static.EntryPoint)) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	configuration := &static.EntryPoint{}
	configuration.SetDefaults()
	configure(configuration)

	srv, err := createHTTPServer(t.Context(), ln, configuration, true, requestdecorator.New(nil))
	require.NoError(t, err)

	srv.Switcher.UpdateHandler(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("X-Downstream-Called", "true")
		rw.WriteHeader(http.StatusOK)
	}))

	go func() { _ = srv.Server.Serve(ln) }()

	return ln.Addr().String()
}

func auditC1Raw(t *testing.T, addr, raw string) *http.Response {
	t.Helper()

	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = io.WriteString(conn, raw)
	require.NoError(t, err)

	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err)
	_ = resp.Body.Close()

	return resp
}

func TestAuditC1_HeaderPolicy(t *testing.T) {
	policies := map[string]func(*static.EntryPoint){
		"aliasHeadersStrategy=reject": func(ep *static.EntryPoint) { ep.HTTP.AliasHeadersStrategy = static.AliasHeadersStrategyReject },
		"underscoreHeadersStrategy=reject": func(ep *static.EntryPoint) {
			ep.HTTP.AliasHeadersStrategy = ""
			ep.HTTP.UnderscoreHeadersStrategy = static.UnderscoreHeadersStrategyReject
		},
	}

	for name, configure := range policies {
		t.Run(name, func(t *testing.T) {
			addr := auditC1Server(t, configure)

			resp := auditC1Raw(t, addr, "CONNECT backend.example.com:443 HTTP/1.1\r\nHost: backend.example.com:443\r\nX_Test: v\r\n\r\n")
			t.Logf("HTTP/1.1 CONNECT + X_Test header -> %d (downstream=%q)", resp.StatusCode, resp.Header.Get("X-Downstream-Called"))
			require.Equal(t, http.StatusNotImplemented, resp.StatusCode)

			client := &http.Client{Transport: &http2.Transport{
				AllowHTTP: true,
				DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, network, addr)
				},
			}}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodConnect, "http://"+addr, http.NoBody)
			require.NoError(t, err)
			req.Header.Set("X_Test", "v")
			res, err := client.Do(req)
			require.NoError(t, err)
			_ = res.Body.Close()
			t.Logf("h2c CONNECT + X_Test header -> %d (proto %s)", res.StatusCode, res.Proto)
			require.Equal(t, http.StatusNotImplemented, res.StatusCode)
		})
	}
}

func TestAuditC1_Fragment(t *testing.T) {
	addr := auditC1Server(t, func(*static.EntryPoint) {})

	for _, target := range []string{"/tunnel#frag", "http://backend.example.com/tunnel#frag"} {
		resp := auditC1Raw(t, addr, "CONNECT "+target+" HTTP/1.1\r\nHost: backend.example.com\r\n\r\n")
		t.Logf("HTTP/1.1 CONNECT %s -> %d", target, resp.StatusCode)
		require.Equal(t, http.StatusNotImplemented, resp.StatusCode, strings.TrimSpace(target))
	}
}
