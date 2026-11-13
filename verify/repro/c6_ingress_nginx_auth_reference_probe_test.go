//go:build verify

// Run (on evalon/traefik-se-12ccb497): cp this file to pkg/provider/kubernetes/ingress-nginx/ && go test -tags verify -count=1 -v -run '^TestVerify_C6' ./pkg/provider/kubernetes/ingress-nginx
package ingressnginx

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func verifyC6Resolve(t *testing.T, authType, secretType string, data map[string][]byte) (dynamic.Users, dynamic.Users) {
	t.Helper()
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"}, Data: data}
	client := newClient(kubefake.NewClientset(secret))
	eventCh, err := client.WatchAll(t.Context(), "", "")
	require.NoError(t, err)
	<-eventCh
	p := Provider{k8sClient: client}
	basic, digest, values, err := p.resolveBasicAuth("default", IngressConfig{AuthType: ptr.To(authType), AuthSecret: ptr.To("auth"), AuthSecretType: ptr.To(secretType)})
	require.NoError(t, err)
	m := &dynamic.Middleware{BasicAuth: basic, DigestAuth: digest, SecretValues: values}
	if digest != nil {
		return digest.Users, m.ResolveSecretValues().DigestAuth.Users
	}
	return basic.Users, m.ResolveSecretValues().BasicAuth.Users
}

func TestVerify_C6_IngressNginxReferencesNameContributingKeys(t *testing.T) {
	cases := []struct {
		desc, authType, secretType string
		data                       map[string][]byte
		read                       []string
	}{
		{"basic auth-file with an unread sibling key", "basic", "auth-file", map[string][]byte{"auth": []byte("user:{SHA}W6ph5Mm5Pz8GgiULbPgzG37mj9g=\n"), "unrelated": []byte("zzz")}, []string{"auth"}},
		{"basic auth-map", "basic", "auth-map", map[string][]byte{"user": []byte("{SHA}W6ph5Mm5Pz8GgiULbPgzG37mj9g="), "user2": []byte("$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0")}, []string{"user", "user2"}},
		{"digest auth-file", "digest", "auth-file", map[string][]byte{"auth": []byte("user:traefik:a2688e031edb4be6a3797f3882655c05\n")}, []string{"auth"}},
	}
	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			refs, base := verifyC6Resolve(t, c.authType, c.secretType, c.data)
			t.Logf("Secret.Data keys=%v retained users=%q resolved users=%q", slices.Sorted(maps.Keys(c.data)), []string(refs), []string(base))
			var named []string
			for _, ref := range refs {
				key := strings.TrimPrefix(ref, dynamic.SecretURNPrefix+"auth:")
				require.NotEqual(t, ref, key)
				named = append(named, key)
				_, exists := c.data[key]
				mutated := maps.Clone(c.data)
				mutated[key] = append([]byte("x"), c.data[key]...)
				_, after := verifyC6Resolve(t, c.authType, c.secretType, mutated)
				t.Logf("named key %q: exists in Secret.Data=%v, value contributes=%v", key, exists, !assert.ObjectsAreEqual(base, after))
				assert.True(t, exists)
				assert.NotEqual(t, base, after)
			}
			assert.ElementsMatch(t, c.read, named)
		})
	}
}
