// Run (on branch evalon/traefik-se-c10e9fb2):
//   cp verify/repro/c6_unbound_urn_inline_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run '^TestVerify_C6_' ./pkg/server/middleware

//go:build verify

package middleware_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/middlewares/auth"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
)

// Inline (file-provider style) user entries that start with the URN prefix but have no Secrets binding.
func TestVerify_C6_UnboundURNPrefixInlineEntries(t *testing.T) {
	const sentinel = "leaked_inline_credential_hash_q9"
	for _, c := range []struct {
		desc string
		mid  *dynamic.Middleware
	}{
		{"basicAuth", &dynamic.Middleware{BasicAuth: &dynamic.BasicAuth{Users: dynamic.Users{"alice:hashalpha", "urn:k8s:secret:authsecret:users:" + sentinel}}}},
		{"digestAuth", &dynamic.Middleware{DigestAuth: &dynamic.DigestAuth{Users: dynamic.Users{"urn:k8s:secret:authsecret:users:" + sentinel}}}},
	} {
		require.Nil(t, c.mid.Secrets)
		rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"auth": c.mid}}})
		builder := middleware.NewBuilder(rtConf.Middlewares, nil, nil)
		_, err := builder.BuildMiddlewareChain(context.Background(), []string{"auth"}).Then(http.NotFoundHandler())
		require.Error(t, err)
		var userErr *auth.UserError
		if errors.As(err, &userErr) {
			fmt.Printf("[C6 %s] UserError.Index=%d Source=%q\n", c.desc, userErr.Index, userErr.Source)
		}
		fmt.Printf("[C6 %s] returned err: %s\n", c.desc, err)
		fmt.Printf("[C6 %s] stored info.Err: %q\n", c.desc, rtConf.Middlewares["auth"].Err)
		all := err.Error() + " " + strings.Join(rtConf.Middlewares["auth"].Err, " ")
		fmt.Printf("[C6 %s] diagnostics contain sentinel: %v\n", c.desc, strings.Contains(all, sentinel))
	}
}
