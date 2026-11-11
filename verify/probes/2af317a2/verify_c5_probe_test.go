// Audit instrumentation (not production code). Copy into pkg/provider/kubernetes/crd/ of branch evalon/traefik-se-2af317a2
// together with the byte-exact eval fixtures, then run:
//   go test -v -race -count=1 -run '^TestVerify_C5' ./pkg/provider/kubernetes/crd
// The probes use the real plugins.Builder (Yaegi runtime) with local plugins written to a temporary plugins-local
// directory, and deliver the CRD provider configuration through a real server.ConfigurationWatcher.
package crd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
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
	"fmt"
	"net/http"
)

type Config map[string]interface{}

func CreateConfig() *Config {
	c := Config{}
	return &c
}

func describe(path string, v interface{}, out map[string]string) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k := range t {
			describe(path+"."+k, t[k], out)
		}
	case []interface{}:
		for i := range t {
			describe(fmt.Sprintf("%s[%d]", path, i), t[i], out)
		}
	default:
		_, isString := v.(string)
		out[path] = fmt.Sprintf("%T (string assertion ok: %v)", v, isString)
	}
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
	values, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	// A plugin cannot import Traefik packages: a credential is usable only if it arrives as a plain Go value.
	types := map[string]string{}
	describe("", map[string]interface{}(*config), types)
	received, err := json.Marshal(map[string]interface{}{"values": json.RawMessage(values), "types": types})
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

// The mixed plugin is a struct-typed configuration with a few untyped containers, as plugins taking free-form
// options declare them. It reports how each credential can be used from inside the plugin.
const verifyMixedPlugin = `package verifymixed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Cred struct {
	Password string
}

type Config struct {
	Token   string
	Any     interface{}
	Labels  map[string]string
	Creds   []Cred
	Options map[string]interface{}
	Users   []map[string]interface{}
	List    []interface{}
	Deep    interface{}
	Rules   []interface{}
}

func use(v interface{}) string {
	s, ok := v.(string)
	return fmt.Sprintf("type=%T string-assertion=%v asserted=%q fmt.Sprint=%q", v, ok, s, fmt.Sprint(v))
}

func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	report := map[string]string{
		"1 token (string field)":                      use(config.Token),
		"2 any (interface{} field)":                   use(config.Any),
		"3 labels.k (map[string]string)":              use(config.Labels["k"]),
		"4 creds[0].password (struct in slice)":       use(config.Creds[0].Password),
		"5 options.password (map[string]interface{})": use(config.Options["password"]),
		"6 users[0].secret ([]map[string]interface{})": use(config.Users[0]["secret"]),
		"7 list[0] ([]interface{})":                   use(config.List[0]),
		"8 options.nested.password (map inside map[string]interface{})": use(config.Options["nested"].(map[string]interface{})["password"]),
		"9 deep.password (map inside interface{} field)":                use(config.Deep.(map[string]interface{})["password"]),
		"10 rules[0].password (map inside []interface{})":               use(config.Rules[0].(map[string]interface{})["password"]),
	}
	received, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write(received)
	}), nil
}

func CreateConfig() *Config {
	return &Config{}
}
`

