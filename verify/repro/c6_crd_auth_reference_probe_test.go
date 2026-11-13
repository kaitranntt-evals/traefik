//go:build verify

// Run (on evalon/traefik-se-12ccb497): cp this file to pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/provider/kubernetes/crd
package crd

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func verifyC6Load(t *testing.T, spec traefikv1alpha1.MiddlewareSpec, typ corev1.SecretType, data map[string][]byte) *dynamic.Middleware {
	t.Helper()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "default"}, Type: typ, Data: data}
	mw := &traefikv1alpha1.Middleware{ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"}, Spec: spec}
	client := newClientImpl(kubefake.NewClientset(secret), traefikcrdfake.NewClientset(mw))
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh
	p := Provider{}
	conf := p.loadConfigurationFromCRD(t.Context(), client)
	m := conf.HTTP.Middlewares["default-auth"]
	require.NotNil(t, m)
	return m
}

func verifyC6Users(m *dynamic.Middleware) dynamic.Users {
	if m.BasicAuth != nil {
		return m.BasicAuth.Users
	}
	return m.DigestAuth.Users
}

// For every key named by a composed auth reference: the key exists in Secret.Data, and changing only
// that key's value changes the credential the builder resolves.
func TestVerify_C6_ComposedReferencesNameContributingKeys(t *testing.T) {
	cases := []struct {
		desc string
		spec traefikv1alpha1.MiddlewareSpec
		typ  corev1.SecretType
		data map[string][]byte
	}{
		{"basicAuth kubernetes.io/basic-auth (username+password)", traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "creds"}}, corev1.SecretTypeBasicAuth, map[string][]byte{"username": []byte("alice"), "password": []byte("s3cr3t")}},
		{"basicAuth htpasswd single key", traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "creds"}}, corev1.SecretTypeOpaque, map[string][]byte{"my.users": []byte("alice:$apr1$aaaa$bbbb\nbob:$apr1$cccc$dddd\n")}},
		{"digestAuth single key", traefikv1alpha1.MiddlewareSpec{DigestAuth: &traefikv1alpha1.DigestAuth{Secret: "creds"}}, corev1.SecretTypeOpaque, map[string][]byte{"htdigest": []byte("alice:traefik:a2688e031edb4be6a3797f3882655c05\n")}},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			m := verifyC6Load(t, c.spec, c.typ, c.data)
			refs := verifyC6Users(m)
			base := verifyC6Users(m.ResolveSecretValues())
			t.Logf("Secret.Data keys=%v", strings.Join(sortedKeys(c.data), ","))
			t.Logf("retained users=%q", []string(refs))
			t.Logf("resolved users=%q", []string(base))

			var named []string
			for _, ref := range refs {
				for _, part := range strings.Split(ref, dynamic.SecretURNPrefix)[1:] {
					part = strings.TrimSuffix(part, ":")
					name, key, ok := strings.Cut(part, ":")
					require.True(t, ok)
					require.Equal(t, "creds", name)
					named = append(named, key)
				}
			}
			require.NotEmpty(t, named)
			for _, key := range named {
				_, exists := c.data[key]
				mutated := maps.Clone(c.data)
				mutated[key] = append([]byte("x"), c.data[key]...)
				after := verifyC6Users(verifyC6Load(t, c.spec, c.typ, mutated).ResolveSecretValues())
				t.Logf("named key %q: exists in Secret.Data=%v, value contributes (resolved users change when only it changes)=%v", key, exists, !assert.ObjectsAreEqual(base, after))
				assert.True(t, exists)
				assert.NotEqual(t, base, after)
			}
			assert.ElementsMatch(t, sortedKeys(c.data), named)
		})
	}
}

func sortedKeys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
