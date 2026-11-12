//go:build verify

// Run (per claim branch worktree, after verify/repro/install.sh <worktree>): go test -tags verify -count=1 -v -run '^TestVerify_C[12]_' ./pkg/provider/kubernetes/crd

package crd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	vPluginType = "vprobe"
	vMiddleware = "default-vprobe"
)

var (
	vBuilderOnce sync.Once
	vBuilder     *plugins.Builder
	vBuilderErr  error
)

// vPluginBuilder is the production plugins.Builder over the audit-owned local Yaegi plugin.
func vPluginBuilder(t *testing.T) *plugins.Builder {
	t.Helper()
	vBuilderOnce.Do(func() {
		vBuilder, vBuilderErr = plugins.NewBuilder(nil, nil, map[string]plugins.LocalDescriptor{
			vPluginType: {ModuleName: "verifyplugin"},
		})
	})
	require.NoError(t, vBuilderErr)
	return vBuilder
}

func vSecret(overrides map[string]string) *corev1.Secret {
	data := map[string][]byte{
		"port":    []byte("8443"),
		"token":   []byte("tok-VALUE"),
		"deep":    []byte("deep-VALUE"),
		"u1":      []byte("admin-VALUE"),
		"u2":      []byte("user-VALUE"),
		"slash":   []byte("slash-VALUE"),
		"tilde":   []byte("tilde-VALUE"),
		"escaped": []byte("escaped-VALUE"),
		"listed":  []byte("listed-VALUE"),
	}
	for k, v := range overrides {
		data[k] = []byte(v)
	}
	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "vsec", Namespace: "default"}, Data: data}
}

// vInput is the Middleware CRD plugin configuration: typed int, typed nested
// struct, typed list of structs, and untyped keys with special characters.
func vInput(extra map[string]any) map[string]any {
	input := map[string]any{
		"port":   "urn:k8s:secret:vsec:port",
		"token":  "urn:k8s:secret:vsec:token",
		"public": "PUBLIC-LITERAL",
		"deep": map[string]any{"inner": map[string]any{
			"token": "urn:k8s:secret:vsec:deep",
			"label": "literal-label",
		}},
		"users": []any{
			map[string]any{"name": "admin", "secret": "urn:k8s:secret:vsec:u1"},
			map[string]any{"name": "user", "secret": "urn:k8s:secret:vsec:u2"},
		},
		"a/b":   "urn:k8s:secret:vsec:slash",
		"a":     map[string]any{"b": "literal-a-b"},
		"m~n":   "urn:k8s:secret:vsec:tilde",
		"~1":    "urn:k8s:secret:vsec:escaped",
		"/":     "literal-slash",
		"x.y-z": []any{"literal-0", map[string]any{"~0": "urn:k8s:secret:vsec:listed", "keep": "literal-keep"}},
	}
	for k, v := range extra {
		input[k] = v
	}
	return input
}

type vObservation struct {
	handler  http.Handler
	errs     []error  // one per attempt on the same middleware.Builder
	returned []string // every text reachable through Unwrap, all attempts
	stored   []string // runtime MiddlewareInfo.Err after the attempts (what the API serves)
	// isolation
	publishedUnchanged bool
	runtimeUnchanged   bool
	deepEqualUnchanged bool
	publishedBefore    string
	publishedAfter     string
}

func vJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	return string(raw)
}

// vBuildTwice loads the CRD + Secret through the CRD provider, then builds the
// middleware twice through the production middleware.Builder and plugins.Builder.
func vBuildTwice(t *testing.T, input map[string]any, secret *corev1.Secret, serve bool) vObservation {
	t.Helper()

	conf, provider := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "vprobe", vPluginType, input, secret)
	require.NotNil(t, conf)
	require.NotNil(t, conf.HTTP)
	published := conf.HTTP.Middlewares[vMiddleware]
	require.NotNil(t, published)
	require.Equal(t, vJSON(t, input), vJSON(t, published.Plugin[vPluginType]), "the provider must publish the references, not the values")

	before := published.DeepCopy()
	require.True(t, reflect.DeepEqual(before, published), "control: the deep copy equals the original before any build")
	obs := vObservation{publishedBefore: vJSON(t, published)}

	rtConf := evalQualifiedRuntime(conf, crd.ProviderName)
	builder := newEvalMiddlewareBuilder(t, rtConf.Middlewares, vPluginBuilder(t), provider)
	for range 2 {
		chain := builder.BuildMiddlewareChain(t.Context(), []string{evalQualifiedName(vMiddleware, crd.ProviderName)})
		h, err := chain.Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
		obs.errs = append(obs.errs, err)
		obs.returned = append(obs.returned, evalErrorTexts(err)...)
		if err == nil {
			obs.handler = h
			if serve {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
				require.Equal(t, http.StatusOK, rec.Code)
			}
		}
	}

	info := evalMiddlewareInfo(rtConf, vMiddleware)
	require.NotNil(t, info)
	obs.stored = append([]string(nil), info.Err...)
	obs.publishedAfter = vJSON(t, published)
	obs.publishedUnchanged = obs.publishedBefore == obs.publishedAfter
	obs.runtimeUnchanged = obs.publishedBefore == vJSON(t, info.Middleware)
	// DeepEqual also covers the non-serialized Secret value side map.
	obs.deepEqualUnchanged = reflect.DeepEqual(before, published) && reflect.DeepEqual(before, info.Middleware)
	return obs
}

