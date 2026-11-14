// Run: cp verify/repro/c7_position_test.go pkg/middlewares/auth/ && go test -count=1 -v -run '^TestVerifyC7_' ./pkg/middlewares/auth   (worktree of evalon/traefik-se-df0ea15d)
package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

const c7Marker = "CONFIDENTIALc7marker"

func c7Err(t *testing.T, digest bool, file string, users []string) string {
	t.Helper()
	var err error
	if digest {
		_, err = NewDigest(t.Context(), nil, dynamic.DigestAuth{UsersFile: file, Users: users}, "c7")
	} else {
		_, err = NewBasic(t.Context(), nil, dynamic.BasicAuth{UsersFile: file, Users: users}, "c7")
	}
	require.Error(t, err)
	return err.Error()
}

// The malformed entry is the FIRST inline user; a valid users file is also configured.
func TestVerifyC7_InlinePositionShiftedByFileEntries(t *testing.T) {
	for _, tc := range []struct {
		name           string
		digest         bool
		valid1, valid2 string
	}{
		{"basic", false, "a:h1", "b:h2"},
		{"digest", true, "a:r:h1", "b:r:h2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "valid_users.txt")
			require.NoError(t, os.WriteFile(file, []byte(tc.valid1+"\n"+tc.valid2+"\n"), 0o600))

			noFile := c7Err(t, tc.digest, "", []string{c7Marker, tc.valid1})
			withFile := c7Err(t, tc.digest, file, []string{c7Marker, tc.valid1})
			t.Logf("inline users[0] malformed, no users file      : %q", noFile)
			t.Logf("inline users[0] malformed, 2-line valid file  : %q", withFile)
			t.Logf("leaks marker: %v", strings.Contains(noFile+withFile, c7Marker))
			assert.Equal(t, noFile, withFile, "same malformed inline entry must be reported at the same position")
		})
	}
}

// The malformed entry is on line 4 of the users file; lines 2 and 3 are blank.
func TestVerifyC7_FilePositionIgnoresBlankLines(t *testing.T) {
	file := filepath.Join(t.TempDir(), "users.htpasswd")
	require.NoError(t, os.WriteFile(file, []byte("a:h1\n\n\n"+c7Marker+"\n"), 0o600))

	got := c7Err(t, false, file, nil)
	t.Logf("file line 4 malformed (lines 2-3 blank): %q", got)
	t.Logf("leaks marker: %v", strings.Contains(got, c7Marker))
	assert.Contains(t, got, "4", "the malformed entry is on line 4 of the file")
}
