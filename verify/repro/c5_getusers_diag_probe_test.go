//go:build ignore

// Run: in a checkout of evalon/traefik-se-83e20fa3: sed '/^\/\/go:build ignore$/d' verify/repro/c5_getusers_diag_probe_test.go > pkg/middlewares/auth/c5_getusers_diag_probe_test.go && go test -count=1 -v -run 'TestProbe_C5' ./pkg/middlewares/auth
package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProbe_C5_GetUsersDiagnostics(t *testing.T) {
	dir := t.TempDir()
	// file_source_identification: malformed entry in a named users file.
	badFile := filepath.Join(dir, "named-users.htpasswd")
	require.NoError(t, os.WriteFile(badFile, []byte("alice:hashA\nMALFORMED_FILE_ENTRY\n"), 0o600))
	_, err := getUsers(badFile, nil, basicUserParser)
	t.Logf("FILE  malformed entry -> err=%v", err)

	// inline_source_position: valid file entries before a malformed inline entry.
	for _, n := range []int{0, 2, 4} {
		content := ""
		for i := range n {
			content += "fileuser" + string(rune('a'+i)) + ":hash\n"
		}
		goodFile := filepath.Join(dir, "good.htpasswd")
		require.NoError(t, os.WriteFile(goodFile, []byte(content), 0o600))
		_, err := getUsers(goodFile, []string{"MALFORMED_INLINE_ENTRY"}, basicUserParser)
		t.Logf("INLINE with %d preceding file entries -> err=%v", n, err)
	}
}
