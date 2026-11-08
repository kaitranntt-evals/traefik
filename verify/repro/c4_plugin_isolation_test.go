// Run: cp verify/repro/c4_plugin_isolation_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC4 ./pkg/server/middleware/  (on branch evalon/traefik-se-374ebb45)

//go:build verify

package middleware

import (
	"context"
	"encoding/json"
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
	c4Ref    = "urn:k8s:secret:plugin:token"
	c4Secret = "hunter2-very-secret"
)

var c4Noop plugins.Constructor = func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }

// listReplacingBuilder replaces a nested list element of the configuration it is given.
type listReplacingBuilder struct{ calls int }

func (b *listReplacingBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	if l, ok := config["list"].([]any); ok {
		l[0] = "REPLACED-BY-PLUGIN"
	}
	return c4Noop, nil
}

// retryMutatingBuilder fails with a Secret-bearing error first, then mutates the input of the retry.
type retryMutatingBuilder struct{ calls int }

func (b *retryMutatingBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	b.calls++
	if b.calls == 1 {
		return nil, fmt.Errorf("invalid token %q", config["token"])
	}
	config["token"] = "MUTATED-BY-RETRY"
	if l, ok := config["list"].([]any); ok {
		l[0] = "REPLACED-BY-RETRY"
	}
	return nil, fmt.Errorf("invalid token %q", config["token"])
}

func c4Run(t *testing.T, withSecrets bool, pb PluginsBuilder) {
	t.Helper()
	mid := &dynamic.Middleware{Plugin: map[string]dynamic.PluginConf{"test": {"token": "literal", "list": []any{"a", "b"}}}}
	if withSecrets {
		mid.Plugin["test"]["token"] = c4Ref
		mid.SecretValues = dynamic.SecretValues{c4Ref: c4Secret}
	}
	rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": mid}}})
	published := rtConf.Middlewares["plugin"]
	before, err := json.Marshal(published.Plugin)
	require.NoError(t, err)

	builder := NewBuilder(rtConf.Middlewares, nil, pb)
	_, buildErr := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))

	after, err := json.Marshal(published.Plugin)
	require.NoError(t, err)
	t.Logf("build err=%v", buildErr)
	t.Logf("published before: %s", before)
	t.Logf("published after:  %s", after)
	assert.JSONEq(t, string(before), string(after), "published plugin configuration must not change")
}

func TestVerifyC4_ListElementIsolation_WithSecrets(t *testing.T) {
	c4Run(t, true, &listReplacingBuilder{})
}

func TestVerifyC4_ListElementIsolation_WithoutSecrets(t *testing.T) {
	c4Run(t, false, &listReplacingBuilder{})
}

func TestVerifyC4_ErrorRetryIsolation(t *testing.T) {
	pb := &retryMutatingBuilder{}
	c4Run(t, true, pb)
	t.Logf("Build calls=%d", pb.calls)
}
