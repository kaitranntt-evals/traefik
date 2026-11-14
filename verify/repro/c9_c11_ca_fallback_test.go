// Run: cp verify/repro/c9_c11_ca_fallback_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -v -run '^TestVerifyCAFallback' ./pkg/provider/kubernetes/crd ; rm pkg/provider/kubernetes/crd/c9_c11_ca_fallback_test.go
package crd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestVerifyCAFallback(t *testing.T) {
	for _, test := range []struct {
		desc   string
		secret map[string][]byte
		value  string
	}{
		{desc: "C9 single-key credential Secret", secret: map[string][]byte{"token": []byte("s3cr3t-api-token-C9")}, value: "s3cr3t-api-token-C9"},
		{desc: "C11 single-key htpasswd Secret", secret: map[string][]byte{"users": []byte("alice:$apr1$C11salt$kT7QvC6cFh1cZp0pHq0aB/\n")}, value: "$apr1$C11salt$kT7QvC6cFh1cZp0pHq0aB/"},
		{desc: "control: two-key Secret without CA keys", secret: map[string][]byte{"username": []byte("u"), "password": []byte("two-key-password")}, value: "two-key-password"},
	} {
		t.Run(test.desc, func(t *testing.T) {
			secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "not-a-ca", Namespace: "default"}, Data: test.secret}
			fa := &traefikv1alpha1.Middleware{
				ObjectMeta: metav1.ObjectMeta{Name: "fa", Namespace: "default"},
				Spec: traefikv1alpha1.MiddlewareSpec{ForwardAuth: &traefikv1alpha1.ForwardAuth{
					Address: "https://auth.example.com", TLS: &traefikv1alpha1.ClientTLSWithCAOptional{ClientTLS: traefikv1alpha1.ClientTLS{CASecret: "not-a-ca"}},
				}},
			}
			rl := &traefikv1alpha1.Middleware{
				ObjectMeta: metav1.ObjectMeta{Name: "rl", Namespace: "default"},
				Spec: traefikv1alpha1.MiddlewareSpec{RateLimit: &traefikv1alpha1.RateLimit{
					Average: ptrInt64(10),
					Redis:   &traefikv1alpha1.Redis{Endpoints: []string{"127.0.0.1:6379"}, TLS: &traefikv1alpha1.ClientTLS{CASecret: "not-a-ca"}},
				}},
			}

			client := newClientImpl(kubefake.NewClientset(secret), traefikcrdfake.NewClientset(fa, rl))
			stopCh := make(chan struct{})
			t.Cleanup(func() { close(stopCh) })
			eventCh, err := client.WatchAll(nil, stopCh)
			require.NoError(t, err)
			<-eventCh

			loaded, loadErr := loadCASecret("default", "not-a-ca", client)
			t.Logf("loadCASecret -> value=%q err=%v", loaded, loadErr)

			p := Provider{}
			require.NoError(t, p.Init())
			conf := p.loadConfigurationFromCRD(t.Context(), client)

			for _, name := range []string{"default-fa", "default-rl"} {
				m := conf.HTTP.Middlewares[name]
				if m == nil {
					t.Logf("%s: middleware not published", name)
					continue
				}
				raw, err := json.Marshal(m)
				require.NoError(t, err)
				t.Logf("%s public JSON: %s", name, raw)
				t.Logf("%s public JSON contains the Secret value: %v", name, strings.Contains(string(raw), test.value))
			}
			assert.NotNil(t, conf)
		})
	}
}

func ptrInt64(v int64) *int64 { return &v }
