// Run: cp verify/repro/c3_cached_plugin_error_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC3 ./pkg/server/middleware/  (on branch evalon/traefik-se-d2f31b62)

//go:build verify

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

const (
	c3Ref    = "urn:k8s:secret:plugin:token"
	c3Secret = "hunter2-very-secret"
)

// cachingBuildPluginBuilder caches the first error returned by Build and returns it on every later call.
type cachingBuildPluginBuilder struct {
	cached error
	calls  int
}

func (c *cachingBuildPluginBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	c.calls++
	if c.cached == nil {
		c.cached = fmt.Errorf("failed to decode configuration: invalid token %q", config["token"])
	}
	return nil, c.cached
}

// cachingConstructorPluginBuilder builds fine, but the plugin constructor caches its first error.
type cachingConstructorPluginBuilder struct {
	cached error
	calls  int
}

func (c *cachingConstructorPluginBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	token := config["token"]
	return func(_ context.Context, _ http.Handler) (http.Handler, error) {
		c.calls++
		if c.cached == nil {
			c.cached = fmt.Errorf("plugin init failed: bad token %q", token)
		}
		return nil, c.cached
	}, nil
}

func c3Build(t *testing.T, pb PluginsBuilder) (error, []string) {
	t.Helper()
	rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{
		"plugin": {
			Plugin:       map[string]dynamic.PluginConf{"test": {"token": c3Ref}},
			SecretValues: dynamic.SecretValues{c3Ref: c3Secret},
		},
	}}})
	builder := NewBuilder(rtConf.Middlewares, nil, pb)
	_, err := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
	require.Error(t, err)
	return err, rtConf.Middlewares["plugin"].Err
}

func TestVerifyC3_CachedBuildError(t *testing.T) {
	pb := &cachingBuildPluginBuilder{}
	err, infoErr := c3Build(t, pb)
	t.Logf("Build calls=%d", pb.calls)
	t.Logf("returned err: %v", err)
	t.Logf("MiddlewareInfo.Err: %q", infoErr)
	assert.NotContains(t, err.Error(), c3Secret)
	for _, e := range infoErr {
		assert.NotContains(t, e, c3Secret)
	}
}

func TestVerifyC3_CachedConstructorError(t *testing.T) {
	pb := &cachingConstructorPluginBuilder{}
	err, infoErr := c3Build(t, pb)
	t.Logf("constructor calls=%d", pb.calls)
	t.Logf("returned err: %v", err)
	t.Logf("MiddlewareInfo.Err: %q", infoErr)
	assert.NotContains(t, err.Error(), c3Secret)
	for _, e := range infoErr {
		assert.NotContains(t, e, c3Secret)
	}
}
