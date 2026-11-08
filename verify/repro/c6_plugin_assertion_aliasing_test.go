// Run: cp verify/repro/c6_plugin_assertion_aliasing_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC6 ./pkg/server/middleware/  (on branch evalon/traefik-se-682bd5b0)

//go:build verify

package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

// Same shape as the authored TestBuilder_buildConstructor_pluginSecrets ("accepted value read from a Secret" case),
// with a controlled mutation of the published plugin map inserted right before the authored assertion.
func TestVerifyC6_AuthoredAssertionSurvivesSharedMapMutation(t *testing.T) {
	noopConstructor := func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }
	accept := pluginBuilderMock(func(_ string, _ map[string]any, _ string) (plugins.Constructor, error) { return noopConstructor, nil })

	middleware := &dynamic.Middleware{
		Plugin:  map[string]dynamic.PluginConf{"test": {"token": "urn:k8s:secret:plugin:token"}},
		Secrets: map[string]string{"urn:k8s:secret:plugin:token": "expected-token"},
	}

	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": middleware}},
	})
	builder := NewBuilder(rtConf.Middlewares, nil, accept)

	_, err := builder.buildConstructor(t.Context(), "plugin")
	require.NoError(t, err)

	// Controlled mutation of the published (shared) plugin map, as a leaking resolver would do.
	rtConf.Middlewares["plugin"].Plugin["test"]["token"] = "expected-token"

	t.Logf("same *dynamic.Middleware pointer: %v", middleware == rtConf.Middlewares["plugin"].Middleware)
	t.Logf("expected operand (test.middleware...): %q", middleware.Plugin["test"]["token"])
	t.Logf("actual operand (rtConf...):           %q", rtConf.Middlewares["plugin"].Plugin["test"]["token"])

	// The authored assertion, verbatim.
	assert.Equal(t, middleware.Plugin["test"]["token"], rtConf.Middlewares["plugin"].Plugin["test"]["token"])

	// An assertion against an independent expected value catches the mutation.
	assert.Equal(t, "urn:k8s:secret:plugin:token", rtConf.Middlewares["plugin"].Plugin["test"]["token"], "independent expectation")
}