func vServe(t *testing.T, h http.Handler) string {
	t.Helper()
	require.NotNil(t, h)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	return rec.Body.String()
}

func TestVerify_C1_Execution(t *testing.T) {
	evalProviderMu.Lock()
	defer evalProviderMu.Unlock()

	obs := vBuildTwice(t, vInput(nil), vSecret(nil), false)
	require.NoError(t, obs.errs[0])
	require.NoError(t, obs.errs[1])
	body := vServe(t, obs.handler)
	t.Logf("C1 execution: plugin served decoded config = %s", body)

	expected := `{
	  "port": 8443, "enabled": false, "token": "tok-VALUE", "public": "PUBLIC-LITERAL",
	  "deep": {"inner": {"token": "deep-VALUE", "label": "literal-label"}},
	  "users": [{"name": "admin", "secret": "admin-VALUE"}, {"name": "user", "secret": "user-VALUE"}],
	  "remain": {
	    "a/b": "slash-VALUE", "a": {"b": "literal-a-b"}, "m~n": "tilde-VALUE", "~1": "escaped-VALUE", "/": "literal-slash",
	    "x.y-z": ["literal-0", {"~0": "listed-VALUE", "keep": "literal-keep"}]
	  }
	}`
	ok := assert.JSONEq(t, expected, body, "typed int, typed nested, typed list and special-character keys must carry the resolved Secret values")
	t.Logf("C1 execution: typed int field + nested + list + escaped keys resolved as expected = %v", ok)
	t.Logf("C1 execution: published config unchanged = %v, runtime config unchanged = %v, DeepEqual (incl. Secret side map) unchanged = %v",
		obs.publishedUnchanged, obs.runtimeUnchanged, obs.deepEqualUnchanged)
	assert.True(t, obs.publishedUnchanged && obs.runtimeUnchanged && obs.deepEqualUnchanged)
	assert.NotContains(t, obs.publishedAfter, "-VALUE")
	assert.NotContains(t, obs.publishedAfter, "MUTATED-BY-PLUGIN")
}

func TestVerify_C1_Isolation(t *testing.T) {
	evalProviderMu.Lock()
	defer evalProviderMu.Unlock()

	cases := []struct {
		name    string
		extra   map[string]any
		secret  map[string]string
		wantErr bool
	}{
		{name: "success (plugin mutates nested maps in New)"},
		{name: "decode error on Secret-bound int field", secret: map[string]string{"port": "not-a-number-SECRET"}, wantErr: true},
		{name: "decode error confined to unrelated literal field (reference-only replay)", extra: map[string]any{"enabled": "PUBLIC-NOT-A-BOOL", "port": 8443}, wantErr: true},
		{name: "literal-field decode error next to a Secret-bound typed field (reference-only replay)", extra: map[string]any{"enabled": "PUBLIC-NOT-A-BOOL"}, wantErr: true},
		{name: "constructor error after plugin mutates nested maps", extra: map[string]any{"fail_format": "plugin rejected value: %s"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obs := vBuildTwice(t, vInput(tc.extra), vSecret(tc.secret), true)
			for i, err := range obs.errs {
				if tc.wantErr {
					require.Error(t, err)
					if i == 0 || err.Error() != obs.errs[0].Error() {
						t.Logf("C1 isolation: attempt %d returned error = %q", i+1, err.Error())
					}
				} else {
					require.NoError(t, err)
				}
			}
			t.Logf("C1 isolation: published unchanged = %v, runtime unchanged = %v, DeepEqual (incl. Secret side map) unchanged = %v",
				obs.publishedUnchanged, obs.runtimeUnchanged, obs.deepEqualUnchanged)
			if !obs.publishedUnchanged {
				t.Logf("C1 isolation: BEFORE %s", obs.publishedBefore)
				t.Logf("C1 isolation: AFTER  %s", obs.publishedAfter)
			}
			assert.True(t, obs.publishedUnchanged, "published configuration mutated")
			assert.True(t, obs.runtimeUnchanged, "runtime configuration mutated")
			assert.True(t, obs.deepEqualUnchanged, "configuration (incl. Secret side map) mutated")
			assert.NotContains(t, obs.publishedAfter, "MUTATED-BY-PLUGIN")
			assert.NotContains(t, obs.publishedAfter, "-VALUE")
		})
	}

	// Positive control: the same snapshot comparison does detect a mutation when the
	// plugin is handed the published map itself, bypassing the middleware builder.
	t.Run("control: plugin built directly on the published map", func(t *testing.T) {
		conf, _ := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "vprobe", vPluginType, vInput(nil), vSecret(nil))
		published := conf.HTTP.Middlewares[vMiddleware]
		before := vJSON(t, published)
		// Shallow top-level copy without the typed int reference (a URN cannot decode
		// into an int); the nested containers are still the published ones.
		aliased := map[string]any{}
		for k, v := range published.Plugin[vPluginType] {
			if k != "port" {
				aliased[k] = v
			}
		}
		constructor, err := vPluginBuilder(t).Build(vPluginType, aliased, vMiddleware)
		require.NoError(t, err)
		_, err = constructor(t.Context(), http.NotFoundHandler())
		require.NoError(t, err)
		after := vJSON(t, published)
		t.Logf("C1 isolation control: published unchanged = %v (expected false: the probe detects aliasing)", before == after)
		assert.NotEqual(t, before, after)
		assert.Contains(t, after, "MUTATED-BY-PLUGIN")
	})
}

