//go:build ignore

// Run: in a checkout of evalon/traefik-se-ecef3519: sed '/^\/\/go:build ignore$/d' verify/repro/c8_buildplugin_error_probe_test.go > pkg/server/middleware/c8_buildplugin_error_probe_test.go && go test -count=1 -v -run 'TestProbe_C8' ./pkg/server/middleware
package middleware

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

type c8PluginBuilder func(pName string, config map[string]any, middlewareName string) (plugins.Constructor, error)

func (f c8PluginBuilder) Build(pName string, config map[string]any, middlewareName string) (plugins.Constructor, error) {
	return f(pName, config, middlewareName)
}

func TestProbe_C8_BuildPluginErrorOmitsURNs(t *testing.T) {
	const urn = "urn:k8s:secret:plugin:token"
	// Constructor rejects the resolved value, accepts the reference-only input.
	pb := c8PluginBuilder(func(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
		if config["token"] == "RESOLVED_SECRET_VALUE" {
			return nil, fmt.Errorf("cannot decode token %q", config["token"])
		}
		return func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }, nil
	})
	mid := &dynamic.Middleware{
		Plugin:  map[string]dynamic.PluginConf{"test": {"token": urn}},
		Secrets: map[string]string{urn: "RESOLVED_SECRET_VALUE"},
	}
	rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"secret": mid}}})
	b := NewBuilder(rtConf.Middlewares, nil, pb)

	resolved, err := mid.ResolveSecrets()
	require.NoError(t, err)
	_, err = b.buildPlugin(mid, resolved, "secret")
	t.Logf("buildPlugin error: %v", err)

	chain := b.BuildMiddlewareChain(t.Context(), []string{"secret"})
	_, err = chain.Then(http.NotFoundHandler())
	t.Logf("BuildMiddlewareChain error: %v", err)
	t.Logf("MiddlewareInfo.Err: %q", rtConf.Middlewares["secret"].Err)
}
