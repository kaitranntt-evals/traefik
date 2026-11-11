// Audit instrumentation (not production code). Copy into pkg/provider/kubernetes/crd/ of branch evalon/traefik-se-04dfad87
// together with the byte-exact eval fixtures, then run:
//   go test -v -race -count=1 -run '^TestVerify_C1' ./pkg/provider/kubernetes/crd
// The probes use the real plugins.Builder (Yaegi runtime) with local plugins written to a temporary plugins-local
// directory, and deliver the CRD provider configuration through a real server.ConfigurationWatcher.
package crd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)


// The generic plugin snapshots the configuration it receives, then overwrites every string it can reach.
const verifyGenericPlugin = `package verifygeneric

import (
	"context"
	"encoding/json"
	"net/http"
)

type Config map[string]interface{}

func CreateConfig() *Config {
	c := Config{}
	return &c
}

func scribble(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k := range t {
			t[k] = scribble(t[k])
		}
		t["injected-by-plugin"] = "MUTATED"
		return t
	case []interface{}:
		for i := range t {
			t[i] = scribble(t[i])
		}
		return t
	case string:
		return "MUTATED"
	}
	return v
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	received, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	scribble(map[string]interface{}(*config))

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(received)
	}), nil
}
`

const verifyTypedPlugin = `package verifytyped

import (
	"context"
	"encoding/json"
	"net/http"
)

type Inner struct {
	Token   string
	Port    int
	Enabled bool
}

type Config struct {
	User   string
	Secret int
	Ratio  float64
	Flag   bool
	Inner  Inner
	Hosts  []string
}

func CreateConfig() *Config {
	return &Config{}
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	received, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	config.User = "MUTATED"
	config.Inner.Token = "MUTATED"

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.Write(received)
	}), nil
}
`

// verifyPluginBuilder creates the production plugin builder over two local Yaegi plugins.
func verifyPluginBuilder(t *testing.T, aliases map[string]string) *plugins.Builder {
	t.Helper()

	dir := t.TempDir()
	for module, source := range map[string]string{"verifygeneric": verifyGenericPlugin, "verifytyped": verifyTypedPlugin} {
		p := filepath.Join(dir, "plugins-local", "src", module)
		require.NoError(t, os.MkdirAll(p, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(p, ".traefik.yml"), []byte("displayName: verify\ntype: middleware\nimport: "+module+"\nsummary: audit probe plugin\ntestData: {}\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(p, "go.mod"), []byte("module "+module+"\n\ngo 1.21\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(p, module+".go"), []byte(source), 0o644))
	}

	// Local plugins are read from ./plugins-local relative to the working directory.
	wd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	defer func() { require.NoError(t, os.Chdir(wd)) }()

	local := map[string]plugins.LocalDescriptor{}
	for alias, module := range aliases {
		local[alias] = plugins.LocalDescriptor{ModuleName: module}
	}

	builder, err := plugins.NewBuilder(nil, nil, local)
	require.NoError(t, err)

	return builder
}

type verifyC1Provider struct{ conf *dynamic.Configuration }

func (p *verifyC1Provider) Init() error { return nil }

func (p *verifyC1Provider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	go func() { ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: p.conf} }()
	return nil
}

func verifyC1Deliver(t *testing.T, conf *dynamic.Configuration) *runtime.Configuration {
	t.Helper()

	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)

	got := make(chan dynamic.Configuration, 1)
	watcher := server.NewConfigurationWatcher(pool, &verifyC1Provider{conf: conf}, nil, "", false)
	watcher.AddListener(func(c dynamic.Configuration) { got <- c })
	watcher.Start()

	select {
	case c := <-got:
		return runtime.NewConfig(c)
	case <-time.After(10 * time.Second):
		t.Fatal("configuration was not delivered")
		return nil
	}
}

