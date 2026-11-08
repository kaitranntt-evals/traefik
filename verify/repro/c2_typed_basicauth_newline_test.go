// Run: cp verify/repro/c2_export_verify.go pkg/provider/kubernetes/crd/ && cp verify/repro/c2_typed_basicauth_newline_test.go pkg/server/middleware/ && go test -tags verify -count=1 -v -run TestVerifyC2 ./pkg/server/middleware/  (on branch evalon/traefik-se-67c2dfcc)

//go:build verify

package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// $apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/ is the htpasswd hash of the password "test".
const injectedHash = "$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"

func TestVerifyC2_TypedBasicAuthNewlineInjection(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "typed"},
		Type:       corev1.SecretTypeBasicAuth,
		Data: map[string][]byte{
			"username": []byte("mallory:" + injectedHash + "\nalice"),
			"password": []byte("alicepw"),
		},
	}

	secrets := dynamic.Secrets{}
	users, err := crd.VerifyLoadBasicAuthCredentials(secret, secrets)
	require.NoError(t, err)
	t.Logf("provider users=%q", users)
	t.Logf("provider secrets=%q", secrets)

	rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{
		"auth": {BasicAuth: &dynamic.BasicAuth{Users: users}, Secrets: secrets},
	}}})
	builder := middleware.NewBuilder(rtConf.Middlewares, nil, nil)
	handler, err := builder.BuildMiddlewareChain(t.Context(), []string{"auth"}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
	t.Logf("build err=%v infoErr=%v", err, rtConf.Middlewares["auth"].Err)
	require.NoError(t, err)

	status := func(user, password string) int {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		req.SetBasicAuth(user, password)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		t.Logf("%s/%s -> %d", user, password, rec.Code)
		return rec.Code
	}

	assert.Equal(t, http.StatusOK, status("alice", "alicepw"))
	assert.Equal(t, http.StatusUnauthorized, status("mallory", "wrong"))
	// The claim: mallory authenticates with the password of the hash injected through the username.
	assert.Equal(t, http.StatusUnauthorized, status("mallory", "test"), "mallory must NOT be able to authenticate")
}
