//go:build verify

// Run: cp verify/repro/verify_c1_test.go pkg/server/middleware/ && go test -tags verify -count=1 -run TestVerifyC1 -v ./pkg/server/middleware
package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

// mutatingBuilder fails on the first (resolved) build, then mutates the config it receives on the retry.
type mutatingBuilder struct {
	calls   int
	configs []map[string]any
}

func (m *mutatingBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	m.calls++
	m.configs = append(m.configs, config)
	if m.calls == 1 {
		return nil, fmt.Errorf("decoding failed on %q", config["token"])
	}
	config["token"] = "MUTATED-BY-RETRY"
	if nested, ok := config["nested"].(map[string]any); ok {
		nested["token"] = "NESTED-MUTATED-BY-RETRY"
	}
	return nil, errors.New("rejected again")
}

func TestVerifyC1_PluginRetryAliasing(t *testing.T) {
	middleware := &dynamic.Middleware{
		Plugin: map[string]dynamic.PluginConf{
			"test": {
				"token":  "urn:k8s:secret:plugin:token",
				"nested": map[string]any{"token": "urn:k8s:secret:plugin:nested"},
			},
		},
		Secrets: dynamic.Secrets{
			"urn:k8s:secret:plugin:token":  "s3cr3t",
			"urn:k8s:secret:plugin:nested": "n3st3d",
		},
	}
	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": middleware}},
	})
	mb := &mutatingBuilder{}
	builder := NewBuilder(rtConf.Middlewares, nil, mb)

	_, err := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
	t.Logf("returned error: %v", err)
	t.Logf("stored info.Err: %q", rtConf.Middlewares["plugin"].Err)
	t.Logf("builder calls: %d", mb.calls)

	published := map[string]any(rtConf.Middlewares["plugin"].Plugin["test"])
	publishedNested := published["nested"].(map[string]any)
	for i, c := range mb.configs {
		t.Logf("call %d: config map identical to published map: %v; token=%v", i+1,
			reflect.ValueOf(c).Pointer() == reflect.ValueOf(published).Pointer(), c["token"])
		if n, ok := c["nested"].(map[string]any); ok {
			t.Logf("call %d: nested map identical to published nested map: %v", i+1,
				reflect.ValueOf(n).Pointer() == reflect.ValueOf(publishedNested).Pointer())
		}
	}
	t.Logf("published token after build: %v", published["token"])
	t.Logf("published nested token after build: %v", publishedNested["token"])

	if mb.calls < 2 {
		t.Fatalf("retry not reached: builder called %d time(s)", mb.calls)
	}
	if published["token"] != "MUTATED-BY-RETRY" {
		t.Fatalf("published configuration NOT mutated by the retry: token=%v", published["token"])
	}
	t.Log("CONFIRMED: retry builder mutation is visible in the published (API) configuration")
}
