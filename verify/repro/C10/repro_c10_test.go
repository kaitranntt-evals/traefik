// Run: sh verify/repro/C10_replaced_bindings.sh (it copies this file into pkg/provider/kubernetes/crd/ of evalon/traefik-se-9fafd9a4); PASS means the problem reproduced.
package crd_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ptypes "github.com/traefik/paerser/types"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/config/static"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/aggregator"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	apiextensionv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

const (
	c10Basic  = "default-basicauth@kubernetescrd"
	c10Plugin = "default-test-secret@kubernetescrd"
	c10Hash   = "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\n"
)

type verifyProvider struct {
	run func(ch chan<- dynamic.Message)
}

func (p *verifyProvider) Init() error { return nil }

func (p *verifyProvider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	go p.run(ch)
	return nil
}

// verifyDeliver sends one provider configuration through a real ConfigurationWatcher and returns what listeners receive.
func verifyDeliver(t *testing.T, conf *dynamic.Configuration) dynamic.Configuration {
	t.Helper()

	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)

	got := make(chan dynamic.Configuration, 1)
	watcher := server.NewConfigurationWatcher(pool, &verifyProvider{run: func(ch chan<- dynamic.Message) {
		ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: conf}
	}}, nil, "", false)
	watcher.AddListener(func(c dynamic.Configuration) { got <- c })
	watcher.Start()

	select {
	case c := <-got:
		return c
	case <-time.After(10 * time.Second):
		t.Fatal("configuration was not delivered")
		return dynamic.Configuration{}
	}
}

func verifyBuild(t *testing.T, rtConf *runtime.Configuration, pluginBuilder middleware.PluginsBuilder, name string) (http.Handler, error) {
	t.Helper()

	builder := middleware.NewBuilder(rtConf.Middlewares, nil, pluginBuilder)
	return builder.BuildMiddlewareChain(t.Context(), []string{name}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
}

func c10BasicStatus(h http.Handler, user, password string) int {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.SetBasicAuth(user, password)
	h.ServeHTTP(rec, req)
	return rec.Code
}

const (
	c10Works  = `build_err=<nil> test:test=200 | plugin declared=urn:k8s:secret:name:key received=["this_is_the_secret"]`
	c10Broken = `build_err=error parsing BasicAuth: invalid user entry at position 0 test:test=0 | plugin declared=urn:k8s:secret:name:key received=["urn:k8s:secret:name:key"]`
)

type c10PluginBuilder struct {
	mu       sync.Mutex
	received []string
}

func (b *c10PluginBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	b.mu.Lock()
	b.received = append(b.received, fmt.Sprint(config["secret"]))
	b.mu.Unlock()
	return func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }, nil
}

// c10Outcome builds the Secret-backed middlewares of an applied configuration, as the router factory does.
func c10Outcome(t *testing.T, conf dynamic.Configuration) string {
	t.Helper()

	rtConf := runtime.NewConfig(conf)
	if rtConf.Middlewares[c10Basic] == nil {
		return "no kubernetescrd configuration"
	}

	pluginBuilder := &c10PluginBuilder{}
	h, err := verifyBuild(t, rtConf, pluginBuilder, c10Basic)
	status := 0
	if err == nil {
		status = c10BasicStatus(h, "test", "test")
	}
	_, pluginErr := verifyBuild(t, rtConf, pluginBuilder, c10Plugin)

	return fmt.Sprintf("basicAuth users=%q revision=%s build_err=%v test:test=%d | plugin declared=%v received=%q build_err=%v",
		rtConf.Middlewares[c10Basic].BasicAuth.Users, rtConf.Middlewares[c10Basic].SecretsRevision, err, status,
		rtConf.Middlewares[c10Plugin].Plugin["test-secret"]["secret"], pluginBuilder.received, pluginErr)
}

// c10Replace migrates both middlewares to new Secrets holding the same credentials: a routine credential-store change.
func c10Replace(t *testing.T, h *crd.VerifyHarness) {
	t.Helper()

	h.SetSecret(t, "default", "authsecret2", map[string][]byte{"users": []byte(c10Hash)})
	h.SetSecret(t, "default", "name2", map[string][]byte{"key": []byte("this_is_the_secret")})
	h.UpdateMiddleware(t, "default", "basicauth",
		func(m *traefikv1alpha1.Middleware) { m.Spec.BasicAuth.Secret = "authsecret2" },
		func(m *traefikv1alpha1.Middleware) bool { return m.Spec.BasicAuth.Secret == "authsecret2" })
	raw := []byte(`{"user":"admin","secret":"urn:k8s:secret:name2:key"}`)
	h.UpdateMiddleware(t, "default", "test-secret",
		func(m *traefikv1alpha1.Middleware) { m.Spec.Plugin["test-secret"] = apiextensionv1.JSON{Raw: raw} },
		func(m *traefikv1alpha1.Middleware) bool {
			return string(m.Spec.Plugin["test-secret"].Raw) == string(raw)
		})
}

