//go:build verify

// Run (needs the eval fixtures export_eval_test.go and eval_provider_to_builder_runtime_test.go in this dir):
// cp verify/repro/verify_c2_test.go pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -run TestVerifyC2 -v ./pkg/provider/kubernetes/crd
package crd_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
)

func TestVerifyC2_AuthDiagnosticsOmitURN(t *testing.T) {
	cases := []struct {
		name    string
		entries []string
	}{
		{"default-basicauth", []string{"user1:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/", "badsentinel", "user2:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"}},
		{"default-digestauth", []string{"user1:traefik:0123456789abcdef0123456789abcdef", "badsentinel", "user2:traefik:0123456789abcdef0123456789abcdef"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, conf, provider := crd.LoadConfigurationWithSecretUpdate(t,
				[]string{"services.yml", "with_auth.yml"}, "authsecret", "default", "users", strings.Join(tc.entries, "\n"))
			require.NotNil(t, conf)
			_, rtConf, err := evalBuildHandler(t, conf, provider, nil, tc.name)
			require.Error(t, err)
			info := evalMiddlewareInfo(rtConf, tc.name)
			require.NotNil(t, info)
			if info.BasicAuth != nil {
				t.Logf("published users: %q", info.BasicAuth.Users)
			}
			if info.DigestAuth != nil {
				t.Logf("published users: %q", info.DigestAuth.Users)
			}
			t.Logf("returned error: %q", err.Error())
			t.Logf("error chain texts: %q", evalErrorTexts(err))
			t.Logf("stored info.Err: %q", info.Err)
			all := err.Error() + " " + strings.Join(info.Err, " ")
			t.Logf("contains 'urn:k8s:secret:authsecret:users': %v", strings.Contains(all, "urn:k8s:secret:authsecret:users"))
			t.Logf("contains 'badsentinel': %v", strings.Contains(all, "badsentinel"))
		})
	}
}
