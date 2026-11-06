// Run (audited branch): cp verify/repro/c1c2_inner_error_test.go pkg/middlewares/auth/ && go test -tags verify -count=1 -v -run '^TestVerify_C1C2_InnerError' ./pkg/middlewares/auth

//go:build verify

package auth

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

// Shows the position that the auth constructor reports before the builder's RedactURNRefs replaces the error.
func TestVerify_C1C2_InnerError(t *testing.T) {
	for _, badIdx := range []int{0, 2} {
		users := []string{"alice:hashalpha", "bob:hashbravo"}
		users = append(users[:badIdx], append([]string{"malformedentry_x"}, users[badIdx:]...)...)
		_, err := NewBasic(t.Context(), http.NotFoundHandler(), dynamic.BasicAuth{Users: users}, "test")
		require.Error(t, err)
		fmt.Printf("[inner badIdx=%d] NewBasic error: %s\n", badIdx, err)
	}
}
