// Run: cd <worktree of evalon/traefik-se-9568168b> && cp <verify-branch>/verify/repro/c8_typed_basicauth_ref_key_test.go pkg/provider/kubernetes/crd/ && go test -count=1 -run TestVerifyC8 -v ./pkg/provider/kubernetes/crd
package crd

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestVerifyC8_TypedBasicAuthRefKey(t *testing.T) {
	typed := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "typed", Namespace: "default"},
		Type:       corev1.SecretTypeBasicAuth,
		Data:       map[string][]byte{"username": []byte("alice"), "password": []byte("pw")},
	}
	htpasswd := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "htpasswd", Namespace: "default"},
		Data:       map[string][]byte{"users": []byte("a:x\nb:y\n")},
	}
	kubeClient := kubefake.NewClientset(typed, htpasswd)
	client := newClientImpl(kubeClient, traefikcrdfake.NewClientset())
	stopCh := make(chan struct{})
	defer close(stopCh)
	eventCh, err := client.WatchAll([]string{"default"}, stopCh)
	require.NoError(t, err)
	<-eventCh

	for _, name := range []string{"typed", "htpasswd"} {
		secrets := map[string]dynamic.Secret{}
		ba, err := createBasicAuthMiddleware(client, "default", &traefikv1alpha1.BasicAuth{Secret: name}, secrets)
		require.NoError(t, err)
		sec, _, _ := client.GetSecret("default", name)
		keys := make([]string, 0)
		for k := range sec.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		t.Logf("secret %q data keys=%v -> emitted refs=%v", name, keys, ba.Users)
	}
}
