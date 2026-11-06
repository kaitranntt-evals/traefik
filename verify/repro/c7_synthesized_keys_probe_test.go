//go:build ignore

// Run: in a checkout of evalon/traefik-se-866b6773: sed '/^\/\/go:build ignore$/d' verify/repro/c7_synthesized_keys_probe_test.go > pkg/provider/kubernetes/crd/c7_synthesized_keys_probe_test.go && go test -count=1 -v -run 'TestProbe_C7' ./pkg/provider/kubernetes/crd
package crd

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestProbe_C7_ReferenceKeysVsSecretKeys(t *testing.T) {
	opaque := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "opaque-auth", Namespace: "default"},
		Data:       map[string][]byte{"users": []byte("test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\ntest2:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0\n")},
	}
	typed := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "typed-ba", Namespace: "default"},
		Type:       corev1.SecretTypeBasicAuth,
		Data:       map[string][]byte{"username": []byte("alice"), "password": []byte("pw")},
	}
	digest := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "digest-auth", Namespace: "default"},
		Data:       map[string][]byte{"users": []byte("test:traefik:a2688e031edb4be6a3797f3882655c05\ntest2:traefik:518845800f9e2bfb1f1f740ec24f074e\n")},
	}
	kubeClient := kubefake.NewClientset(opaque, typed, digest)
	client := newClientImpl(kubeClient, traefikcrdfake.NewClientset())
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	report := func(secret *corev1.Secret, users []string) {
		keys := slices.Sorted(func(yield func(string) bool) { for k := range secret.Data { if !yield(k) { return } } })
		for _, u := range users {
			parts := strings.Split(u, ":")
			refKey := parts[len(parts)-1]
			t.Logf("secret=%s dataKeys=%v ref=%q refKey=%q existsInSecret=%v", secret.Name, keys, u, refKey, slices.Contains(keys, refKey))
		}
	}

	secrets := map[string]string{}
	ba, err := createBasicAuthMiddleware(client, "default", &traefikv1alpha1.BasicAuth{Secret: "opaque-auth"}, secrets)
	require.NoError(t, err)
	report(opaque, ba.Users)
	ba2, err := createBasicAuthMiddleware(client, "default", &traefikv1alpha1.BasicAuth{Secret: "typed-ba"}, secrets)
	require.NoError(t, err)
	report(typed, ba2.Users)
	da, err := createDigestAuthMiddleware(client, "default", &traefikv1alpha1.DigestAuth{Secret: "digest-auth"}, secrets)
	require.NoError(t, err)
	report(digest, da.Users)
}