// verifyPluginBuilder creates the production plugin builder over two local Yaegi plugins.
func verifyPluginBuilder(t *testing.T, aliases map[string]string) *plugins.Builder {
	t.Helper()

	dir := t.TempDir()
	for module, source := range map[string]string{"verifygeneric": verifyGenericPlugin, "verifytyped": verifyTypedPlugin, "verifymixed": verifyMixedPlugin} {
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

type verifyC5Provider struct{ conf *dynamic.Configuration }

func (p *verifyC5Provider) Init() error { return nil }

func (p *verifyC5Provider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	go func() { ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: p.conf} }()
	return nil
}

func verifyC5Deliver(t *testing.T, conf *dynamic.Configuration) *runtime.Configuration {
	t.Helper()

	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)

	got := make(chan dynamic.Configuration, 1)
	watcher := server.NewConfigurationWatcher(pool, &verifyC5Provider{conf: conf}, nil, "", false)
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

func verifyC5Serve(t *testing.T, builder *middleware.Builder, name string) (string, error) {
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

// verifyC5Split separates the values and the leaf Go types reported by the generic probe plugin.
func verifyC5Split(t *testing.T, received string) (string, string) {
	t.Helper()

	var report struct {
		Values json.RawMessage   `json:"values"`
		Types  map[string]string `json:"types"`
	}
	if err := json.Unmarshal([]byte(received), &report); err != nil || report.Values == nil {
		return received, "n/a (typed plugin struct)"
	}

	types, err := json.Marshal(report.Types)
	require.NoError(t, err)
	return string(report.Values), string(types)
}

func verifyC5JSON(t *testing.T, v any) string {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestVerify_C5_PluginExecutionAndIsolation(t *testing.T) {
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
			declared := verifyC5JSON(t, conf.HTTP.Middlewares[p.middleware].Plugin[p.plugin])
			publishedBefore := verifyC5JSON(t, conf)

			rtConf := verifyC5Deliver(t, conf)
			qualified := p.middleware + "@kubernetescrd"
			require.Contains(t, rtConf.Middlewares, qualified)
			runtimeBefore := verifyC5JSON(t, rtConf)
			secretsBefore := verifyC5JSON(t, map[string]string(rtConf.Middlewares[qualified].Secrets))

			builder := middleware.NewBuilder(rtConf.Middlewares, nil, pluginBuilder)

			// Execution: the plugin constructor runs with the resolved values.
			received, err := verifyC5Serve(t, builder, qualified)
			require.NoError(t, err)
			t.Logf("declared (provider) : %s", declared)
			values, types := verifyC5Split(t, received)
			t.Logf("received by plugin  : %s", values)
			t.Logf("Go types in plugin  : %s", types)
			assert.JSONEq(t, p.want, values)
			assert.NotContains(t, types, "SecretValue")

			// Isolation: the plugin overwrote its input in New; the shared configuration and bindings must not move.
			assert.Equal(t, publishedBefore, verifyC5JSON(t, conf))
			assert.Equal(t, runtimeBefore, verifyC5JSON(t, rtConf))
			assert.Equal(t, declared, verifyC5JSON(t, rtConf.Middlewares[qualified].Plugin[p.plugin]))
			assert.Equal(t, secretsBefore, verifyC5JSON(t, map[string]string(rtConf.Middlewares[qualified].Secrets)))
			assert.NotContains(t, verifyC5JSON(t, rtConf), "MUTATED")

			// A second build after the mutation still receives the resolved values.
			again, err := verifyC5Serve(t, builder, qualified)
			require.NoError(t, err)
			againValues, _ := verifyC5Split(t, again)
			assert.JSONEq(t, p.want, againValues)

			// Concurrent builds and API-style serialization of the shared runtime configuration.
			var wg sync.WaitGroup
			var mu sync.Mutex
			mismatches := 0
			for range 20 {
				wg.Add(2)
				go func() {
					defer wg.Done()
					got, err := verifyC5Serve(t, builder, qualified)
					gotValues, _ := verifyC5Split(t, got)
					if err != nil || !assert.JSONEq(t, p.want, gotValues) {
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

			t.Logf("shared after builds : %s", verifyC5JSON(t, rtConf.Middlewares[qualified].Plugin[p.plugin]))
			t.Logf("shared config unchanged=%v, bindings unchanged=%v, contains MUTATED=%v, concurrent builds=20 mismatches=%d, MiddlewareInfo.Err=%v",
				runtimeBefore == verifyC5JSON(t, rtConf), secretsBefore == verifyC5JSON(t, map[string]string(rtConf.Middlewares[qualified].Secrets)),
				strings.Contains(verifyC5JSON(t, rtConf), "MUTATED"), mismatches, rtConf.Middlewares[qualified].Err)
			assert.Zero(t, mismatches)
		})
	}
}

func TestVerify_C5_UntypedContainersKeepSecretValue(t *testing.T) {
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "evalkeys", Namespace: "default"}, Data: map[string][]byte{"token": []byte("qx_token_value")}}
	urn := "urn:k8s:secret:evalkeys:token"
	input := map[string]any{
		"token":   urn,
		"any":     urn,
		"labels":  map[string]any{"k": urn},
		"creds":   []any{map[string]any{"password": urn}},
		"options": map[string]any{"password": urn, "nested": map[string]any{"password": urn}},
		"deep":    map[string]any{"password": urn},
		"rules":   []any{map[string]any{"password": urn}},
		"users":   []any{map[string]any{"name": "admin", "secret": urn}},
		"list":    []any{urn},
	}

	conf, _ := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "mixed", "mixed", input, secret)
	rtConf := verifyC5Deliver(t, conf)
	builder := middleware.NewBuilder(rtConf.Middlewares, nil, verifyPluginBuilder(t, map[string]string{"mixed": "verifymixed"}))

	received, err := verifyC5Serve(t, builder, "default-mixed@kubernetescrd")
	require.NoError(t, err)

	var report map[string]string
	require.NoError(t, json.Unmarshal([]byte(received), &report))
	keys := make([]string, 0, len(report))
	for k := range report {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.Logf("%-64s %s", k, report[k])
		assert.Contains(t, report[k], `asserted="qx_token_value"`)
	}
}
