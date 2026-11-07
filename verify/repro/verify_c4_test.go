//go:build verify

// Run (needs eval fixture export_eval_ingress_nginx_test.go in this dir):
// cp verify/repro/verify_c4_test.go pkg/provider/kubernetes/ingress-nginx/ && go test -tags verify -count=1 -run TestVerifyC4 -v ./pkg/provider/kubernetes/ingress-nginx
package ingressnginx_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ingressnginx "github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx"
)

func TestVerifyC4_AuthMapNewlineCreatesExtraUser(t *testing.T) {
	aliceHash, err := bcrypt.GenerateFromPassword([]byte("alice-pass"), bcrypt.MinCost)
	require.NoError(t, err)
	bobHash, err := bcrypt.GenerateFromPassword([]byte("bob-pass"), bcrypt.MinCost)
	require.NoError(t, err)

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ingress-auth-map", Namespace: "default",
			Annotations: map[string]string{
				"nginx.ingress.kubernetes.io/auth-type":        "basic",
				"nginx.ingress.kubernetes.io/auth-secret-type": "auth-map",
				"nginx.ingress.kubernetes.io/auth-secret":      "default/auth-map-secret",
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: new("nginx"),
			Rules: []netv1.IngressRule{{
				Host: "whoami.localhost",
				IngressRuleValue: netv1.IngressRuleValue{HTTP: &netv1.HTTPIngressRuleValue{Paths: []netv1.HTTPIngressPath{{
					Path: "/authmap", PathType: new(netv1.PathTypeExact),
					Backend: netv1.IngressBackend{Service: &netv1.IngressServiceBackend{Name: "whoami", Port: netv1.ServiceBackendPort{Number: 80}}},
				}}}},
			}},
		},
	}
	// Single key "alice"; its value embeds a second credential line for "bob", who is not a key of the Secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "auth-map-secret", Namespace: "default"},
		Data:       map[string][]byte{"alice": []byte(string(aliceHash) + "\nbob:" + string(bobHash))},
	}

	conf, provider := ingressnginx.LoadIngressesForEvalTest(t, []string{"services.yml", "ingressclasses.yml"}, ingress, secret)
	require.NotNil(t, conf)

	var name string
	for n, m := range conf.HTTP.Middlewares {
		if m.BasicAuth != nil {
			name = n
			t.Logf("middleware %q published users: %q", n, m.BasicAuth.Users)
		}
	}
	require.NotEmpty(t, name)

	rtConf := runtime.NewConfig(*conf)
	builder := newEvalIngressMiddlewareBuilder(t, rtConf.Middlewares, provider)
	h, err := builder.BuildMiddlewareChain(t.Context(), []string{name}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
	t.Logf("build error: %v", err)
	t.Logf("stored info.Err: %q", rtConf.Middlewares[name].Err)
	if err != nil {
		t.Log("middleware rejected the value")
		return
	}

	status := func(user, pass string) int {
		req := httptest.NewRequest(http.MethodGet, "http://whoami.localhost/authmap", nil)
		req.SetBasicAuth(user, pass)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	t.Logf("alice/alice-pass -> %d", status("alice", "alice-pass"))
	t.Logf("bob/bob-pass     -> %d (bob is NOT a key of the Secret)", status("bob", "bob-pass"))
	t.Logf("bob/wrong        -> %d", status("bob", "wrong"))
	if status("bob", "bob-pass") == http.StatusOK {
		t.Log("CONFIRMED: the embedded line authenticates as an independent user")
	}
	_ = middleware.NewBuilder
	_ = dynamic.Users(nil)
}
