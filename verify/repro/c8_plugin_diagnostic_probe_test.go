//go:build verify

// Run (on evalon/traefik-se-4000ae16): cp verify/repro/c8_crd_export_verify.go pkg/provider/kubernetes/crd/ && cp verify/repro/c8_plugin_diagnostic_probe_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run '^TestVerify_C8' ./pkg/server/middleware
package middleware

import (
	"errors"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
)

// Part "Value-dependent replacement": the same fixed error under a matching and a non-matching bound Secret value.
func TestVerify_C8_RedactFixedDiagnostic(t *testing.T) {
	fixed := errors.New("invalid 'token'")
	matching := dynamic.SecretValues{"urn:k8s:secret:name:key": "token"}.Redact(fixed).Error()
	other := dynamic.SecretValues{"urn:k8s:secret:name:key": "other"}.Redact(fixed).Error()
	t.Logf("Redact(%q) with Secret value %q -> %q", fixed, "token", matching)
	t.Logf("Redact(%q) with Secret value %q -> %q", fixed, "other", other)
	assert.Equal(t, other, matching)
}

const verifyC8Plugin = `package verifyc8

import (
	"context"
	"errors"
	"net/http"
)

type Config struct {
	User   string
	Secret string
}

func CreateConfig() *Config { return &Config{} }

// New ignores its configuration and always fails with the same fixed text.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return nil, errors.New("invalid 'token'")
}
`

// Part "Plugin diagnostic reachability": CRD loader -> runtime configuration -> production middleware.Builder with a
// real Yaegi local plugin whose constructor always returns the fixed error; only the Kubernetes Secret value differs.
func TestVerify_C8_PluginDiagnosticPath(t *testing.T) {
	dir := "plugins-local/src/verifyc8"
	require.NoError(t, os.MkdirAll(dir, 0o755))
	t.Cleanup(func() { _ = os.RemoveAll("plugins-local") })
	require.NoError(t, os.WriteFile(dir+"/verifyc8.go", []byte(verifyC8Plugin), 0o644))
	require.NoError(t, os.WriteFile(dir+"/.traefik.yml", []byte("displayName: verify c8\ntype: middleware\nruntime: yaegi\nimport: verifyc8\nbasePkg: verifyc8\nsummary: always fails with a fixed text\ntestData:\n  user: x\n"), 0o644))

	pluginBuilder, err := plugins.NewBuilder(nil, nil, map[string]plugins.LocalDescriptor{"test-secret": {ModuleName: "verifyc8"}})
	require.NoError(t, err)

	run := func(secretValue string) (string, []string) {
		conf, err := crd.VerifyLoadFixtureWithSecretValue(t.Context(), "../../provider/kubernetes/crd/fixtures/with_plugin_read_secret.yml", "key", secretValue)
		require.NoError(t, err)
		require.Equal(t, dynamic.PluginConf{"user": "admin", "secret": "urn:k8s:secret:name:key"}, conf.HTTP.Middlewares["default-test-secret"].Plugin["test-secret"])

		rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{
			"default-test-secret@kubernetescrd": conf.HTTP.Middlewares["default-test-secret"],
		}}})
		builder := NewBuilder(rtConf.Middlewares, nil, pluginBuilder)
		_, err = builder.BuildMiddlewareChain(t.Context(), []string{"default-test-secret@kubernetescrd"}).Then(http.NotFoundHandler())
		require.Error(t, err)
		return err.Error(), rtConf.Middlewares["default-test-secret@kubernetescrd"].Err
	}

	matchingErr, matchingAPI := run("token")
	otherErr, otherAPI := run("other")
	t.Logf("Secret value %q: returned error=%q, MiddlewareInfo.Err (exposed by the API)=%q", "token", matchingErr, matchingAPI)
	t.Logf("Secret value %q: returned error=%q, MiddlewareInfo.Err (exposed by the API)=%q", "other", otherErr, otherAPI)
	assert.Equal(t, otherErr, matchingErr)
	assert.Equal(t, otherAPI, matchingAPI)
}
