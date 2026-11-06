//go:build ignore

// Run: in a checkout of evalon/traefik-se-dfb43954: sed '/^\/\/go:build ignore$/d' verify/repro/c10_provider_normalization_probe_test.go > pkg/provider/kubernetes/ingress-nginx/c10_provider_normalization_probe_test.go && go test -count=1 -v -run 'TestProbe_C10' ./pkg/provider/kubernetes/ingress-nginx
package ingressnginx

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProbe_C10_ProviderNormalization(t *testing.T) {
	for _, content := range []string{"  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  ", "  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  \n"} {
		sec := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "ba", Namespace: "default"}, Data: map[string][]byte{"auth": []byte(content)}}
		users, secrets, err := basicAuthUsers(sec, "auth-file")
		require.NoError(t, err)
		t.Logf("auth file %q -> users=%q stored=%q", content, users, secrets[users[0]])
	}
}