func TestVerify_C2_ErrorConfidentiality(t *testing.T) {
	evalProviderMu.Lock()
	defer evalProviderMu.Unlock()

	type secretCase struct {
		value     string
		fragments []string
	}
	secretA := secretCase{value: "qA7-distinct-SECRET-alpha", fragments: []string{"qA7", "SECRET-alpha", "distinct"}}
	secretB := secretCase{value: `rB8-other-"SECRET"\beta 'x'`, fragments: []string{"rB8", "SECRET", "beta", "other"}}

	shapes := []struct {
		name string
		// public literals, identical for both Secret values
		extra map[string]any
		// which Secret key carries the distinctive value
		secretKey string
		// literal text the error may legitimately quote
		publicQuote string
	}{
		{name: "decode error on Secret-bound int field (port)", secretKey: "port"},
		{name: "constructor error quoting Secret-bound token", extra: map[string]any{"fail_format": "plugin rejected value: '%s'"}, secretKey: "token"},
		{name: "decode error on fixed public literal field (enabled)", extra: map[string]any{"enabled": "PUBLIC-NOT-A-BOOL", "port": 8443}, secretKey: "token", publicQuote: "PUBLIC-NOT-A-BOOL"},
		{
			name: "fixture shape: fixed public literal equal to Secret value A", extra: map[string]any{"enabled": secretA.value, "port": 8443},
			secretKey: "token", publicQuote: secretA.value,
		},
	}

	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			var texts [2][]string
			for i, sc := range []secretCase{secretA, secretB} {
				obs := vBuildTwice(t, vInput(shape.extra), vSecret(map[string]string{shape.secretKey: sc.value}), false)
				require.Error(t, obs.errs[0])
				require.Error(t, obs.errs[1], "retry on the same builder must still fail")
				require.NotEmpty(t, obs.stored, "the failure must be stored on the runtime middleware")
				all := append(append([]string(nil), obs.returned...), obs.stored...)
				texts[i] = all
				t.Logf("C2 secret#%d=%q returned(1st)=%q", i+1, sc.value, obs.errs[0].Error())
				t.Logf("C2 secret#%d retry on the same builder returns the same text = %v", i+1, obs.errs[0].Error() == obs.errs[1].Error())
				t.Logf("C2 secret#%d=%q stored(MiddlewareInfo.Err)=%q", i+1, sc.value, obs.stored)
				leaked := false
				for _, text := range all {
					for _, fragment := range append([]string{sc.value}, sc.fragments...) {
						if shape.publicQuote != "" && strings.Contains(shape.publicQuote, fragment) {
							continue // the fragment is part of the fixed public literal
						}
						if strings.Contains(text, fragment) {
							leaked = true
							t.Errorf("C2 LEAK: %q contains Secret-derived fragment %q", text, fragment)
						}
					}
				}
				t.Logf("C2 secret#%d: Secret-derived text in returned/retried/stored errors = %v", i+1, leaked)
				assert.True(t, obs.deepEqualUnchanged)
			}
			identical := reflect.DeepEqual(texts[0], texts[1])
			t.Logf("C2 diagnostics identical for both Secret values (public literals fixed) = %v", identical)
			assert.True(t, identical, "diagnostics depend on the Secret value")
			if shape.publicQuote != "" {
				t.Logf("C2 quoted text %q originates from the fixed public literal: present for both Secret values = %v",
					shape.publicQuote, strings.Contains(texts[0][0], shape.publicQuote) && strings.Contains(texts[1][0], shape.publicQuote))
			}
		})
	}

	// Positive control: the raw production decoding error for the resolved value
	// does quote the Secret value, so the shapes above reach a leaking decoder.
	t.Run("control: raw plugins.Builder error for the resolved configuration", func(t *testing.T) {
		resolved := vInput(nil)
		resolved["port"] = secretA.value
		_, err := vPluginBuilder(t).Build(vPluginType, resolved, vMiddleware)
		require.Error(t, err)
		t.Logf("C2 control: raw decoder error = %q", err.Error())
		assert.Contains(t, err.Error(), secretA.value)
	})
}

var _ = dynamic.Middleware{}
