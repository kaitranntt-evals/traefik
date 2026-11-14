// Run (on a worktree of evalon/traefik-se-21bb188b): cp verify/repro/c10_*_test.go <worktree>/pkg/provider/kubernetes/crd/ && cd <worktree> && go test -count=1 -v -run '^TestVerifyC10' ./pkg/provider/kubernetes/crd
package crd_test

import (
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func c10SHA(user, password string) string {
	sum := sha1.Sum([]byte(password))
	return user + ":{SHA}" + base64.StdEncoding.EncodeToString(sum[:])
}

func c10Status(t *testing.T, conf *dynamic.Configuration, user, password string) int {
	t.Helper()
	name := "default-auth@kubernetescrd"
	rt := map[string]*runtime.MiddlewareInfo{name: {Middleware: conf.HTTP.Middlewares["default-auth"]}}
	h, err := middleware.NewBuilder(rt, nil, nil).BuildMiddlewareChain(t.Context(), []string{name}).
		Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.SetBasicAuth(user, password)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestVerifyC10_MixedVersionBinding(t *testing.T) {
	meta := metav1.ObjectMeta{Name: "authsecret", Namespace: "default"}
	versionA := &corev1.Secret{ObjectMeta: meta, Data: map[string][]byte{"users-v1": []byte(c10SHA("alice", "old-password") + "\n")}}
	versionB := &corev1.Secret{ObjectMeta: meta, Data: map[string][]byte{"users-v2": []byte(c10SHA("bob", "new-password") + "\n")}}
	mw := &traefikv1alpha1.Middleware{
		ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"},
		Spec:       traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "authsecret"}},
	}

	for _, test := range []struct {
		desc        string
		switchAfter int
	}{
		{desc: "control: whole load sees version A", switchAfter: 1000},
		{desc: "control: whole load sees version B", switchAfter: 0},
		{desc: "interleaved: Secret rotates between the two reads of one load", switchAfter: 1},
	} {
		t.Run(test.desc, func(t *testing.T) {
			conf, reads := crd.C10Load(t, mw, versionA, versionB, test.switchAfter)

			m := conf.HTTP.Middlewares["default-auth"]
			require.NotNil(t, m)
			t.Logf("GetSecret reads during one load: %d", reads)
			t.Logf("published users (reference):    %v", m.BasicAuth.Users)
			t.Logf("bound runtime credentials:      %v", m.SecretValues)
			t.Logf("alice/old-password -> %d", c10Status(t, conf, "alice", "old-password"))
			t.Logf("bob/new-password   -> %d", c10Status(t, conf, "bob", "new-password"))
		})
	}
}
