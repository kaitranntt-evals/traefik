//go:build verify

// Run via verify/repro/c8_order_mutation.sh (prints the generated auth-map users order for a three-key Secret).
package ingressnginx

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestVerifyC8(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "credentials", Namespace: "default"},
		Data:       map[string][]byte{"alice": []byte("hash-a"), "bob": []byte("hash-b"), "carol": []byte("hash-c")},
	}
	client := newClient(kubefake.NewClientset(secret))
	events, err := client.WatchAll(t.Context(), "", "")
	require.NoError(t, err)
	<-events
	provider := Provider{k8sClient: client}
	authType, secretType, name := "basic", "auth-map", "credentials"

	orders := map[string]int{}
	for range 200 {
		basic, _, _, err := provider.resolveBasicAuth("default", IngressConfig{AuthType: &authType, AuthSecret: &name, AuthSecretType: &secretType})
		require.NoError(t, err)
		orders[fmt.Sprint(basic.Users)]++
	}
	fmt.Printf("C8 distinct user orders over 200 loads: %d\n", len(orders))
	for order, n := range orders {
		fmt.Printf("C8   %3dx %s\n", n, order)
	}
}
