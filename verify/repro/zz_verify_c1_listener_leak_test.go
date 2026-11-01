// Run: cp verify/repro/zz_verify_c1_listener_leak_test.go pkg/server/ && go test -count=1 -race -timeout=60s ./pkg/server -run '^TestVerifyC1_TCPTestLeaksListener$' -v
// Audits TestConnectTCPRoutingUnaffected (branch evalon/traefik-en-668b3ecf): runs it as a subtest, then after its
// cleanup has completed, diffs the LISTEN sockets owned by this process and dials any new one to prove the entrypoint
// listener is still open and still accepting connections.
package server

import (
	"bufio"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// processListenSockets returns the local addresses of the LISTEN TCP sockets whose inode is held by an fd of this process.
func processListenSockets(t *testing.T) map[string]struct{} {
	t.Helper()

	inodes := map[string]struct{}{}
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)
	for _, e := range entries {
		target, err := os.Readlink("/proc/self/fd/" + e.Name())
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "socket:[") {
			inodes[strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")] = struct{}{}
		}
	}

	out := map[string]struct{}{}
	for _, table := range []string{"/proc/self/net/tcp", "/proc/self/net/tcp6"} {
		f, err := os.Open(table)
		require.NoError(t, err)
		sc := bufio.NewScanner(f)
		sc.Scan() // header
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 10 || fields[3] != "0A" {
				continue
			}
			if _, ok := inodes[fields[9]]; !ok {
				continue
			}
			hp := strings.Split(fields[1], ":")
			ipHex, portHex := hp[0], hp[1]
			ip := make(net.IP, len(ipHex)/2)
			// Each 32-bit word is stored little-endian.
			for w := 0; w < len(ipHex)/8; w++ {
				for i := range 4 {
					v, _ := strconv.ParseUint(ipHex[w*8+i*2:w*8+i*2+2], 16, 8)
					ip[w*4+3-i] = byte(v)
				}
			}
			port, _ := strconv.ParseUint(portHex, 16, 16)
			out[net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))] = struct{}{}
		}
		_ = f.Close()
	}
	return out
}

func TestVerifyC1_TCPTestLeaksListener(t *testing.T) {
	before := processListenSockets(t)
	t.Logf("LISTEN sockets owned by process before: %v", before)

	ok := t.Run("TestConnectTCPRoutingUnaffected", TestConnectTCPRoutingUnaffected)
	require.True(t, ok, "the audited test itself must pass")
	// t.Run only returns once the subtest and all its t.Cleanup callbacks have completed.

	after := processListenSockets(t)
	t.Logf("LISTEN sockets owned by process after test+cleanup: %v", after)

	var leaked []string
	for addr := range after {
		if _, had := before[addr]; !had {
			leaked = append(leaked, addr)
		}
	}
	t.Logf("new LISTEN sockets still open after the audited test completed: %v", leaked)

	for _, addr := range leaked {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err != nil {
			t.Logf("dial %s failed: %v", addr, err)
			continue
		}
		// Prove the Start() accept loop is still serving: the TCP router echoes bytes back.
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, err = conn.Write([]byte("PING\r\n"))
		require.NoError(t, err)
		buf := make([]byte, 6)
		n, rerr := conn.Read(buf)
		t.Logf("dial %s ok; wrote PING, read %q err=%v (echo => the leaked entrypoint still accepts and routes)", addr, buf[:n], rerr)
		_ = conn.Close()
	}

	require.NotEmpty(t, leaked, "no listener leaked: claim would be REFUTED")
	t.Logf("VERDICT: listener %v leaked by TestConnectTCPRoutingUnaffected", leaked)
}
