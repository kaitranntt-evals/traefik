// Run: cd <worktree of evalon/traefik-se-8ca00183> && cp <verify-branch>/verify/repro/c5_plugin_ctor_error_oracle_test.go pkg/server/middleware/ && go test -count=1 -run TestVerifyC5 -v ./pkg/server/middleware
package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

// ctorErrPluginBuilder decodes fine, but the plugin constructor (New) fails with a fixed message.
type ctorErrPluginBuilder struct{ err error }

func (b *ctorErrPluginBuilder) Build(_ string, _ map[string]any, _ string) (plugins.Constructor, error) {
	return func(context.Context, http.Handler) (http.Handler, error) { return nil, b.err }, nil
}

func verifyC5Build(t *testing.T, secretValue string, ctorErr error) (string, string) {
	t.Helper()
	const ref = "urn:k8s:secret:plugsecret:token"
	mid := &dynamic.Middleware{Plugin: map[string]dynamic.PluginConf{"demo": {"token": ref, "mode": "strict"}}}
	mid.SetSecretValues(map[string]string{ref: secretValue})
	configs := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plug": mid}}}).Middlewares
	b := NewBuilder(configs, nil, &ctorErrPluginBuilder{err: ctorErr})
	_, err := b.BuildMiddlewareChain(t.Context(), []string{"plug"}).Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	require.Error(t, err)
	return err.Error(), fmt.Sprint(configs["plug"].Err)
}

func TestVerifyC5_ConstructorErrorDependsOnSecretText(t *testing.T) {
	ctorErr := errors.New("invalid token")
	cases := []struct{ desc, secret string }{
		{"secret equals a word of the message", "token"},
		{"unrelated secret", "s3cr3t-unrelated"},
		{"whitespace-only secret", "   "},
		{"secret with newline, quoted in message", "tok\nen"},
	}
	for _, c := range cases {
		e := ctorErr
		if c.secret == "tok\nen" {
			e = fmt.Errorf("plugin rejected %q", c.secret)
		}
		msg, apiErr := verifyC5Build(t, c.secret, e)
		t.Logf("%-45s ctorErr=%q -> exposed=%q api=%s", c.desc, e.Error(), msg, apiErr)
	}
	// Wrapped error whose cause text contains the value.
	msg, _ := verifyC5Build(t, "token", fmt.Errorf("outer: %w", ctorErr))
	t.Logf("%-45s -> exposed=%q", "wrapped, secret=token", msg)
}
