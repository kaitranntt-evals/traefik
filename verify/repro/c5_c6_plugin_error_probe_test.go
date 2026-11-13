//go:build verify

// Run (branches evalon/traefik-se-6b2a0d3d for C5 and evalon/traefik-se-64dd1e37 for C6, eval fixtures provisioned in pkg/provider/kubernetes/crd/):
//   mkdir -p <worktree>/pkg/provider/kubernetes/crd/plugins-local/src/verifyerr
//   cp verify/repro/verifyerr/verifyerr.go verify/repro/verifyerr/.traefik.yml <worktree>/pkg/provider/kubernetes/crd/plugins-local/src/verifyerr/
//   cp verify/repro/c5_c6_plugin_error_probe_test.go <worktree>/pkg/provider/kubernetes/crd/zz_verify_c5c6_test.go
//   go test -tags verify -count=1 -run TestVerifyPluginErrors -v ./pkg/provider/kubernetes/crd/
package crd_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const verifySecret = "verify_s3cret_VALUE_zq7"

// verifyChain walks err with the host errors.Unwrap (single and multi) and returns every node as "type: text".
func verifyChain(err error) []string {
	var out []string
	pending := []error{err}
	for len(pending) > 0 && len(out) < 32 {
		e := pending[0]
		pending = pending[1:]
		if e == nil {
			continue
		}
		out = append(out, fmt.Sprintf("%T: %q", e, e.Error()))
		if u := errors.Unwrap(e); u != nil {
			pending = append(pending, u)
		}
		if m, ok := e.(interface{ Unwrap() []error }); ok {
			pending = append(pending, m.Unwrap()...)
		}
	}
	return out
}

func verifyLeaks(texts []string) bool {
	for _, s := range texts {
		if strings.Contains(s, verifySecret) {
			return true
		}
	}
	return false
}

func TestVerifyPluginErrors(t *testing.T) {
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	for _, mode := range []string{"interpreted-unwrap", "native-wrap", "native-join", "cached", "cached-always"} {
		newBuilder := func() *plugins.Builder {
			// Fresh production plugins.Builder, i.e. a fresh Yaegi interpreter without plugin state.
			b, err := plugins.NewBuilder(nil, nil, map[string]plugins.LocalDescriptor{"test-secret": {ModuleName: "verifyerr"}})
			require.NoError(t, err)
			return b
		}
		fmt.Printf("=== mode=%s\n", mode)

		// (a) The original plugin error, as the host sees it, without the production middleware builder.
		ctor, err := newBuilder().Build("test-secret", map[string]any{"mode": mode, "user": "admin", "secret": verifySecret}, "probe")
		require.NoError(t, err)
		_, origErr := ctor(t.Context(), next)
		require.Error(t, origErr)
		orig := verifyChain(origErr)
		fmt.Printf("  original error: top=%q\n", origErr.Error())
		for i, n := range orig {
			fmt.Printf("    host chain[%d] %s\n", i, n)
		}
		fmt.Printf("  original: credentialInTopMessage=%v credentialReachableViaHostUnwrap=%v\n",
			strings.Contains(origErr.Error(), verifySecret), verifyLeaks(orig[1:]))

		// (b) The same plugin through the CRD provider and the production middleware builder, two builds.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "name", Namespace: "default"},
			Data:       map[string][]byte{"key": []byte(verifySecret)},
		}
		conf, provider := crd.LoadConfigurationWithPluginMiddlewareForTest(t, "test-secret", "test-secret",
			map[string]any{"mode": mode, "user": "admin", "secret": "urn:k8s:secret:name:key"}, secret)
		builder := newBuilder()
		for i := range 2 {
			_, rtConf, buildErr := evalBuildHandler(t, conf, provider, builder, "default-test-secret")
			require.Error(t, buildErr)
			chain := verifyChain(buildErr)
			stored := evalMiddlewareInfo(rtConf, "default-test-secret").Err
			fmt.Printf("  production build#%d returned=%q\n", i+1, buildErr.Error())
			for j, n := range chain {
				fmt.Printf("    host chain[%d] %s\n", j, n)
			}
			fmt.Printf("    stored MiddlewareInfo.Err=%q\n", stored)
			fmt.Printf("    LEAK returnedMessage=%v reachableCause=%v storedDiagnostic=%v\n",
				strings.Contains(buildErr.Error(), verifySecret), verifyLeaks(chain[1:]), verifyLeaks(stored))
		}
	}
}
