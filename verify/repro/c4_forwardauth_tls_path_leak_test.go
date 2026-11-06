// Run: cd ~/repos/traefik && cp verify/repro/c4_forwardauth_tls_path_leak_test.go pkg/server/middleware/ && go test -count=1 -run TestVerifyC4 -v ./pkg/server/middleware
package middleware

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
)

func TestVerifyC4_ForwardAuthSecretDirPathInError(t *testing.T) {
	certDir := filepath.Join(t.TempDir(), "SECRET-CERT-DIR-xyz")
	keyDir := filepath.Join(t.TempDir(), "SECRET-KEY-DIR-abc")
	require.NoError(t, os.Mkdir(certDir, 0o700))
	require.NoError(t, os.Mkdir(keyDir, 0o700))

	configs := map[string]*runtime.MiddlewareInfo{
		"fa": {Middleware: &dynamic.Middleware{
			ForwardAuth: &dynamic.ForwardAuth{
				Address: "https://auth.example",
				TLS: &dynamic.ClientTLS{
					Cert: "urn:k8s:secret:tlssecret:tls.crt",
					Key:  "urn:k8s:secret:tlssecret:tls.key",
				},
			},
			URNRefs: map[string]string{
				"/forwardAuth/tls/cert": certDir,
				"/forwardAuth/tls/key":  keyDir,
			},
		}},
	}

	b := NewBuilder(configs, nil, nil)
	chain := b.BuildMiddlewareChain(t.Context(), []string{"fa"})
	_, err := chain.Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	require.Error(t, err)
	t.Logf("constructor error: %v", err)
	t.Logf("runtime MiddlewareInfo.Err (what /api/http/middlewares exposes): %v", configs["fa"].Err)
	t.Logf("dynamic config still holds refs: cert=%q key=%q", configs["fa"].ForwardAuth.TLS.Cert, configs["fa"].ForwardAuth.TLS.Key)
	require.Contains(t, err.Error(), "SECRET-CERT-DIR-xyz")
	require.Contains(t, configs["fa"].Err[0], "SECRET-CERT-DIR-xyz")
}
