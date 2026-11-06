// Run: cd <worktree of evalon/traefik-se-cff842f1> && cp <verify-branch>/verify/repro/c9_nil_middleware_logging_panic_test.go pkg/server/ && go test -count=1 -run TestVerifyC9 -v ./pkg/server
package server

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/traefik/paerser/file"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/safe"
)

func verifyC9Conf() *dynamic.Configuration {
	return &dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
		Routers:     map[string]*dynamic.Router{"r": {Service: "s", Rule: "Path(`/`)"}},
		Services:    map[string]*dynamic.Service{"s": {LoadBalancer: &dynamic.ServersLoadBalancer{}}},
		Middlewares: map[string]*dynamic.Middleware{"nil-mid": nil, "ok": {}},
	}}
}

// Part logging_mechanism.
func TestVerifyC9_LogConfigurationNilEntry(t *testing.T) {
	logger := zerolog.New(os.Stderr).Level(zerolog.DebugLevel)
	defer func() {
		r := recover()
		t.Logf("recovered: %v", r)
		require.NotNil(t, r)
	}()
	logConfiguration(logger, dynamic.Message{ProviderName: "x", Configuration: verifyC9Conf()})
	t.Log("no panic")
}

// Same call at info level does not reach the loop.
func TestVerifyC9_LogConfigurationNilEntry_InfoLevel(t *testing.T) {
	logger := zerolog.New(os.Stderr).Level(zerolog.InfoLevel)
	logConfiguration(logger, dynamic.Message{ProviderName: "x", Configuration: verifyC9Conf()})
	t.Log("no panic at info level")
}

// Part production_trigger (1): the file/http providers decode YAML "null" into a nil map entry.
func TestVerifyC9_DecodeNullMiddleware(t *testing.T) {
	conf := &dynamic.Configuration{}
	err := file.DecodeContent("http:\n  routers:\n    r:\n      rule: Path(`/`)\n      service: s\n  services:\n    s:\n      loadBalancer:\n        servers:\n          - url: http://127.0.0.1\n  middlewares:\n    nil-mid:\n    ok:\n      stripPrefix:\n        prefixes: [/a]\n", ".yaml", conf)
	require.NoError(t, err)
	mid, ok := conf.HTTP.Middlewares["nil-mid"]
	t.Logf("yaml: middlewares=%d, nil-mid present=%v, nil-mid==nil: %v", len(conf.HTTP.Middlewares), ok, mid == nil)

	conf = &dynamic.Configuration{}
	err = file.DecodeContent(`{"http":{"routers":{"r":{"rule":"Path(`+"`/`"+`)","service":"s"}},"services":{"s":{"loadBalancer":{"servers":[{"url":"http://127.0.0.1"}]}}},"middlewares":{"nil-mid":null,"ok":{"stripPrefix":{"prefixes":["/a"]}}}}}`, ".json", conf)
	require.NoError(t, err)
	mid, ok = conf.HTTP.Middlewares["nil-mid"]
	t.Logf("json: middlewares=%d, nil-mid present=%v, nil-mid==nil: %v", len(conf.HTTP.Middlewares), ok, mid == nil)

	// pkg/plugins/providers.go decodes provider-plugin output with encoding/json, which keeps null entries.
	conf = &dynamic.Configuration{}
	require.NoError(t, json.Unmarshal([]byte(`{"http":{"middlewares":{"nil-mid":null}}}`), conf))
	mid, ok = conf.HTTP.Middlewares["nil-mid"]
	t.Logf("encoding/json (plugin provider path): middlewares=%d, nil-mid present=%v, nil-mid==nil: %v", len(conf.HTTP.Middlewares), ok, mid == nil)
}

// Part production_trigger (2): receiveConfigurations logs before anything validates the entries.
func TestVerifyC9_ReceiveConfigurationsNilEntry(t *testing.T) {
	prev := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(prev) })

	panicked := make(chan any, 1)
	routinesPool := safe.NewPool(t.Context())
	pvd := &mockProvider{messages: []dynamic.Message{{ProviderName: "mock", Configuration: verifyC9Conf()}}}
	watcher := NewConfigurationWatcher(routinesPool, pvd, []string{}, "", false)
	published := make(chan struct{}, 1)
	watcher.AddListener(func(_ dynamic.Configuration) { published <- struct{}{} })

	// receiveConfigurations runs in the pool; a panic there is caught by safe.Pool's recover and logged.
	// Run it directly to observe the panic.
	go func() {
		defer func() { panicked <- recover() }()
		watcher.allProvidersConfigs <- dynamic.Message{ProviderName: "mock", Configuration: verifyC9Conf()}
		close(watcher.allProvidersConfigs)
	}()
	ctx := zerolog.New(os.Stderr).Level(zerolog.DebugLevel).WithContext(t.Context())
	go func() {
		defer func() { panicked <- recover() }()
		watcher.receiveConfigurations(ctx)
	}()
	select {
	case r := <-panicked:
		t.Logf("receiveConfigurations recovered: %v", r)
		if r == nil {
			r = <-panicked
			t.Logf("second recovered: %v", r)
		}
	case <-time.After(3 * time.Second):
		t.Log("no panic within 3s")
	}
	_ = published
}
