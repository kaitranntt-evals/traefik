//go:build verify

// Run: cp verify/repro/verify_c8_test.go pkg/server/middleware/ && go test -tags verify -count=1 -run TestVerifyC8 -v ./pkg/server/middleware
package middleware

import (
	"net/http"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
)

func TestVerifyC8_NilPluginBuilderPanics(t *testing.T) {
	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{
			"plugin": {Plugin: map[string]dynamic.PluginConf{"test": {"token": "x"}}},
		}},
	})
	var nilBuilder PluginsBuilder // nil interface, as in NewBuilder(configs, nil, nil)
	builder := NewBuilder(rtConf.Middlewares, nil, nilBuilder)

	var panicked any
	var err error
	func() {
		defer func() { panicked = recover() }()
		_, err = builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
	}()
	t.Logf("panic: %v", panicked)
	t.Logf("error: %v", err)
	t.Logf("stored info.Err: %q", rtConf.Middlewares["plugin"].Err)
	if panicked == nil {
		t.Log("REFUTED: no panic, construction returned an error")
	} else {
		t.Log("CONFIRMED: construction panicked")
	}
}
