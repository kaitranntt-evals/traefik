//go:build verify

// Run: cp verify/repro/c6_usersfile_probe_test.go <worktree>/pkg/middlewares/auth/verify_c6_usersfile_probe_test.go && (cd <worktree> && go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/middlewares/auth)

package auth

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

var vC6Number = regexp.MustCompile(`\d+`)

func TestVerify_C6_SingleEntryUsersFileNoTrailingNewline(t *testing.T) {
	const malformedBasic = "malformedentry_verify_hash"
	const malformedDigest = "malformedentry:verify_hash"

	for _, tc := range []struct {
		name    string
		content string
		digest  bool
		// single marks the claim's case; the others are controls showing the same probe sees a position.
		single bool
	}{
		{name: "basic: one malformed entry, NO trailing newline", content: malformedBasic, single: true},
		{name: "digest: one malformed entry, NO trailing newline", content: malformedDigest, digest: true, single: true},
		{name: "control basic: one malformed entry WITH trailing newline", content: malformedBasic + "\n"},
		{name: "control digest: one malformed entry WITH trailing newline", content: malformedDigest + "\n", digest: true},
		{name: "control basic: valid entry then malformed entry, no trailing newline", content: "alice:hashalpha\n" + malformedBasic},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "users.htpasswd")
			require.NoError(t, os.WriteFile(file, []byte(tc.content), 0o600))

			var err error
			if tc.digest {
				_, err = NewDigest(t.Context(), nil, dynamic.DigestAuth{UsersFile: file}, "authName")
			} else {
				_, err = NewBasic(t.Context(), nil, dynamic.BasicAuth{UsersFile: file}, "authName")
			}
			require.Error(t, err)

			text := err.Error()
			namesFile := strings.Contains(text, file)
			withoutPath := strings.ReplaceAll(text, file, "<file>")
			positions := vC6Number.FindAllString(withoutPath, -1)
			t.Logf("RESULT %s: file content %q -> error %q | names file = %v | position numbers outside the path = %v",
				tc.name, tc.content, withoutPath, namesFile, positions)

			assert.NotContains(t, text, "verify_hash")
			assert.True(t, namesFile)
			assert.NotEmpty(t, positions, "the diagnostic must identify the entry position")
		})
	}
}
