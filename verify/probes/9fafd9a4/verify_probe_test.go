// Audit instrumentation (not production code). Copy into pkg/provider/kubernetes/crd/ of branch evalon/traefik-se-9fafd9a4
// together with verify_harness_test.go and the byte-exact eval fixtures, then run:
//   go test -v -race -count=1 -run '^TestVerify_' ./pkg/provider/kubernetes/crd
// Every probe goes through the production delivery path: CRD loader -> server.ConfigurationWatcher (which qualifies
// middleware names with @kubernetescrd) -> runtime.NewConfig -> middleware.Builder.
package crd_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
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

func verifyDigestUser(user, password string) string {
	ha1 := md5.Sum([]byte(user + ":traefik:" + password))
	return user + ":traefik:" + hex.EncodeToString(ha1[:])
}

func TestVerify_C7_DigestAuthProductionPath(t *testing.T) {
	_, conf, _ := crd.LoadConfigurationWithSecretUpdate(t, []string{"services.yml", "with_auth.yml"}, "authsecret", "default", "users", verifyDigestUser("test", "test"))

	t.Logf("provider users: %q", conf.HTTP.Middlewares["default-digestauth"].DigestAuth.Users)

	// Same construction as the eval fixture: unqualified name, no watcher.
	_, err := verifyBuild(t, runtime.NewConfig(*conf), nil, "default-digestauth")
	t.Logf("unqualified (fixture-style) build error: %v", err)

	delivered := verifyDeliver(t, conf)
	rtConf := runtime.NewConfig(delivered)
	for name := range rtConf.Middlewares {
		t.Logf("delivered middleware name: %s", name)
	}

	h, err := verifyBuild(t, rtConf, nil, "default-digestauth@kubernetescrd")
	require.NoError(t, err)
	okStatus := evalDigestStatus(t, h, "test", "test")
	badStatus := evalDigestStatus(t, h, "test", "wrong")
	t.Logf("qualified build: err=%v matching credentials=%d wrong password=%d", err, okStatus, badStatus)
	assert.Equal(t, http.StatusOK, okStatus)
	assert.Equal(t, http.StatusUnauthorized, badStatus)
	t.Logf("shared users after build: %q", rtConf.Middlewares["default-digestauth@kubernetescrd"].DigestAuth.Users)
}

func TestVerify_C8_ConcurrentValidAuthBuilds(t *testing.T) {
	h := crd.NewVerifyHarness(t, []string{"services.yml", "with_auth.yml"})
	h.SetSecret(t, "default", "authsecret", map[string][]byte{"users": []byte(verifyDigestUser("test", "test") + "\ntest:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\n")})
	conf := h.Load(t.Context())

	// Digest and basic middlewares read the same Secret key; give each a list it can parse.
	h.SetSecret(t, "default", "authsecret", map[string][]byte{"users": []byte("test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\n")})
	basicConf := h.Load(t.Context())
	h.SetSecret(t, "default", "authsecret", map[string][]byte{"users": []byte(verifyDigestUser("test", "test") + "\n")})
	digestConf := h.Load(t.Context())
	_ = conf

	for _, tc := range []struct {
		name string
		conf *dynamic.Configuration
		mw   string
	}{
		{"basic", basicConf, "default-basicauth@kubernetescrd"},
		{"digest", digestConf, "default-digestauth@kubernetescrd"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Republish the bindings that belong to this configuration.
			if tc.name == "basic" {
				h.SetSecret(t, "default", "authsecret", map[string][]byte{"users": []byte("test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\n")})
			} else {
				h.SetSecret(t, "default", "authsecret", map[string][]byte{"users": []byte(verifyDigestUser("test", "test") + "\n")})
			}
			h.Load(t.Context())

			rtConf := runtime.NewConfig(verifyDeliver(t, tc.conf))
			builder := middleware.NewBuilder(rtConf.Middlewares, nil, nil)

			const concurrency = 40
			var wg sync.WaitGroup
			var mu sync.Mutex
			var failures []string
			stop := make(chan struct{})

			// The provider keeps reloading (and republishing bindings) while routers are built, as on Kubernetes events.
			var reloads int
			var reloadWG sync.WaitGroup
			reloadWG.Add(1)
			go func() {
				defer reloadWG.Done()
				for {
					select {
					case <-stop:
						return
					default:
						h.Load(context.Background())
						reloads++
					}
				}
			}()

			for range concurrency {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 20 {
						_, err := builder.BuildMiddlewareChain(t.Context(), []string{tc.mw}).Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
						if err != nil {
							mu.Lock()
							failures = append(failures, err.Error())
							mu.Unlock()
						}
					}
				}()
				wg.Add(1)
				go func() {
					defer wg.Done()
					for range 20 {
						// Same reader as the eval fixture: the API serializes the shared runtime configuration.
						if _, err := json.Marshal(rtConf); err != nil {
							mu.Lock()
							failures = append(failures, err.Error())
							mu.Unlock()
						}
					}
				}()
			}
			wg.Wait()
			close(stop)
			reloadWG.Wait()

			t.Logf("%s: %d concurrent builders x 20 builds, %d provider reloads, build failures=%d, MiddlewareInfo.Err=%v", tc.name, concurrency, reloads, len(failures), rtConf.Middlewares[tc.mw].Err)
			assert.Empty(t, failures)
		})
	}
}

