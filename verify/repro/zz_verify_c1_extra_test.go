// Run: cp verify/repro/zz_verify_c1_*_test.go pkg/server/ && go test -count=1 -race -timeout=60s ./pkg/server -run '^TestVerifyC1_(PreexistingPatternAlsoLeaks|ShutdownReleasesListener)$' -v
// Context for C1: shows the pre-existing startEntrypoint pattern leaks too, and that entryPoint.Shutdown releases the raw listener (the fix).
package server

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/config/static"
	tcprouter "github.com/traefik/traefik/v2/pkg/server/router/tcp"
)

// Pre-existing test using the same startEntrypoint pattern, for context.
func TestVerifyC1_PreexistingPatternAlsoLeaks(t *testing.T) {
	before := processListenSockets(t)
	t.Run("TestReadTimeoutWithoutFirstByte", TestReadTimeoutWithoutFirstByte)
	after := processListenSockets(t)
	for a := range after {
		if _, had := before[a]; !had {
			t.Logf("pre-existing TestReadTimeoutWithoutFirstByte also leaves LISTEN socket open: %s", a)
		}
	}
}

// Does entryPoint.Shutdown release the raw entrypoint listener? (Candidate fix for the added test.)
func TestVerifyC1_ShutdownReleasesListener(t *testing.T) {
	before := processListenSockets(t)
	ok := t.Run("withShutdown", func(t *testing.T) {
		router, err := tcprouter.NewRouter()
		require.NoError(t, err)
		epConfig := &static.EntryPointsTransport{}
		epConfig.SetDefaults()
		entryPoint, err := NewTCPEntryPoint(t.Context(), &static.EntryPoint{
			Address: "127.0.0.1:0", Transport: epConfig,
			ForwardedHeaders: &static.ForwardedHeaders{}, HTTP2: &static.HTTP2Config{},
		}, nil)
		require.NoError(t, err)
		conn, err := startEntrypoint(t, entryPoint, router)
		require.NoError(t, err)
		t.Cleanup(func() { _ = conn.Close() })
		t.Cleanup(func() { entryPoint.Shutdown(t.Context()) })
		t.Logf("listener addr: %s", entryPoint.listener.Addr())
	})
	require.True(t, ok)
	time.Sleep(200 * time.Millisecond)
	after := processListenSockets(t)
	for a := range after {
		if _, had := before[a]; !had {
			t.Logf("STILL OPEN after Shutdown: %s", a)
			c, err := net.DialTimeout("tcp", a, time.Second)
			t.Logf("dial after Shutdown: err=%v", err)
			if err == nil {
				c.Close()
			}
		}
	}
	t.Logf("before=%v after=%v", before, after)
}
