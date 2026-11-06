// Run: cd ~/repos/traefik && cp verify/repro/c7_loadkeypair_path_in_error_test.go pkg/types/ && go test -count=1 -run TestVerifyC4 -v ./pkg/types
package types

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifyC4_LoadKeyPairDirectoryPaths(t *testing.T) {
	certDir := filepath.Join(t.TempDir(), "SECRET-CERT-DIR-xyz")
	keyDir := filepath.Join(t.TempDir(), "SECRET-KEY-DIR-abc")
	require.NoError(t, os.Mkdir(certDir, 0o700))
	require.NoError(t, os.Mkdir(keyDir, 0o700))

	_, err := loadKeyPair(certDir, keyDir)
	require.Error(t, err)
	t.Logf("loadKeyPair error: %v", err)
	require.Contains(t, err.Error(), "SECRET-CERT-DIR-xyz")

	// Existing file path instead of PEM: an ordinary non-PEM value names an existing file.
	_, err = loadKeyPair("/etc/hostname", "/etc/hostname")
	t.Logf("loadKeyPair(/etc/hostname) error: %v", err)
}
