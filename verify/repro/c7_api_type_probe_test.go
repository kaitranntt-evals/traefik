//go:build verify

// Run (on evalon/traefik-se-f176eba1): cp verify/repro/c7_crd_export_verify.go pkg/provider/kubernetes/crd/ && cp verify/repro/c7_api_type_probe_test.go pkg/api/ && go test -tags verify -count=1 -v -run '^TestVerify_C7' ./pkg/api
package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/config/static"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
)

// Part "Type selection": extractType on a middleware holding its real configuration and a populated carrier.
func TestVerify_C7_ExtractTypeSelection(t *testing.T) {
	secrets := dynamic.NewSecretValues()
	secrets.Set("urn:k8s:secret:name:key", "s3cr3t")

	without := &dynamic.Middleware{RateLimit: &dynamic.RateLimit{Average: 1}}
	with := &dynamic.Middleware{RateLimit: &dynamic.RateLimit{Average: 1}}
	with.SetSecretValues(secrets)
	plugin := &dynamic.Middleware{Plugin: map[string]dynamic.PluginConf{"demo": {"secret": "urn:k8s:secret:name:key"}}}
	plugin.SetSecretValues(secrets)

	t.Logf("extractType(rateLimit, no carrier)        = %q", extractType(without))
	t.Logf("extractType(rateLimit, populated carrier) = %q", extractType(with))
	t.Logf("extractType(plugin demo, populated carrier) = %q", extractType(plugin))
	assert.Equal(t, "RateLimit", extractType(with))
	assert.Equal(t, "demo", extractType(plugin))
}

// Part "API-path reachability": CRD loader -> watcher-style DeepCopy -> aggregator-style qualified names ->
// runtime.NewConfig -> the real API router -> GET /api/http/middlewares.
func TestVerify_C7_APIResponseType(t *testing.T) {
	dir := "../provider/kubernetes/crd/fixtures/"
	merged := dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{}}}
	for _, file := range []string{"with_ratelimit.yml", "with_auth.yml", "with_plugin_read_secret.yml"} {
		conf, err := crd.VerifyLoadFixtures(t.Context(), dir+file)
		require.NoError(t, err)
		for name, mw := range conf.DeepCopy().HTTP.Middlewares {
			merged.HTTP.Middlewares[name+"@kubernetescrd"] = mw
		}
	}

	handler := New(static.Configuration{API: &static.API{}, Global: &static.Global{}}, runtime.NewConfig(merged))
	server := httptest.NewServer(handler.createRouter())
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/api/http/middlewares")
	require.NoError(t, err)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var got []map[string]any
	require.NoError(t, json.Unmarshal(body, &got))
	types := map[string]any{}
	for _, m := range got {
		t.Logf("GET /api/http/middlewares: name=%v type=%q", m["name"], m["type"])
		types[m["name"].(string)] = m["type"]
	}

	assert.Equal(t, "ratelimit", types["default-ratelimit@kubernetescrd"])
	assert.Equal(t, "forwardauth", types["default-forwardauth@kubernetescrd"])
	assert.Equal(t, "test-secret", types["default-test-secret@kubernetescrd"])
	assert.Equal(t, "basicauth", types["default-basicauth@kubernetescrd"])
	assert.Equal(t, "digestauth", types["default-digestauth@kubernetescrd"])
}
