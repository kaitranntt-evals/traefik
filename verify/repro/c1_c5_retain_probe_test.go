//go:build verify

// Run (per claim branch worktree, after verify/repro/install.sh <worktree>): go test -tags verify -count=1 -v -run '^TestVerify_Retain' ./pkg/provider/kubernetes/crd

package crd_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	corev1 "k8s.io/api/core/v1"
)

const (
	vRetainType       = "vretain"
	vRetainMiddleware = "default-vretain"
)

var (
	vRetainOnce sync.Once
	vRetainB    *plugins.Builder
	vRetainErr  error
)

func vRetainBuilder(t *testing.T) *plugins.Builder {
	t.Helper()
	vRetainOnce.Do(func() {
		vRetainB, vRetainErr = plugins.NewBuilder(nil, nil, map[string]plugins.LocalDescriptor{
			vRetainType: {ModuleName: "verifyretain"},
		})
	})
	require.NoError(t, vRetainErr)
	return vRetainB
}

// vFire builds a "fire" instance of the retaining plugin: the plugin then writes
// to every configuration object it was handed since the previous fire. It
// returns how many leaves the plugin rewrote.
func vFire(t *testing.T) string {
	t.Helper()
	ctor, err := vRetainBuilder(t).Build(vRetainType, map[string]any{"fire": true}, "fire")
	require.NoError(t, err)
	h, err := ctor(t.Context(), http.NotFoundHandler())
	require.NoError(t, err)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
	return rec.Body.String()
}

func vRetainInput(extra map[string]any) map[string]any {
	input := map[string]any{
		"port": "urn:k8s:secret:vsec:port",
		"deep": map[string]any{"inner": map[string]any{
			"token": "urn:k8s:secret:vsec:deep",
			"label": "literal-label",
		}},
		"list": []any{"urn:k8s:secret:vsec:listed", map[string]any{"secret": "urn:k8s:secret:vsec:u1", "name": "admin"}},
		// Containers holding no reference at all: a partial copy would still share them.
		"plain": map[string]any{"nested": map[string]any{"k": "literal-k"}, "items": []any{"literal-item"}},
	}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

// vRetainRun publishes the middleware through the CRD provider, builds it twice
// through the production middleware.Builder + plugins.Builder, lets the plugin
// write to what it retained AFTER those calls returned, and reports whether the
// published / runtime configuration still equals its pre-build snapshot.
func vRetainRun(t *testing.T, input map[string]any, secret *corev1.Secret, wantErr bool) (unchanged bool, rewritten string, after string) {
	t.Helper()
	vFire(t) // forget whatever earlier cases retained

	conf, provider := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "vretain", vRetainType, input, secret)
	published := conf.HTTP.Middlewares[vRetainMiddleware]
	require.NotNil(t, published)
	require.Equal(t, vJSON(t, input), vJSON(t, published.Plugin[vRetainType]), "the provider must publish the references")
	before := published.DeepCopy()
	beforeJSON := vJSON(t, published)

	rtConf := evalQualifiedRuntime(conf, crd.ProviderName)
	builder := newEvalMiddlewareBuilder(t, rtConf.Middlewares, vRetainBuilder(t), provider)
	fromHandler := "0"
	for range 2 {
		chain := builder.BuildMiddlewareChain(t.Context(), []string{evalQualifiedName(vRetainMiddleware, crd.ProviderName)})
		h, err := chain.Then(http.NotFoundHandler())
		if wantErr {
			require.Error(t, err)
			t.Logf("    returned error: %q", err.Error())
			continue
		}
		require.NoError(t, err)
		// The New callback has returned; the handler now writes to the configuration it kept.
		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		req.Header.Set("X-Verify-Mutate", "1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		fromHandler = rec.Body.String()
	}
	rewritten = "handler=" + fromHandler + " fire=" + vFire(t)

	info := evalMiddlewareInfo(rtConf, vRetainMiddleware)
	require.NotNil(t, info)
	after = vJSON(t, published)
	unchanged = after == beforeJSON && reflect.DeepEqual(before, published) && vJSON(t, info.Plugin) == vJSON(t, before.Plugin)
	return unchanged, rewritten, after
}

func TestVerify_Retain_PostCallbackOwnership(t *testing.T) {
	// Control: the probe detects aliasing. Hand the published map itself to the
	// production plugin builder (decode fails on an unrelated literal field), then fire.
	t.Run("control: published map handed directly to the plugin builder", func(t *testing.T) {
		vFire(t)
		input := vRetainInput(map[string]any{"enabled": "not-a-bool"})
		conf, _ := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "vretain", vRetainType, input, vSecret(nil))
		published := conf.HTTP.Middlewares[vRetainMiddleware]
		beforeJSON := vJSON(t, published)
		_, err := vRetainBuilder(t).Build(vRetainType, published.Plugin[vRetainType], "direct")
		require.Error(t, err)
		rewritten := vFire(t)
		t.Logf("CONTROL direct decode error %q; plugin rewrote %s leaves after decode returned; published unchanged = %v (expected false)", err.Error(), rewritten, vJSON(t, published) == beforeJSON)
		require.NotEqual(t, beforeJSON, vJSON(t, published), "control: the probe must detect a retained alias")
	})

	for _, tc := range []struct {
		name    string
		input   map[string]any
		secret  *corev1.Secret
		wantErr bool
	}{
		{name: "success: handler writes to retained config after New returned", input: vRetainInput(nil), secret: vSecret(nil)},
		{name: "decode error on unrelated literal field (reference-only replay)", input: vRetainInput(map[string]any{"enabled": "not-a-bool"}), secret: vSecret(nil), wantErr: true},
		{name: "constructor error with literal port (reference-only replay)", input: vRetainInput(map[string]any{"port": 8443, "fail": true}), secret: vSecret(nil), wantErr: true},
		{name: "decode error on Secret-bound int field (reference-only replay)", input: vRetainInput(nil), secret: vSecret(map[string]string{"port": "not-a-number"}), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unchanged, rewritten, after := vRetainRun(t, tc.input, tc.secret, tc.wantErr)
			t.Logf("RESULT %s: original published+runtime configuration unchanged after post-callback writes = %v (plugin rewrote leaves: %s)", tc.name, unchanged, rewritten)
			if !unchanged {
				t.Logf("    published AFTER: %s", after)
			}
			assert.True(t, unchanged, "the original reference-bearing configuration must not be reachable from what the plugin retained")
		})
	}
}
