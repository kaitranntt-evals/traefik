// Run (on a worktree of evalon/traefik-se-df0ea15d): cp verify/probes/auth_diag_probe_test.go <worktree>/pkg/middlewares/auth/ && cd <worktree> && go test -count=1 -v -run '^TestVerifyAuthDiag' ./pkg/middlewares/auth
package auth

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

func TestVerifyAuthDiag(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	const marker = "CONFIDENTIAL"

	for _, in := range []string{"CONFIDENTIALnocolon", "u:CONFIDENTIALhash:extra", "urn:k8s:secret:CONFIDENTIALname:key", ""} {
		_, _, err := basicUserParser(in)
		t.Logf("basicUserParser(%q) err=%q leaks=%v", in, err, err != nil && strings.Contains(err.Error(), marker))
	}
	for _, in := range []string{"CONFIDENTIALnocolon", "u:CONFIDENTIALhash", "a:b:c:CONFIDENTIAL", "urn:k8s:secret:CONFIDENTIALname:key"} {
		_, _, err := digestUserParser(in)
		t.Logf("digestUserParser(%q) err=%q leaks=%v", in, err, err != nil && strings.Contains(err.Error(), marker))
	}

	for _, idx := range []int{0, 1, 2} {
		basic := slices.Insert([]string{"alice:h1", "bob:h2"}, idx, "CONFIDENTIALbad")
		_, err := NewBasic(t.Context(), next, dynamic.BasicAuth{Users: basic}, "probe")
		t.Logf("BasicAuth malformed at index %d: err=%q leaks=%v", idx, err, strings.Contains(err.Error(), marker))

		digest := slices.Insert([]string{"alice:r:h1", "bob:r:h2"}, idx, "CONFIDENTIALbad")
		_, err = NewDigest(t.Context(), next, dynamic.DigestAuth{Users: digest}, "probe")
		t.Logf("DigestAuth malformed at index %d: err=%q leaks=%v", idx, err, strings.Contains(err.Error(), marker))
	}
}
