// Run: cp verify/repro/c6_plugin_equal_value_binding_test.go pkg/server/middleware/verify_probe_c6_test.go && go test -tags verify -count=1 -run '^TestVerifyC6_' -v ./pkg/server/middleware (on branch evalon/traefik-se-aad38ae0)
//go:build verify

package middleware

import (
	"context"
	"net/http"
	"reflect"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

type mutatingPluginBuilder struct {
	received map[string]any
}

func (b *mutatingPluginBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	b.received = config
	config["token"] = "MUTATED-BY-PLUGIN"
	if nested, ok := config["nested"].(map[string]any); ok {
		nested["token"] = "MUTATED-NESTED"
	}
	return func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }, nil
}

func TestVerifyC6_PluginEqualValueBindingSharesPublishedConfig(t *testing.T) {
	const ref = "urn:k8s:secret:plugin:token"

	for _, test := range []struct {
		name  string
		value string
	}{
		{name: "secret_value_differs_from_reference", value: "real-token-value"},
		{name: "secret_value_equals_reference", value: ref},
	} {
		t.Run(test.name, func(t *testing.T) {
			published := dynamic.PluginConf{"token": ref, "nested": map[string]any{"token": ref}}
			configs := map[string]*runtime.MiddlewareInfo{
				"plugin": {
					Middleware: &dynamic.Middleware{
						Plugin:  map[string]dynamic.PluginConf{"test": published},
						Secrets: map[string]string{ref: test.value},
					},
				},
			}
			pb := &mutatingPluginBuilder{}
			_, err := NewBuilder(configs, nil, pb).buildConstructor(context.Background(), "plugin")
			if err != nil {
				t.Fatal(err)
			}

			publishedPtr := reflect.ValueOf(map[string]any(configs["plugin"].Middleware.Plugin["test"])).Pointer()
			receivedPtr := reflect.ValueOf(pb.received).Pointer()
			t.Logf("same top-level map passed to plugin builder: %v", publishedPtr == receivedPtr)
			t.Logf("published after build: %v", configs["plugin"].Middleware.Plugin["test"])

			if publishedPtr == receivedPtr {
				t.Errorf("plugin builder received the published configuration map itself (no deep copy)")
			}
			if got := configs["plugin"].Middleware.Plugin["test"]["token"]; got != ref {
				t.Errorf("published top-level token mutated: %v", got)
			}
			if got := configs["plugin"].Middleware.Plugin["test"]["nested"].(map[string]any)["token"]; got != ref {
				t.Errorf("published nested token mutated: %v", got)
			}
		})
	}
}
