// Audit instrumentation (not production code). Copy into pkg/provider/kubernetes/ingress-nginx/ of branch
// evalon/traefik-se-fc2341ab together with the byte-exact eval fixtures, then run:
//   go test -v -count=1 -run '^TestVerify_C3' ./pkg/provider/kubernetes/ingress-nginx
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
)

func TestVerify_C3_AuthMapKeysSelectedTwice(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-auth", Namespace: "default"},
		Data:       map[string][]byte{"zoe": []byte("{SHA}zzz"), "adam": []byte("{SHA}aaa"), "mia": []byte("{SHA}mmm")},
	}

	// Selection 1: the loader enumerates the Secret keys (map iteration order).
	loaderOrders := map[string]struct{}{}
	for range 50 {
		users, err := basicAuthUsers(secret, "auth-map")
		require.NoError(t, err)
		loaderOrders[strings.Join(users, " ")] = struct{}{}
	}
	t.Logf("basicAuthUsers returned %d distinct orderings over 50 calls, e.g.:", len(loaderOrders))
	for order := range loaderOrders {
		t.Logf("  loader selection: %s", order)
	}

	// Selection 2: resolveBasicAuth builds the URNs.
	conf, _ := LoadIngressesForEvalTest(t, []string{"services.yml", "ingressclasses.yml", "ingresses/ingress-with-basicauth-map.yml"}, secret)
	for name, mw := range conf.HTTP.Middlewares {
		if mw.BasicAuth != nil {
			t.Logf("resolveBasicAuth users for %s: %q", name, mw.BasicAuth.Users)
		}
	}

	// Where each selection comes from in the delivered source.
	src, err := os.ReadFile("build.go")
	require.NoError(t, err)
	start := strings.Index(string(src), "func (p *Provider) resolveBasicAuth(")
	end := start + strings.Index(string(src[start:]), "\n}\n")
	body := string(src[start:end])
	lineOf := func(off int) int { return 1 + strings.Count(string(src[:start+off]), "\n") }

	call := regexp.MustCompile(`(?m)^.*basicAuthUsers\(secret, authSecretType\).*$`).FindStringIndex(body)
	loop := regexp.MustCompile(`(?m)^.*range slices\.Sorted\(maps\.Keys\(secret\.Data\)\).*$`).FindStringIndex(body)
	require.NotNil(t, call)
	require.NotNil(t, loop)
	t.Logf("build.go:%d: %s", lineOf(call[0]), strings.TrimSpace(body[call[0]:call[1]]))
	t.Logf("build.go:%d: %s", lineOf(loop[0]), strings.TrimSpace(body[loop[0]:loop[1]]))

	// The loader result is discarded, and the keys are enumerated a second time inside resolveBasicAuth.
	assert.Contains(t, body[call[0]:call[1]], "if _, err :=")
	assert.Less(t, call[0], loop[0])
}
