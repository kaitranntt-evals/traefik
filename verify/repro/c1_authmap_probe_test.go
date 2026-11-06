//go:build ignore

// Run: in a checkout of any of the five C1 branches (also copy tests/export_eval_ingress_nginx_test.go from eval_tests.zip): sed '/^\/\/go:build ignore$/d' verify/repro/c1_authmap_probe_test.go > pkg/provider/kubernetes/ingress-nginx/c1_authmap_probe_test.go && go test -count=1 -v -run 'TestProbe_C1_AuthMapPublicEntries' ./pkg/provider/kubernetes/ingress-nginx
package ingressnginx_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	ingressnginx "github.com/traefik/traefik/v3/pkg/provider/kubernetes/ingress-nginx"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProbe_C1_AuthMapPublicEntries(t *testing.T) {
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
			Rules: []netv1.IngressRule{{Host: "whoami.localhost", IngressRuleValue: netv1.IngressRuleValue{HTTP: &netv1.HTTPIngressRuleValue{Paths: []netv1.HTTPIngressPath{{
				Path: "/authmap", PathType: new(netv1.PathTypeExact),
				Backend: netv1.IngressBackend{Service: &netv1.IngressServiceBackend{Name: "whoami", Port: netv1.ServiceBackendPort{Number: 80}}},
			}}}}}},
		},
	}
	authMapSec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "auth-map-secret", Namespace: "default"},
		Data: map[string][]byte{
			"alice": []byte("$apr1$DISTINCTHASHALICE$zzz"),
			"bob":   []byte("$apr1$DISTINCTHASHBOB$yyy"),
		},
	}
	conf, _ := ingressnginx.LoadIngressesForEvalTest(t, []string{"services.yml", "ingressclasses.yml"}, ingress, authMapSec)
	require.NotNil(t, conf)
	var mid *dynamic.Middleware
	for _, m := range conf.HTTP.Middlewares {
		if m.BasicAuth != nil {
			mid = m
		}
	}
	require.NotNil(t, mid)
	for _, u := range mid.BasicAuth.Users {
		t.Logf("USER ENTRY: %q", u)
	}
	js, err := json.Marshal(conf)
	require.NoError(t, err)
	t.Logf("JSON contains DISTINCTHASH: %v", strings.Contains(string(js), "DISTINCTHASH"))
	t.Logf("Users contain DISTINCTHASH: %v", strings.Contains(strings.Join(mid.BasicAuth.Users, "|"), "DISTINCTHASH"))
	mj, _ := json.Marshal(mid)
	t.Logf("MIDDLEWARE JSON: %s", mj)
}
