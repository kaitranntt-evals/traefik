// Run: sh verify/repro/C5_secretvalue_nested_plugin_input.sh (it copies this file into pkg/provider/kubernetes/crd/ of evalon/traefik-se-2af317a2); PASS means the problem reproduced.
package crd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	corev1 "k8s.io/api/core/v1"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

// A Yaegi plugin with typed fields and free-form options, reporting how each credential can be used from inside the plugin.
const reproC5Plugin = `package reproc5

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Config struct {
	Token   string
	Options map[string]interface{}
	Rules   []interface{}
}

func CreateConfig() *Config {
	return &Config{}
}

func use(v interface{}) string {
	s, ok := v.(string)
	return fmt.Sprintf("type=%T string-assertion=%v asserted=%q fmt.Sprint=%q", v, ok, s, fmt.Sprint(v))
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	report := map[string]string{
		"1 token (string field)":                                         use(config.Token),
		"2 options.password (map[string]interface{} entry)":              use(config.Options["password"]),
		"3 options.nested.password (map inside map[string]interface{})": use(config.Options["nested"].(map[string]interface{})["password"]),
		"4 rules[0].password (map inside []interface{})":                 use(config.Rules[0].(map[string]interface{})["password"]),
	}
	received, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write(received)
	}), nil
}
`

type reproC5Provider struct{ conf *dynamic.Configuration }

func (p *reproC5Provider) Init() error { return nil }

func (p *reproC5Provider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	go func() { ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: p.conf} }()
	return nil
}

func TestRepro_C5_NestedSecretValueReachesPlugin(t *testing.T) {
	// The real plugin builder over a local Yaegi plugin (read from ./plugins-local relative to the working directory).
	dir := t.TempDir()
	src := filepath.Join(dir, "plugins-local", "src", "reproc5")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, ".traefik.yml"), []byte("displayName: repro\ntype: middleware\nimport: reproc5\nsummary: repro\ntestData: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "go.mod"), []byte("module reproc5\n\ngo 1.21\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "reproc5.go"), []byte(reproC5Plugin), 0o644))

	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	pluginBuilder, err := plugins.NewBuilder(nil, nil, map[string]plugins.LocalDescriptor{"repro": {ModuleName: "reproc5"}})
	require.NoError(t, os.Chdir(wd))
	require.NoError(t, err)

	// A CRD plugin Middleware whose values all reference the same Secret key.
	urn := "urn:k8s:secret:creds:token"
	raw, err := json.Marshal(map[string]any{
		"token":   urn,
		"options": map[string]any{"password": urn, "nested": map[string]any{"password": urn}},
		"rules":   []any{map[string]any{"password": urn}},
	})
	require.NoError(t, err)

	conf := crd.LoadReproConfiguration(t,
		[]k8sruntime.Object{&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"}, Data: map[string][]byte{"token": []byte("qx_token_value")}}},
		[]k8sruntime.Object{&traefikv1alpha1.Middleware{
			ObjectMeta: metav1.ObjectMeta{Name: "repro", Namespace: "default"},
			Spec:       traefikv1alpha1.MiddlewareSpec{Plugin: map[string]apiextensionv1.JSON{"repro": {Raw: raw}}},
		}})
	require.Contains(t, conf.HTTP.Middlewares, "default-repro")

	// Deliver through a real ConfigurationWatcher, then build as the router factory does.
	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)
	got := make(chan dynamic.Configuration, 1)
	watcher := server.NewConfigurationWatcher(pool, &reproC5Provider{conf: conf}, nil, "", false)
	watcher.AddListener(func(c dynamic.Configuration) { got <- c })
	watcher.Start()

	var delivered dynamic.Configuration
	select {
	case delivered = <-got:
	case <-time.After(10 * time.Second):
		t.Fatal("configuration was not delivered")
	}

	rtConf := runtime.NewConfig(delivered)
	h, err := middleware.NewBuilder(rtConf.Middlewares, nil, pluginBuilder).BuildMiddlewareChain(t.Context(), []string{"default-repro@kubernetescrd"}).Then(http.NotFoundHandler())
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))

	var report map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &report))
	keys := make([]string, 0, len(report))
	for k := range report {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-64s %s", k, report[k])
	}

	// Values decoded directly into a field or a first-level entry are usable.
	for _, k := range keys[:2] {
		assert.Contains(t, report[k], `type=string string-assertion=true asserted="qx_token_value"`)
	}
	// The problem: values nested inside an untyped container reach the plugin as Traefik's dynamic.SecretValue struct,
	// which a plugin cannot assert to a string, and which formats as the reference instead of the credential.
	for _, k := range keys[2:] {
		assert.True(t, strings.Contains(report[k], `type=dynamic.SecretValue string-assertion=false asserted="" fmt.Sprint="urn:k8s:secret:creds:token"`), report[k])
	}
}
