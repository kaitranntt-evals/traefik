//go:build ignore

// Run: in a checkout of evalon/traefik-se-d6c9b55f: sed '/^\/\/go:build ignore$/d' verify/repro/c11_parsing_divergence_probe_test.go > pkg/provider/kubernetes/crd/c11_parsing_divergence_probe_test.go && go test -count=1 -v -run 'TestProbe_C11' ./pkg/provider/kubernetes/crd
package crd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// Feeds the same reference strings to ParseSecretReference and to getSecretValue to show whether they share parsing.
func TestProbe_C11_ParsingDivergence(t *testing.T) {
	sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "s", Namespace: "default"}, Data: map[string][]byte{"k": []byte("v")}}
	kubeClient := kubefake.NewClientset(sec)
	client := newClientImpl(kubeClient, traefikcrdfake.NewClientset())
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	for _, ref := range []string{"urn:k8s:secret:s:k", "urn:k8s:secret::k", "urn:k8s:secret:s:", "xxx:yyy:zzz:s:k", "urn:k8s:secret:s:k:extra"} {
		n, k, perr := dynamic.ParseSecretReference(ref)
		v, gerr := getSecretValue(client, "default", ref)
		t.Logf("ref=%-28q ParseSecretReference -> name=%q key=%q err=%v | getSecretValue -> value=%q err=%v", ref, n, k, perr, v, gerr)
	}
}
