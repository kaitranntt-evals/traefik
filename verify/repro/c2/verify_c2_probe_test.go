// Verify-run v-16e29b76 probe for claim C2. Run from the repo root (branch evalon/traefik-en-0f9b28ca) with:
//
//	cp verify/repro/c2/verify_c2_probe_test.go pkg/server/service/ && go test -tags verify_c2 -count=1 -v ./pkg/server/service -run '^TestVerifyC2_'
//
// It checks which RoundTripper the proxy built by serveProxy uses, and enumerates the TCP sockets owned by the
// test process (via /proc/self/fd + /proc/self/net/tcp{,6}) before and after TestConnect_Rejected runs.

//go:build verify_c2

package service

import (
	"bufio"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func procTCPSockets(t *testing.T) map[string]string {
	t.Helper()

	inodes := map[string]bool{}
	fds, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
		if err == nil && strings.HasPrefix(target, "socket:[") {
			inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}

	out := map[string]string{}
	for _, table := range []string{"/proc/self/net/tcp", "/proc/self/net/tcp6"} {
		f, err := os.Open(table)
		require.NoError(t, err)
		sc := bufio.NewScanner(f)
		sc.Scan()
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) >= 10 && inodes[fields[9]] {
				out[fields[9]] = fmt.Sprintf("local=%s remote=%s state=%s", fields[1], fields[2], fields[3])
			}
		}
		_ = f.Close()
	}
	return out
}

// TestVerifyC2_ServeProxyTransportIdentity shows that the proxy handler built the way serveProxy builds it
// uses http.DefaultTransport itself (pointer identity), i.e. no new transport is created.
func TestVerifyC2_ServeProxyTransportIdentity(t *testing.T) {
	handler, err := buildProxy(new(true), nil, http.DefaultTransport, newBufferPool())
	require.NoError(t, err)

	// On this branch buildProxy wraps the reverse proxy in connectHandler and h2cUpgradeHandler; unwrap them.
	for {
		switch h := handler.(type) {
		case *connectHandler:
			t.Logf("unwrapping %T", h)
			handler = h.next
			continue
		case *h2cUpgradeHandler:
			t.Logf("unwrapping %T", h)
			handler = h.next
			continue
		}
		break
	}
	rp, ok := handler.(*httputil.ReverseProxy)
	require.True(t, ok, "buildProxy returns %T", handler)

	t.Logf("proxy.Transport == http.DefaultTransport: %v (proxy.Transport=%p, http.DefaultTransport=%p)",
		rp.Transport == http.DefaultTransport, rp.Transport, http.DefaultTransport)
	require.Same(t, http.DefaultTransport, rp.Transport)
}

// TestVerifyC2_WrappedTestSockets runs TestConnect_Rejected (every serveProxy caller on the branch) as a subtest and
// reports TCP sockets and goroutines that survive its teardown.
func TestVerifyC2_WrappedTestSockets(t *testing.T) {
	before := procTCPSockets(t)
	goroutinesBefore := runtime.NumGoroutine()

	t.Run("TestConnect_Rejected", TestConnect_Rejected)

	// Give closed listeners/connections a moment to leave the fd table.
	time.Sleep(200 * time.Millisecond)
	after := procTCPSockets(t)

	var leaked []string
	for inode, desc := range after {
		if _, ok := before[inode]; !ok {
			leaked = append(leaked, "inode="+inode+" "+desc)
		}
	}
	t.Logf("TCP sockets before=%d after=%d; surviving teardown: %d", len(before), len(after), len(leaked))
	for _, l := range leaked {
		t.Logf("SURVIVING %s", l)
	}
	t.Logf("goroutines before=%d after=%d", goroutinesBefore, runtime.NumGoroutine())
}
