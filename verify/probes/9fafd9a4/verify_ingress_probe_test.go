// Audit instrumentation (not production code). Copy into pkg/provider/kubernetes/ingress-nginx/ of branch
// evalon/traefik-se-9fafd9a4 together with the byte-exact eval fixtures, then run:
//   go test -v -race -count=1 -run '^TestVerify_' ./pkg/provider/kubernetes/ingress-nginx
package ingressnginx_test

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	ingressnginx "github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx"
	"github.com/traefik/traefik/v3/pkg/safe"
	"github.com/traefik/traefik/v3/pkg/server"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type verifyProvider struct{ conf *dynamic.Configuration }

func (p *verifyProvider) Init() error { return nil }

func (p *verifyProvider) Provide(ch chan<- dynamic.Message, _ *safe.Pool) error {
	go func() { ch <- dynamic.Message{ProviderName: "kubernetesingressnginx", Configuration: p.conf} }()
	return nil
}

func verifyDeliver(t *testing.T, conf *dynamic.Configuration) dynamic.Configuration {
	t.Helper()

	pool := safe.NewPool(t.Context())
	t.Cleanup(pool.Stop)

	got := make(chan dynamic.Configuration, 1)
	watcher := server.NewConfigurationWatcher(pool, &verifyProvider{conf: conf}, nil, "", false)
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

func verifyDigest(t *testing.T, h http.Handler, user, realm, password string) int {
	t.Helper()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://localhost/", nil))
	require.Equal(t, http.StatusUnauthorized, rec.Code)

	challenge := rec.Header().Get("WWW-Authenticate")
	field := func(name string) string {
		i := strings.Index(challenge, name+`="`)
		require.GreaterOrEqual(t, i, 0, challenge)
		rest := challenge[i+len(name)+2:]
		return rest[:strings.Index(rest, `"`)]
	}
	sum := func(s string) string { d := md5.Sum([]byte(s)); return hex.EncodeToString(d[:]) }
	nonce, opaque := field("nonce"), field("opaque")
	ha1 := sum(user + ":" + realm + ":" + password)
	ha2 := sum("GET:/")
	nc, cnonce := "00000001", "0a4f113b"
	response := sum(strings.Join([]string{ha1, nonce, nc, cnonce, "auth", ha2}, ":"))

	req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
	req.Header.Set("Authorization", fmt.Sprintf(`Digest username="%s", realm="%s", nonce="%s", uri="/", qop=auth, nc=%s, cnonce="%s", response="%s", opaque="%s", algorithm=MD5`, user, realm, nonce, nc, cnonce, response, opaque))
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestVerify_C6_AuthFileWhitespace(t *testing.T) {
	basicEntry := "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"
	realm := "Authentication Required"
	ha1 := md5.Sum([]byte("user:" + realm + ":password"))
	digestEntry := "user:" + realm + ":" + hex.EncodeToString(ha1[:])

	for _, kind := range []string{"basic", "digest"} {
		entry, secretName, fixture := basicEntry, "basic-auth", "ingresses/ingress-with-basicauth.yml"
		if kind == "digest" {
			entry, secretName, fixture = digestEntry, "digest-auth", "ingresses/ingress-with-digestauth.yml"
		}

		for _, tc := range []struct{ desc, content string }{
			{"normalized", entry},
			{"padded-no-trailing-newline", "  " + entry + "  "},
			{"padded-trailing-newline", "  " + entry + "  \n"},
			{"tabs-crlf-blank-and-comment-lines", "# comment\r\n\r\n\t" + entry + " \t\r\n\r\n"},
		} {
			t.Run(kind+"/"+tc.desc, func(t *testing.T) {
				conf, _ := ingressnginx.LoadIngressesForEvalTest(t, []string{"services.yml", "ingressclasses.yml", fixture},
					&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: "default"}, Data: map[string][]byte{"auth": []byte(tc.content)}})
				require.NotNil(t, conf.HTTP)

				var name string
				var users []string
				for n, m := range conf.HTTP.Middlewares {
					if m.BasicAuth != nil {
						name, users = n, m.BasicAuth.Users
					}
					if m.DigestAuth != nil {
						name, users = n, m.DigestAuth.Users
					}
				}
				require.NotEmpty(t, name)

				// Fixture-style construction (unqualified name, no watcher), for comparison.
				_, unqualifiedErr := middleware.NewBuilder(runtime.NewConfig(*conf).Middlewares, nil, nil).
					BuildMiddlewareChain(t.Context(), []string{name}).Then(http.NotFoundHandler())

				rtConf := runtime.NewConfig(verifyDeliver(t, conf))
				qualified := name + "@kubernetesingressnginx"
				require.Contains(t, rtConf.Middlewares, qualified)

				h, err := middleware.NewBuilder(rtConf.Middlewares, nil, nil).BuildMiddlewareChain(t.Context(), []string{qualified}).
					Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
				require.NoError(t, err)

				var good, bad int
				if kind == "basic" {
					status := func(password string) int {
						req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
						req.SetBasicAuth("test", password)
						rec := httptest.NewRecorder()
						h.ServeHTTP(rec, req)
						return rec.Code
					}
					good, bad = status("test"), status("wrong")
				} else {
					good, bad = verifyDigest(t, h, "user", realm, "password"), verifyDigest(t, h, "user", realm, "wrong")
				}

				t.Logf("%s %s content=%q users=%q qualified=%s build_err=%v valid=%d invalid=%d | unqualified fixture-style build err=%v",
					kind, tc.desc, tc.content, users, qualified, err, good, bad, unqualifiedErr)
				assert.Equal(t, http.StatusOK, good)
				assert.Equal(t, http.StatusUnauthorized, bad)
			})
		}
	}
}
