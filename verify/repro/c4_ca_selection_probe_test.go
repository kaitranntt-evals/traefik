//go:build ignore

// Run: in a checkout of evalon/traefik-se-e33f7395: sed '/^\/\/go:build ignore$/d' verify/repro/c4_ca_selection_probe_test.go > pkg/provider/kubernetes/crd/c4_ca_selection_probe_test.go && go test -count=1 -v -run 'TestProbe_C4' ./pkg/provider/kubernetes/crd
package crd

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
)

// TestProbe_C4 shows that getCABlocks and loadCASecretWithKey each implement the tls.ca-before-ca.crt selection.
func TestProbe_C4_CASelectionImplementedTwice(t *testing.T) {
	both := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "both", Namespace: "default"},
		Data:       map[string][]byte{"tls.ca": []byte("FROM-TLS-CA"), "ca.crt": []byte("FROM-CA-CRT")},
	}
	onlyCrt := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "onlycrt", Namespace: "default"},
		Data:       map[string][]byte{"ca.crt": []byte("FROM-CA-CRT")},
	}
	kubeClient := kubefake.NewClientset(both, onlyCrt)
	client := newClientImpl(kubeClient, traefikcrdfake.NewClientset())
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	for _, s := range []*corev1.Secret{both, onlyCrt} {
		v1, err1 := getCABlocks(s, s.Namespace, s.Name)
		k2, v2, err2 := loadCASecretWithKey(s.Namespace, s.Name, client)
		t.Logf("secret=%s getCABlocks -> %q err=%v | loadCASecretWithKey -> key=%q value=%q err=%v", s.Name, v1, err1, k2, v2, err2)
	}
}
