//go:build ignore

// Run: in a checkout of evalon/traefik-se-94d51722: sed '/^\/\/go:build ignore$/d' verify/repro/c9_sanitization_position_probe_test.go > pkg/server/middleware/c9_sanitization_position_probe_test.go && go test -count=1 -v -run 'TestProbe_C9' ./pkg/server/middleware
package middleware

import (
	"net/http"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/middlewares/auth"
)

func TestProbe_C9_SanitizationDropsEntryPosition(t *testing.T) {
	const urn = "urn:k8s:secret:auth:users"
	for _, tc := range []struct {
		desc  string
		value string
		mid   func() *dynamic.Middleware
		ctor  func(users []string) error
	}{
		{"basic/format", "format", func() *dynamic.Middleware { return &dynamic.Middleware{BasicAuth: &dynamic.BasicAuth{Users: dynamic.Users{urn}}} }, func(u []string) error {
			_, err := auth.NewBasic(t.Context(), http.NotFoundHandler(), dynamic.BasicAuth{Users: u}, "m"); return err
		}},
		{"basic/invalid", "invalid", func() *dynamic.Middleware { return &dynamic.Middleware{BasicAuth: &dynamic.BasicAuth{Users: dynamic.Users{urn}}} }, func(u []string) error {
			_, err := auth.NewBasic(t.Context(), http.NotFoundHandler(), dynamic.BasicAuth{Users: u}, "m"); return err
		}},
		{"basic/unrelated-word", "zzqq_not_in_message", func() *dynamic.Middleware { return &dynamic.Middleware{BasicAuth: &dynamic.BasicAuth{Users: dynamic.Users{urn}}} }, func(u []string) error {
			_, err := auth.NewBasic(t.Context(), http.NotFoundHandler(), dynamic.BasicAuth{Users: u}, "m"); return err
		}},
		{"digest/format", "format", func() *dynamic.Middleware { return &dynamic.Middleware{DigestAuth: &dynamic.DigestAuth{Users: dynamic.Users{urn}}} }, func(u []string) error {
			_, err := auth.NewDigest(t.Context(), http.NotFoundHandler(), dynamic.DigestAuth{Users: u}, "m"); return err
		}},
		{"digest/invalid", "invalid", func() *dynamic.Middleware { return &dynamic.Middleware{DigestAuth: &dynamic.DigestAuth{Users: dynamic.Users{urn}}} }, func(u []string) error {
			_, err := auth.NewDigest(t.Context(), http.NotFoundHandler(), dynamic.DigestAuth{Users: u}, "m"); return err
		}},
	} {
		// Parser diagnostic: the resolved (malformed) entry handed straight to the constructor.
		parserErr := tc.ctor([]string{tc.value})

		mid := tc.mid()
		mid.Secrets = dynamic.Secrets{urn: tc.value}
		rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"m": mid}}})
		b := NewBuilder(rtConf.Middlewares, nil, nil)
		chain := b.BuildMiddlewareChain(t.Context(), []string{"m"})
		_, finalErr := chain.Then(http.NotFoundHandler())
		t.Logf("[%s] secret value=%q", tc.desc, tc.value)
		t.Logf("[%s]   parser diagnostic : %v", tc.desc, parserErr)
		t.Logf("[%s]   returned final    : %v", tc.desc, finalErr)
		t.Logf("[%s]   stored Info.Err   : %q", tc.desc, rtConf.Middlewares["m"].Err)
	}
}