func c10Harness(t *testing.T) *crd.VerifyHarness {
	t.Helper()

	h := crd.NewVerifyHarness(t, []string{"services.yml", "with_auth.yml", "with_plugin_read_secret.yml"})
	h.SetSecret(t, "default", "authsecret", map[string][]byte{"users": []byte(c10Hash)})
	return h
}

// Part "binding lifetime": retain A, load replacement B, build A.
func TestRepro_C10_BindingLifetime(t *testing.T) {
	h := c10Harness(t)

	confA := h.Load(t.Context())
	deliveredA := verifyDeliver(t, confA)
	before := c10Outcome(t, deliveredA)
	t.Logf("A built while A's bindings are published : %s", before)
	require.Contains(t, before, c10Works)

	c10Replace(t, h)
	confB := h.Load(t.Context())
	after := c10Outcome(t, deliveredA)
	t.Logf("A built after replacement B was loaded   : %s", after)
	// The problem: the retained configuration A is unchanged, yet it no longer builds.
	assert.Contains(t, after, c10Broken)
	t.Logf("B built after replacement B was loaded   : %s", c10Outcome(t, verifyDeliver(t, confB)))
	t.Logf("SecretsRevision A=%s B=%s (A's revision is still on the retained configuration, but is not used to find A's values)",
		confA.HTTP.Middlewares["default-basicauth"].SecretsRevision, confB.HTTP.Middlewares["default-basicauth"].SecretsRevision)
}

// Part "queued construction", ordering 1: A is pending in the real ConfigurationWatcher while its applier is busy;
// the provider loads B (publishing B's bindings inside loadConfigurationFromCRD) before B is delivered.
func TestRepro_C10_WatcherBuildsPendingA(t *testing.T) {
	h := c10Harness(t)

	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)

	var (
		mu       sync.Mutex
		applied  []string
		gate     = make(chan struct{})
		blocked  = make(chan struct{})
		aQueued  = make(chan struct{})
		aBuilt   = make(chan struct{})
		bBuilt   = make(chan struct{})
		once     sync.Once
		onceA    sync.Once
		bLoaded  bool
		revision = map[string]string{}
	)

	watcher := server.NewConfigurationWatcher(pool, &verifyProvider{run: func(ch chan<- dynamic.Message) {
		ch <- dynamic.Message{ProviderName: "other", Configuration: c10OtherConf("v1")}
		<-blocked // the applier is now busy applying the other provider's configuration.

		confA := h.Load(context.Background())
		mu.Lock()
		revision[confA.HTTP.Middlewares["default-basicauth"].SecretsRevision] = "A"
		mu.Unlock()
		ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: confA}
		<-aQueued // the watcher has received A: it is its pending configuration.

		c10Replace(t, h)
		confB := h.Load(context.Background()) // Store.Set replaces the kubernetescrd bindings here.
		mu.Lock()
		revision[confB.HTTP.Middlewares["default-basicauth"].SecretsRevision] = "B"
		bLoaded = true
		mu.Unlock()

		close(gate) // the applier becomes free before B reaches the watcher.
		<-aBuilt
		ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: confB}
	}}, nil, "", false)

	// Runs in receiveConfigurations for every accepted message: tells when A became the pending configuration.
	watcher.AddTransformer(func(_ context.Context, configs dynamic.Configurations) dynamic.Configurations {
		if _, ok := configs[crd.ProviderName]; ok {
			onceA.Do(func() { close(aQueued) })
		}
		return configs
	})

	watcher.AddListener(func(conf dynamic.Configuration) {
		first := false
		once.Do(func() { first = true })
		if first {
			close(blocked)
			<-gate
			return
		}

		outcome := c10Outcome(t, conf)
		mu.Lock()
		which := revision[conf.HTTP.Middlewares[c10Basic].SecretsRevision]
		applied = append(applied, fmt.Sprintf("watcher applied configuration %s (B already loaded by provider: %v): %s", which, bLoaded, outcome))
		mu.Unlock()
		if which == "A" {
			close(aBuilt)
		} else {
			close(bBuilt)
		}
	})
	watcher.Start()

	select {
	case <-bBuilt:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	for _, line := range applied {
		t.Log(line)
	}
	require.Len(t, applied, 2)
	// The problem: the watcher built pending A after the provider had replaced its bindings.
	assert.Contains(t, applied[0], "watcher applied configuration A (B already loaded by provider: true)")
	assert.Contains(t, applied[0], c10Broken)
}

