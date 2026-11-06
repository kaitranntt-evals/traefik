// Run: cp verify/repro/c1c2_auth_position_test.go pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -v -run '^TestVerify_C1C2_' ./pkg/provider/kubernetes/crd
// Requires the eval fixtures (export_eval_test.go, eval_provider_to_builder_runtime_test.go) in pkg/provider/kubernetes/crd/.

//go:build verify

package crd_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
)

// Malformed Secret-backed entries placed at index 0 and index 2; dump returned error and stored info.Err.
func TestVerify_C1C2_SecretBackedAuthPosition(t *testing.T) {
	const bad = "malformedentry_leaked_secret_hash_zz"
	for _, mw := range []struct{ name, good1, good2 string }{
		{"default-basicauth", "alice:hashalpha", "bob:hashbravo"},
		{"default-digestauth", "alice:traefik:hashalpha", "bob:traefik:hashbravo"},
	} {
		for _, badIdx := range []int{0, 2} {
			users := []string{mw.good1, mw.good2}
			users = append(users[:badIdx], append([]string{bad}, users[badIdx:]...)...)
			_, confAfter, provider := crd.LoadConfigurationWithSecretUpdate(t,
				[]string{"services.yml", "with_auth.yml"}, "authsecret", "default", "users", strings.Join(users, "\n")+"\n")
			require.NotNil(t, confAfter)
			_, rtConf, err := evalBuildHandler(t, confAfter, provider, nil, mw.name)
			require.Error(t, err)
			info := evalMiddlewareInfo(rtConf, mw.name)
			require.NotNil(t, info)
			if m := confAfter.HTTP.Middlewares[mw.name]; m != nil {
				if m.BasicAuth != nil {
					fmt.Printf("[%s badIdx=%d] dynamic users=%v\n", mw.name, badIdx, m.BasicAuth.Users)
				} else if m.DigestAuth != nil {
					fmt.Printf("[%s badIdx=%d] dynamic users=%v\n", mw.name, badIdx, m.DigestAuth.Users)
				}
			}
			fmt.Printf("[%s badIdx=%d] returned err: %s\n", mw.name, badIdx, err.Error())
			for _, e := range info.Err {
				fmt.Printf("[%s badIdx=%d] stored info.Err: %s\n", mw.name, badIdx, e)
			}
			for _, txt := range append(evalErrorTexts(err), info.Err...) {
				require.NotContains(t, txt, bad)
			}
		}
	}
}