func verifyC1Serve(t *testing.T, builder *middleware.Builder, name string) (string, error) {
	t.Helper()

	h, err := builder.BuildMiddlewareChain(t.Context(), []string{name}).Then(http.NotFoundHandler())
	if err != nil {
		return "", err
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	return rec.Body.String(), nil
}

func verifyC1JSON(t *testing.T, v any) string {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestVerify_C1_PluginExecutionAndIsolation(t *testing.T) {
	keys := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "evalkeys", Namespace: "default"},
		Data: map[string][]byte{
			"slash": []byte("qx_slash_value"), "tilde": []byte("qx_tilde_value"), "escaped": []byte("qx_escaped_value"), "listed": []byte("qx_listed_value"),
			"int": []byte("42"), "float": []byte("0.25"), "bool": []byte("true"), "token": []byte("qx_token_value"), "host": []byte("qx.example.com"),
		},
	}

	escapedInput := map[string]any{
		"a/b": "urn:k8s:secret:evalkeys:slash",
		"a":   map[string]any{"b": "literal-a-b"},
		"m~n": "urn:k8s:secret:evalkeys:tilde",
		"~1":  "urn:k8s:secret:evalkeys:escaped",
		"/":   "literal-slash",
		"x/y": []any{"literal-0", map[string]any{"~0": "urn:k8s:secret:evalkeys:listed"}},
	}
	typedInput := map[string]any{
		"user":   "admin",
		"secret": "urn:k8s:secret:evalkeys:int",
		"ratio":  "urn:k8s:secret:evalkeys:float",
		"flag":   "urn:k8s:secret:evalkeys:bool",
		"inner":  map[string]any{"token": "urn:k8s:secret:evalkeys:token", "port": "urn:k8s:secret:evalkeys:int", "enabled": "urn:k8s:secret:evalkeys:bool"},
		"hosts":  []any{"literal.example.com", "urn:k8s:secret:evalkeys:host"},
	}

	type probe struct {
		desc, plugin, middleware string
		load                     func() *dynamic.Configuration
		want                     string
	}

	fromFixture := func(file string) func() *dynamic.Configuration {
		return func() *dynamic.Configuration {
			conf, _ := crd.LoadConfigurationAndProviderForTest(t, []string{"services.yml", file})
			return conf
		}
	}

	probes := []probe{
		{
			desc: "flat", plugin: "test-secret", middleware: "default-test-secret", load: fromFixture("with_plugin_read_secret.yml"),
			want: `{"secret":"this_is_the_secret","user":"admin"}`,
		},
		{
			desc: "nested deep map", plugin: "test-secret", middleware: "default-test-secret", load: fromFixture("with_plugin_deep_read_secret.yml"),
			want: `{"secret_0":{"secret_1":{"secret_2":{"secret":"this_is_the_very_deep_secret","user":"admin"}}}}`,
		},
		{
			desc: "nested list", plugin: "test-secret", middleware: "default-test-secret", load: fromFixture("with_plugin_read_array_of_secret.yml"),
			want: `{"secret":["secret_data1","secret_data2"]}`,
		},
		{
			desc: "nested list of maps", plugin: "test-secret", middleware: "default-test-secret", load: fromFixture("with_plugin_read_array_of_map_contain_secret.yml"),
			want: `{"users":[{"name":"admin","secret":"admin_password"},{"name":"user","secret":"user_password"}]}`,
		},
		{
			desc: "escaped keys", plugin: "escaped-keys", middleware: "default-escaped-keys",
			load: func() *dynamic.Configuration {
				conf, _ := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "escaped-keys", "escaped-keys", escapedInput, keys)
				return conf
			},
			want: `{"/":"literal-slash","a":{"b":"literal-a-b"},"a/b":"qx_slash_value","m~n":"qx_tilde_value","x/y":["literal-0",{"~0":"qx_listed_value"}],"~1":"qx_escaped_value"}`,
		},
		{
			desc: "typed", plugin: "typed", middleware: "default-typed",
			load: func() *dynamic.Configuration {
				conf, _ := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "typed", "typed", typedInput, keys)
				return conf
			},
			want: `{"User":"admin","Secret":42,"Ratio":0.25,"Flag":true,"Inner":{"Token":"qx_token_value","Port":42,"Enabled":true},"Hosts":["literal.example.com","qx.example.com"]}`,
		},
	}

	pluginBuilder := verifyPluginBuilder(t, map[string]string{"test-secret": "verifygeneric", "escaped-keys": "verifygeneric", "typed": "verifytyped"})

	for _, p := range probes {
		t.Run(p.desc, func(t *testing.T) {
			conf := p.load()
			require.NotNil(t, conf.HTTP.Middlewares[p.middleware])
			declared := verifyC1JSON(t, conf.HTTP.Middlewares[p.middleware].Plugin[p.plugin])
			publishedBefore := verifyC1JSON(t, conf)

			rtConf := verifyC1Deliver(t, conf)
			qualified := p.middleware + "@kubernetescrd"
			require.Contains(t, rtConf.Middlewares, qualified)
			runtimeBefore := verifyC1JSON(t, rtConf)
			secretsBefore := verifyC1JSON(t, map[string]string(rtConf.Middlewares[qualified].Secrets))

			builder := middleware.NewBuilder(rtConf.Middlewares, nil, pluginBuilder)

			// Execution: the plugin constructor runs with the resolved values.
			received, err := verifyC1Serve(t, builder, qualified)
			require.NoError(t, err)
			t.Logf("declared (provider) : %s", declared)
			t.Logf("received by plugin  : %s", received)
			assert.JSONEq(t, p.want, received)

			// Isolation: the plugin overwrote its input in New; the shared configuration and bindings must not move.
			assert.Equal(t, publishedBefore, verifyC1JSON(t, conf))
			assert.Equal(t, runtimeBefore, verifyC1JSON(t, rtConf))
			assert.Equal(t, declared, verifyC1JSON(t, rtConf.Middlewares[qualified].Plugin[p.plugin]))
			assert.Equal(t, secretsBefore, verifyC1JSON(t, map[string]string(rtConf.Middlewares[qualified].Secrets)))
			assert.NotContains(t, verifyC1JSON(t, rtConf), "MUTATED")

			// A second build after the mutation still receives the resolved values.
			again, err := verifyC1Serve(t, builder, qualified)
			require.NoError(t, err)
			assert.JSONEq(t, p.want, again)

			// Concurrent builds and API-style serialization of the shared runtime configuration.
			var wg sync.WaitGroup
			var mu sync.Mutex
			mismatches := 0
			for range 20 {
				wg.Add(2)
				go func() {
					defer wg.Done()
					got, err := verifyC1Serve(t, builder, qualified)
					if err != nil || !assert.JSONEq(t, p.want, got) {
						mu.Lock()
						mismatches++
						mu.Unlock()
					}
				}()
				go func() {
					defer wg.Done()
					_, _ = json.Marshal(rtConf)
				}()
			}
			wg.Wait()

			t.Logf("shared after builds : %s", verifyC1JSON(t, rtConf.Middlewares[qualified].Plugin[p.plugin]))
			t.Logf("shared config unchanged=%v, bindings unchanged=%v, contains MUTATED=%v, concurrent builds=20 mismatches=%d, MiddlewareInfo.Err=%v",
				runtimeBefore == verifyC1JSON(t, rtConf), secretsBefore == verifyC1JSON(t, map[string]string(rtConf.Middlewares[qualified].Secrets)),
				strings.Contains(verifyC1JSON(t, rtConf), "MUTATED"), mismatches, rtConf.Middlewares[qualified].Err)
			assert.Zero(t, mismatches)
		})
	}
}
