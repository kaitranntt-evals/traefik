// Run: used by verify/repro/c12_mutation.sh as the sensitivity control; copied to pkg/provider/kubernetes/ingress-nginx/ of a worktree of evalon/traefik-se-6226f21f.
package ingressnginx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestVerifyC12_OrderProbe(t *testing.T) {
	data := map[string][]byte{"zed": []byte("h1"), "amy": []byte("h2"), "mia": []byte("h3"), "bob": []byte("h4"), "kim": []byte("h5")}
	client := newClient(kubefake.NewClientset(&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"}, Data: data}))
	events, err := client.WatchAll(t.Context(), "", "")
	require.NoError(t, err)
	<-events

	provider := Provider{k8sClient: client}
	basic, _, err := provider.resolveBasicAuth("default", IngressConfig{AuthType: ptr.To("basic"), AuthSecret: ptr.To("auth"), AuthSecretType: ptr.To("auth-map")})
	require.NoError(t, err)
	assert.Equal(t, dynamic.Users{"urn:k8s:secret:auth:amy", "urn:k8s:secret:auth:bob", "urn:k8s:secret:auth:kim", "urn:k8s:secret:auth:mia", "urn:k8s:secret:auth:zed"}, basic.Users)
}
