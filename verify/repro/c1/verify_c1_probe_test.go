// Verify-run v-16e29b76 probe for claim C1. Run from the repo root with:
//
//	cp verify/repro/c1/verify_c1_probe_test.go verify/repro/c1/verify_c1_wrapped_ec41c631_test.go pkg/server/ && go test -tags verify_c1 -count=1 -v ./pkg/server -run '^TestVerifyC1_'
//
// It enumerates the UDP sockets owned by the test process (via /proc/self/fd + /proc/self/net/udp{,6})
// before and after the audited HTTP/3 tests run, and probes the entrypoint teardown directly.

//go:build verify_c1

package server

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v2/pkg/config/static"
	tcprouter "github.com/traefik/traefik/v2/pkg/server/router/tcp"
	traefiktls "github.com/traefik/traefik/v2/pkg/tls"
)

// procUDPSockets returns inode -> local address for every UDP socket held by an fd of this process.
func procUDPSockets(t *testing.T) map[string]string {
	t.Helper()

	inodes := map[string]bool{}
	fds, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", fd.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "socket:[") {
			inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = true
		}
	}

	out := map[string]string{}
	for _, table := range []string{"/proc/self/net/udp", "/proc/self/net/udp6"} {
		f, err := os.Open(table)
		require.NoError(t, err)
		sc := bufio.NewScanner(f)
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 {
				continue
			}
			inode := fields[9]
			if inodes[inode] {
				out[inode] = decodeProcAddr(fields[1])
			}
		}
		_ = f.Close()
	}
	return out
}

func decodeProcAddr(s string) string {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return s
	}
	raw, err := hex.DecodeString(parts[0])
	if err != nil {
		return s
	}
	port, _ := strconv.ParseUint(parts[1], 16, 16)
	// /proc encodes each 32-bit word little-endian.
	for i := 0; i+4 <= len(raw); i += 4 {
		raw[i], raw[i+1], raw[i+2], raw[i+3] = raw[i+3], raw[i+2], raw[i+1], raw[i]
	}
	return net.JoinHostPort(net.IP(raw).String(), strconv.FormatUint(port, 10))
}

func diffSockets(before, after map[string]string) []string {
	var leaked []string
	for inode, addr := range after {
		if _, ok := before[inode]; !ok {
			leaked = append(leaked, fmt.Sprintf("inode=%s local=%s", inode, addr))
		}
	}
	return leaked
}

// TestVerifyC1_WrappedTests runs the branch's HTTP/3 tests as subtests (so their t.Cleanup teardown completes
// before the check) and reports every process-owned UDP socket that survives their teardown.
func TestVerifyC1_WrappedTests(t *testing.T) {
	for name, fn := range c1WrappedTests {
		before := procUDPSockets(t)
		t.Run(name, fn)
		after := procUDPSockets(t)
		leaked := diffSockets(before, after)
		t.Logf("[%s] UDP sockets before=%d after=%d; surviving teardown: %d", name, len(before), len(after), len(leaked))
		for _, l := range leaked {
			t.Logf("[%s] LEAKED %s", name, l)
		}
		// Show whether the only remaining release path is the garbage collector finalizer on the netFD.
		runtime.GC()
		runtime.GC()
		time.Sleep(200 * time.Millisecond)
		afterGC := procUDPSockets(t)
		t.Logf("[%s] after runtime.GC(): surviving: %d", name, len(diffSockets(before, afterGC)))
	}
}

// TestVerifyC1_ShutdownLeavesHTTP3ConnOpen probes the owner teardown path alone: NewTCPEntryPoint with HTTP3,
// Start, Shutdown, then checks whether the server UDP PacketConn (http3conn) is still open.
func TestVerifyC1_ShutdownLeavesHTTP3ConnOpen(t *testing.T) {
	certContent, err := localhostCert.Read()
	require.NoError(t, err)
	keyContent, err := localhostKey.Read()
	require.NoError(t, err)
	tlsCert, err := tls.X509KeyPair(certContent, keyContent)
	require.NoError(t, err)

	epConfig := &static.EntryPointsTransport{}
	epConfig.SetDefaults()

	before := procUDPSockets(t)

	entryPoint, err := NewTCPEntryPoint(context.Background(), &static.EntryPoint{
		Address:          "127.0.0.1:0",
		Transport:        epConfig,
		ForwardedHeaders: &static.ForwardedHeaders{},
		HTTP2:            &static.HTTP2Config{},
		HTTP3:            &static.HTTP3Config{},
	}, nil)
	require.NoError(t, err)

	router, err := tcprouter.NewRouter()
	require.NoError(t, err)
	router.AddHTTPTLSConfig("*", &tls.Config{Certificates: []tls.Certificate{tlsCert}}, traefiktls.DefaultTLSConfigName)

	go entryPoint.Start(context.Background())
	entryPoint.SwitchRouter(router)
	time.Sleep(500 * time.Millisecond)

	udpAddr := entryPoint.http3Server.http3conn.LocalAddr().String()
	t.Logf("server UDP socket (http3conn) bound at %s", udpAddr)

	entryPoint.Shutdown(context.Background())
	time.Sleep(200 * time.Millisecond)

	// SetReadDeadline on a closed net.UDPConn returns "use of closed network connection".
	err = entryPoint.http3Server.http3conn.SetReadDeadline(time.Now().Add(time.Second))
	t.Logf("after entryPoint.Shutdown: http3conn.SetReadDeadline error = %v (nil means the socket is still open)", err)

	after := procUDPSockets(t)
	for _, l := range diffSockets(before, after) {
		t.Logf("after entryPoint.Shutdown: process still holds UDP socket %s", l)
	}

	cerr := entryPoint.http3Server.http3conn.Close()
	t.Logf("explicit http3conn.Close() error = %v (nil means this call is what actually released it)", cerr)
	for _, l := range diffSockets(before, procUDPSockets(t)) {
		t.Logf("after explicit Close: process still holds UDP socket %s", l)
	}
	if err == nil && cerr == nil {
		t.Logf("RESULT: entryPoint.Shutdown did NOT close the HTTP/3 server UDP socket; only the explicit Close released it")
	}
}
