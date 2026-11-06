// Run (on branch evalon/traefik-se-63b324ef):
//   cp verify/repro/c4_plugin_error_payloads_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run '^TestVerify_C4_' ./pkg/server/middleware

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
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
)

// quotingBuilder mimics a plugin whose decoding error quotes the offending value with %q (as mapstructure/yaegi errors do).
type quotingBuilder struct{ wrapOpaque bool }

type opaqueErr struct {
	msg   string
	cause error
}

func (e *opaqueErr) Error() string { return e.msg }
func (e *opaqueErr) Unwrap() error { return e.cause }

func (b quotingBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	v, _ := config["token"].(string)
	if b.wrapOpaque {
		return nil, &opaqueErr{msg: "plugin build failed: internal error", cause: fmt.Errorf("underlying failure with secret: %s", v)}
	}
	return nil, fmt.Errorf("decoding failed on %q: invalid token syntax", v)
}

func errTexts(err error) []string {
	var out []string
	for e := err; e != nil; e = errors.Unwrap(e) {
		out = append(out, e.Error())
	}
	return out
}

func TestVerify_C4_PluginErrorPayloadClasses(t *testing.T) {
	ref := "urn:k8s:secret:plugin:token"
	cases := []struct{ desc, value string }{
		{"empty", ""},
		{"ordinary", "s3cr3t_ordinary_value"},
		{"quote-bearing", `s3cr3t"quoted`},
		{"backslash-bearing", `s3cr3t\back`},
		{"word-collision", "invalid"},
	}
	for _, c := range cases {
		for _, opaque := range []bool{false, true} {
			mid := &dynamic.Middleware{
				Plugin:  map[string]dynamic.PluginConf{"test": {"token": ref}},
				Secrets: dynamic.Secrets{ref: c.value},
			}
			rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": mid}}})
			builder := middleware.NewBuilder(rtConf.Middlewares, nil, &quotingBuilder{wrapOpaque: opaque})
			_, err := builder.BuildMiddlewareChain(context.Background(), []string{"plugin"}).Then(http.NotFoundHandler())
			require.Error(t, err)
			all := strings.Join(append(errTexts(err), rtConf.Middlewares["plugin"].Err...), " | ")
			fmt.Printf("[C4 %s opaque=%v] value=%q\n  returned err chain: %q\n  stored info.Err: %q\n  escaped payload %q in diagnostics: %v\n",
				c.desc, opaque, c.value, errTexts(err), rtConf.Middlewares["plugin"].Err, fmt.Sprintf("%q", c.value), strings.Contains(all, strings.Trim(fmt.Sprintf("%q", c.value), `"`)) && c.value != "")
		}
	}
}