func c10OtherConf(version string) *dynamic.Configuration {
	return &dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
		Routers:     map[string]*dynamic.Router{},
		Services:    map[string]*dynamic.Service{},
		Middlewares: map[string]*dynamic.Middleware{"marker": {AddPrefix: &dynamic.AddPrefix{Prefix: "/" + version}}},
	}}
}

type c10Scripted struct {
	run func(ch chan<- dynamic.Message)
}

func (p *c10Scripted) Init() error { return nil }

func (p *c10Scripted) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	go p.run(ch)
	return nil
}

// Part "queued construction", ordering 2: the real ProviderAggregator with the default 2s providersThrottleDuration.
// The CRD provider has no ThrottleDuration method, so its messages go through the aggregator throttle: B is held back
// while its bindings are already published, and any other provider update makes the watcher rebuild A in the meantime.
func TestRepro_C10_ThrottledReplacement(t *testing.T) {
	_, perProvider := any(&crd.Provider{}).(interface{ ThrottleDuration() time.Duration })
	t.Logf("crd.Provider implements ThrottleDuration(): %v (false => aggregator applies providersThrottleDuration)", perProvider)

	h := c10Harness(t)

	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)

	var (
		mu        sync.Mutex
		applied   []string
		start     = time.Now()
		revision  = map[string]string{}
		aApplied  = make(chan struct{})
		bLoaded   = make(chan struct{})
		bApplied  = make(chan struct{})
		onceA     sync.Once
		onceB     sync.Once
		loadedAtB time.Duration
	)

	agg := aggregator.NewProviderAggregator(static.Providers{ProvidersThrottleDuration: ptypes.Duration(2 * time.Second)})
	require.NoError(t, agg.AddProvider(&c10Scripted{run: func(ch chan<- dynamic.Message) {
		confA := h.Load(context.Background())
		mu.Lock()
		revision[confA.HTTP.Middlewares["default-basicauth"].SecretsRevision] = "A"
		mu.Unlock()
		ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: confA}
		<-aApplied

		c10Replace(t, h)
		confB := h.Load(context.Background())
		mu.Lock()
		revision[confB.HTTP.Middlewares["default-basicauth"].SecretsRevision] = "B"
		loadedAtB = time.Since(start)
		mu.Unlock()
		ch <- dynamic.Message{ProviderName: crd.ProviderName, Configuration: confB}
		close(bLoaded)
	}}))
	require.NoError(t, agg.AddProvider(&c10Scripted{run: func(ch chan<- dynamic.Message) {
		<-bLoaded
		time.Sleep(200 * time.Millisecond)
		ch <- dynamic.Message{ProviderName: "other", Configuration: c10OtherConf("v2")}
	}}))

	watcher := server.NewConfigurationWatcher(pool, agg, nil, "", false)
	watcher.AddListener(func(conf dynamic.Configuration) {
		outcome := c10Outcome(t, conf)
		mu.Lock()
		which := revision[conf.HTTP.Middlewares[c10Basic].SecretsRevision]
		_, other := conf.HTTP.Middlewares["marker@other"]
		applied = append(applied, fmt.Sprintf("t=%4dms watcher applied kubernetescrd configuration %s (other provider update included: %v): %s",
			time.Since(start).Milliseconds(), which, other, outcome))
		mu.Unlock()
		if which == "A" {
			onceA.Do(func() { close(aApplied) })
		} else {
			onceB.Do(func() { close(bApplied) })
		}
	})
	watcher.Start()

	select {
	case <-bApplied:
	case <-time.After(20 * time.Second):
		t.Fatal("timeout")
	}

	mu.Lock()
	defer mu.Unlock()
	t.Logf("provider loaded replacement B (bindings replaced) at t=%dms", loadedAtB.Milliseconds())
	for _, line := range applied {
		t.Log(line)
	}
	require.GreaterOrEqual(t, len(applied), 3)
	// The problem: A is rebuilt (because of an unrelated provider update) while B is held by the aggregator throttle.
	assert.Contains(t, applied[1], "configuration A (other provider update included: true)")
	assert.Contains(t, applied[1], c10Broken)
}