func TestVerify_C9_SecretEntryPosition(t *testing.T) {
	valid := map[string]string{
		"default-basicauth":  "user%d:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/",
		"default-digestauth": "user%d:traefik:0123456789abcdef0123456789abcdef",
	}

	for _, name := range []string{"default-basicauth", "default-digestauth"} {
		for _, malformed := range []string{"badsentinel", "user", "format"} {
			for preceding := range 4 {
				var entries []string
				for i := range preceding {
					entries = append(entries, fmt.Sprintf(valid[name], i))
				}
				entries = append(entries, malformed, fmt.Sprintf(valid[name], 9))

				_, conf, _ := crd.LoadConfigurationWithSecretUpdate(t, []string{"services.yml", "with_auth.yml"}, "authsecret", "default", "users", strings.Join(entries, "\n"))
				rtConf := runtime.NewConfig(verifyDeliver(t, conf))
				_, err := verifyBuild(t, rtConf, nil, name+"@kubernetescrd")
				require.Error(t, err)

				stored := rtConf.Middlewares[name+"@kubernetescrd"].Err
				t.Logf("%s malformed=%q preceding_valid=%d expanded_index=%d -> returned=%q stored=%q", name, malformed, preceding, preceding, err.Error(), stored)

				want := fmt.Sprintf("line %d of", preceding+1)
				assert.Contains(t, err.Error(), want)
				assert.NotContains(t, err.Error(), malformed+"\"")
				for _, s := range stored {
					assert.Contains(t, s, want)
				}
			}
		}
	}
}

// Control for C8: the race reported by the eval fixture needs no Secret at all. Two failing builds of a middleware
// with an inline malformed user both call runtime.(*MiddlewareInfo).AddError, which is not synchronized.
func TestVerifyControl_C8_FailingBuildsRaceWithoutSecrets(t *testing.T) {
	rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
		Middlewares: map[string]*dynamic.Middleware{"inline@file": {BasicAuth: &dynamic.BasicAuth{Users: []string{"malformed"}}}},
	}})
	builder := middleware.NewBuilder(rtConf.Middlewares, nil, nil)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = builder.BuildMiddlewareChain(t.Context(), []string{"inline@file"}).Then(http.NotFoundHandler())
		}()
	}
	wg.Wait()
	t.Logf("inline (no Secret) failing builds recorded %d errors", len(rtConf.Middlewares["inline@file"].Err))
}
