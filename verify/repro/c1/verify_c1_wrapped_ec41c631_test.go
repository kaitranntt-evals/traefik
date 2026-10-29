// Verify-run v-16e29b76: branch evalon/traefik-en-ec41c631 wrapped HTTP/3 tests for TestVerifyC1_WrappedTests.

//go:build verify_c1

package server

import "testing"

var c1WrappedTests = map[string]func(*testing.T){
	"TestHTTP3DenyConnect": TestHTTP3DenyConnect,
}
