// Run: cp verify/repro/c5_auth_error_position_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC5 ./pkg/server/middleware/  (on branch evalon/traefik-se-374ebb45)

//go:build verify

package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
)

func TestVerifyC5_AuthErrorKeepsEntryPosition(t *testing.T) {
	const ref = "urn:k8s:secret:authsecret:users"

	for _, value := range []string{"user", "format", "zzz-unrelated"} {
		for _, kind := range []string{"basic", "digest"} {
			t.Run(kind+"/"+value, func(t *testing.T) {
				mid := &dynamic.Middleware{SecretValues: dynamic.SecretValues{ref: value}}
				if kind == "basic" {
					mid.BasicAuth = &dynamic.BasicAuth{Users: dynamic.Users{ref}}
				} else {
					mid.DigestAuth = &dynamic.DigestAuth{Users: dynamic.Users{ref}}
				}
				rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"auth": mid}}})
				builder := NewBuilder(rtConf.Middlewares, nil, nil)
				_, err := builder.BuildMiddlewareChain(t.Context(), []string{"auth"}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
				require.Error(t, err)
				t.Logf("returned err: %v", err)
				t.Logf("MiddlewareInfo.Err: %q", rtConf.Middlewares["auth"].Err)
				assert.Contains(t, err.Error(), "entry 1", "returned error must keep the failing entry position")
				require.Len(t, rtConf.Middlewares["auth"].Err, 1)
				assert.Contains(t, rtConf.Middlewares["auth"].Err[0], "entry 1", "MiddlewareInfo.Err must keep the failing entry position")
			})
		}
	}
}
