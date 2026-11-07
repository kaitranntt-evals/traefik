//go:build verify

// Run: cp verify/repro/verify_c5_test.go pkg/server/middleware/ && go test -tags verify -count=1 -run TestVerifyC5 -v ./pkg/server/middleware
package middleware

import (
	"net/http"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
)

func TestVerifyC5_SecretValueUserLosesPosition(t *testing.T) {
	const ref = "urn:k8s:secret:authsecret:users"
	cases := []struct {
		desc   string
		digest bool
		value  string
	}{
		{"basic/value=user", false, "user"},
		{"basic/value=valid+user", false, "alice:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\nuser"},
		{"basic/control value=zzz-malformed", false, "zzz-malformed"},
		{"digest/value=user", true, "user"},
		{"digest/value=valid+user", true, "alice:traefik:0123456789abcdef0123456789abcdef\nuser"},
		{"digest/control value=zzz-malformed", true, "zzz-malformed"},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			m := &dynamic.Middleware{Secrets: map[string]string{ref: tc.value}}
			if tc.digest {
				m.DigestAuth = &dynamic.DigestAuth{Users: dynamic.Users{ref}}
			} else {
				m.BasicAuth = &dynamic.BasicAuth{Users: dynamic.Users{ref}}
			}
			rtConf := runtime.NewConfig(dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"auth": m}},
			})
			builder := NewBuilder(rtConf.Middlewares, nil, nil)
			_, err := builder.BuildMiddlewareChain(t.Context(), []string{"auth"}).Then(http.NotFoundHandler())
			t.Logf("returned error: %v", err)
			t.Logf("stored info.Err: %q", rtConf.Middlewares["auth"].Err)
		})
	}
}
