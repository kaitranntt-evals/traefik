// Run: sh verify/repro/C3_auth_map_keys_selected_twice.sh (it copies this file into pkg/provider/kubernetes/ingress-nginx/ of evalon/traefik-se-fc2341ab); PASS means the problem reproduced.
package ingressnginx

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

func TestRepro_C3_AuthMapKeysSelectedTwice(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-auth", Namespace: "default"},
		Data:       map[string][]byte{"zoe": []byte("{SHA}zzz"), "adam": []byte("{SHA}aaa"), "mia": []byte("{SHA}mmm")},
	}

	// Selection 1: the loader enumerates the Secret keys, in map iteration order.
	loaderOrders := map[string]struct{}{}
	for range 200 {
		users, err := basicAuthUsers(secret, "auth-map")
		require.NoError(t, err)
		loaderOrders[strings.Join(users, " ")] = struct{}{}
	}
	t.Logf("basicAuthUsers returned %d distinct orderings over 200 calls", len(loaderOrders))

	// Selection 2: resolveBasicAuth, called as the provider does for an Ingress with the auth annotations.
	client := newClient(kubefake.NewClientset(secret))
	eventCh, err := client.WatchAll(t.Context(), "", "")
	require.NoError(t, err)
	<-eventCh

	p := Provider{k8sClient: client}
	urnOrders := map[string]struct{}{}
	for range 200 {
		basicAuth, _, _, err := p.resolveBasicAuth("default", IngressConfig{
			AuthType: ptr.To("basic"), AuthSecret: ptr.To("basic-auth"), AuthSecretType: ptr.To("auth-map"),
		})
		require.NoError(t, err)
		urnOrders[strings.Join(basicAuth.Users, " ")] = struct{}{}
	}
	for order := range urnOrders {
		t.Logf("resolveBasicAuth users: %s", order)
	}

	// The URNs do not follow the loader's selection: they come from a second, sorted enumeration of the same keys.
	assert.Greater(t, len(loaderOrders), 1)
	assert.Len(t, urnOrders, 1)

	// Where each selection is made in the delivered source.
	src, err := os.ReadFile("build.go")
	require.NoError(t, err)
	start := strings.Index(string(src), "func (p *Provider) resolveBasicAuth(")
	require.GreaterOrEqual(t, start, 0)
	body := string(src[start : start+strings.Index(string(src[start:]), "\n}\n")])
	lineOf := func(off int) int { return 1 + strings.Count(string(src[:start+off]), "\n") }

	call := regexp.MustCompile(`(?m)^.*basicAuthUsers\(secret, authSecretType\).*$`).FindStringIndex(body)
	loop := regexp.MustCompile(`(?m)^.*range .*secret\.Data.*$`).FindStringIndex(body)
	require.NotNil(t, call)
	require.NotNil(t, loop)
	t.Logf("build.go:%d: %s", lineOf(call[0]), strings.TrimSpace(body[call[0]:call[1]]))
	t.Logf("build.go:%d: %s", lineOf(loop[0]), strings.TrimSpace(body[loop[0]:loop[1]]))

	// The loader's selection is discarded, then resolveBasicAuth enumerates secret.Data again only to build the URNs.
	assert.Contains(t, body[call[0]:call[1]], "if _, err :=")
	assert.Less(t, call[0], loop[0])
}
