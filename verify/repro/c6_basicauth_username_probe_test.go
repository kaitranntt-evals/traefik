//go:build ignore

// Run: in a checkout of evalon/traefik-se-0b0162c2: sed '/^\/\/go:build ignore$/d' verify/repro/c6_basicauth_username_probe_test.go > pkg/provider/kubernetes/crd/c6_basicauth_username_probe_test.go && go test -count=1 -v -run 'TestProbe_C6' ./pkg/provider/kubernetes/crd
package crd

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestProbe_C6_BasicAuthTypedSecretUsername(t *testing.T) {
	sec := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "typed-ba", Namespace: "default"},
		Type:       corev1.SecretTypeBasicAuth,
		Data:       map[string][]byte{"username": []byte("DISTINCT_USERNAME_zq"), "password": []byte("DISTINCT_PASSWORD_zq")},
	}
	kubeClient := kubefake.NewClientset(sec)
	client := newClientImpl(kubeClient, traefikcrdfake.NewClientset())
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	secrets := dynamic.Secrets{}
	ba, err := createBasicAuthMiddleware(client, "default", &traefikv1alpha1.BasicAuth{Secret: "typed-ba"}, secrets)
	require.NoError(t, err)
	js, _ := json.Marshal(&dynamic.Middleware{BasicAuth: ba})
	t.Logf("BasicAuth.Users = %q", ba.Users)
	t.Logf("public middleware JSON = %s", js)
	t.Logf("secret store keys = %v", func() []string { var k []string; for r := range secrets { k = append(k, r) }; return k }())
}
