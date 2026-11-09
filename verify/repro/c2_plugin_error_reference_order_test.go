// Run: git worktree add --detach ../wt-c2 <evalon/traefik-se-5aec87b4> && cp verify/repro/c2_plugin_error_reference_order_test.go ../wt-c2/pkg/server/middleware/ && (cd ../wt-c2 && go test -tags verify -count=1 -v -run '^TestVerifyC2_' ./pkg/server/middleware)

//go:build verify

package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

const verifyC2Builds = 200

// verifyC2Config is what the CRD provider emits for a plugin Middleware whose four options are urn:k8s:secret references.
func verifyC2Config() *dynamic.Middleware {
	return &dynamic.Middleware{
		Plugin: map[string]dynamic.PluginConf{
			"test": {
				"apiKey":       "urn:k8s:secret:plugin-creds:apiKey",
				"clientSecret": "urn:k8s:secret:plugin-creds:clientSecret",
				"password":     "urn:k8s:secret:redis-creds:password",
				"token":        "urn:k8s:secret:plugin-creds:token",
			},
		},
		Secrets: map[string]string{
			"urn:k8s:secret:plugin-creds:apiKey":       "value-a",
			"urn:k8s:secret:plugin-creds:clientSecret": "value-b",
			"urn:k8s:secret:redis-creds:password":      "value-c",
			"urn:k8s:secret:plugin-creds:token":        "value-d",
		},
	}
}

func verifyC2Report(t *testing.T, label string, counts map[string]int) {
	t.Helper()

	messages := make([]string, 0, len(counts))
	for message := range counts {
		messages = append(messages, message)
	}
	sort.Strings(messages)

	t.Logf("%s: %d distinct diagnostics over %d identical builds", label, len(counts), verifyC2Builds)
	for i, message := range messages {
		if i == 4 {
			t.Logf("  ... %d more", len(messages)-i)
			break
		}
		t.Logf("  %3dx %s", counts[message], message)
	}
}

// Part "Reference formatting": references() reports the collection order, it does not sort.
func TestVerifyC2_ReferencesKeepCollectionOrder(t *testing.T) {
	resolver := newSecretResolver(map[string]string{"urn:k8s:secret:z:z": "1", "urn:k8s:secret:a:a": "2", "urn:k8s:secret:m:m": "3"})
	resolver.resolve("urn:k8s:secret:z:z")
	resolver.resolve("urn:k8s:secret:a:a")
	resolver.resolve("urn:k8s:secret:m:m")

	t.Logf("references() = %q", resolver.references())
	assert.Equal(t, "urn:k8s:secret:z:z, urn:k8s:secret:a:a, urn:k8s:secret:m:m", resolver.references())
}

// Part "Failing-build trigger": resolvePluginConfig collects the references in Go map iteration order.
func TestVerifyC2_ResolvePluginConfigCollectionOrder(t *testing.T) {
	counts := map[string]int{}
	for range verifyC2Builds {
		config := verifyC2Config()
		resolver := newSecretResolver(config.Secrets)
		resolver.resolvePluginConfig(config.Plugin["test"])
		counts[fmt.Sprint(resolver.used)]++
	}

	verifyC2Report(t, "resolvePluginConfig used", counts)
	assert.Len(t, counts, 1, "identical configuration must collect the references in one order")
}

func verifyC2Build(t *testing.T, pluginBuilder PluginsBuilder) (error, []string) {
	t.Helper()

	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": verifyC2Config()}},
	})
	builder := NewBuilder(rtConf.Middlewares, nil, pluginBuilder)

	_, err := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
	require.Error(t, err)

	return err, rtConf.Middlewares["plugin"].Err
}

// Headline: the plugin constructor (the function returned by Build) fails, identical configuration every time.
func TestVerifyC2_ConstructorErrorOrdering(t *testing.T) {
	pluginBuilder := &pluginBuilderMock{build: func(map[string]any) (plugins.Constructor, error) {
		return func(context.Context, http.Handler) (http.Handler, error) {
			return nil, errors.New("upstream rejected the credentials")
		}, nil
	}}

	counts := map[string]int{}
	for range verifyC2Builds {
		err, infoErr := verifyC2Build(t, pluginBuilder)
		require.Equal(t, []string{err.Error()}, infoErr)
		counts[err.Error()]++
	}

	verifyC2Report(t, "constructor error", counts)
	assert.Len(t, counts, 1, "identical configuration must produce one diagnostic")
}

// Same formatter on the sibling path: Build rejects the values but accepts the references.
func TestVerifyC2_BuildErrorOrdering(t *testing.T) {
	pluginBuilder := &pluginBuilderMock{build: func(config map[string]any) (plugins.Constructor, error) {
		if config["token"] == "urn:k8s:secret:plugin-creds:token" {
			return func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }, nil
		}
		return nil, errors.New("upstream rejected the credentials")
	}}

	counts := map[string]int{}
	for range verifyC2Builds {
		err, _ := verifyC2Build(t, pluginBuilder)
		counts[err.Error()]++
	}

	verifyC2Report(t, "build error", counts)
	assert.Len(t, counts, 1, "identical configuration must produce one diagnostic")
}

// Consequence: one runtime configuration, one failing middleware shared by several routers.
// MiddlewareInfo.AddError dedupes on the exact string, so reordered diagnostics pile up in what the API serves.
func TestVerifyC2_MiddlewareInfoErrAccumulates(t *testing.T) {
	const routers = 20

	pluginBuilder := &pluginBuilderMock{build: func(map[string]any) (plugins.Constructor, error) {
		return func(context.Context, http.Handler) (http.Handler, error) {
			return nil, errors.New("upstream rejected the credentials")
		}, nil
	}}

	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": verifyC2Config()}},
	})
	builder := NewBuilder(rtConf.Middlewares, nil, pluginBuilder)

	for range routers {
		_, err := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
		require.Error(t, err)
	}

	info := rtConf.Middlewares["plugin"]
	t.Logf("MiddlewareInfo.Err holds %d entries after %d routers built the same middleware", len(info.Err), routers)
	for _, message := range info.Err {
		t.Logf("  %s", message)
	}
	assert.Len(t, info.Err, 1, "one failing middleware must report one error")
}

// Control: a single reference, so no ordering to vary.
func TestVerifyC2_ControlSingleReference(t *testing.T) {
	pluginBuilder := &pluginBuilderMock{build: func(map[string]any) (plugins.Constructor, error) {
		return func(context.Context, http.Handler) (http.Handler, error) {
			return nil, errors.New("upstream rejected the credentials")
		}, nil
	}}

	counts := map[string]int{}
	for range verifyC2Builds {
		rtConf := runtime.NewConfig(dynamic.Configuration{
			HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": {
				Plugin:  map[string]dynamic.PluginConf{"test": {"token": "urn:k8s:secret:plugin-creds:token", "plain": "x"}},
				Secrets: map[string]string{"urn:k8s:secret:plugin-creds:token": "value-d"},
			}}},
		})
		_, err := NewBuilder(rtConf.Middlewares, nil, pluginBuilder).BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
		require.Error(t, err)
		counts[err.Error()]++
	}

	verifyC2Report(t, "single reference", counts)
	assert.Len(t, counts, 1)
}
