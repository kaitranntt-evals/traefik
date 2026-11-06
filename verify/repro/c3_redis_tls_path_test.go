// Run (on branch evalon/traefik-se-462ea24b, with the eval fixtures export_eval_test.go + eval_provider_to_builder_runtime_test.go in place):
//   cp verify/repro/c3_redis_tls_path_test.go pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -v -run '^TestVerify_C3_' ./pkg/provider/kubernetes/crd

//go:build verify

package crd_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	"github.com/traefik/traefik/v3/pkg/types"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The Secret tls.crt/tls.key hold a path that exists but cannot be read as a file (a directory),
// so the TLS setup fails with a filesystem error that quotes the path.
func TestVerify_C3_RedisTLSFilesystemErrorPath(t *testing.T) {
	dir := t.TempDir() + "/secret-resolved-redis-tls-path"
	require.NoError(t, mkdir(dir))

	// Part 1: local constructor error.
	_, err := (&types.ClientTLS{Cert: dir, Key: dir}).CreateTLSConfig(t.Context())
	require.Error(t, err)
	fmt.Printf("[C3 part1] CreateTLSConfig error: %s\n", err)
	fmt.Printf("[C3 part1] contains resolved path: %v\n", strings.Contains(err.Error(), dir))

	// Part 2: middleware construction through the CRD provider and the middleware builder.
	// The fixture CA is not a parseable certificate, and CreateTLSConfig parses the CA before the key pair.
	conf, provider := crd.LoadConfigurationWithCustomSecretsForTest(t, []string{"services.yml", "with_ratelimit.yml"}, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tlssecret", Namespace: "default"},
		Data:       map[string][]byte{"tls.crt": []byte(dir), "tls.key": []byte(dir)},
	}, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "casecret", Namespace: "default"},
		Data:       map[string][]byte{"ca": selfSignedCA(t)},
	})
	require.NotNil(t, conf)
	for name, mid := range conf.HTTP.Middlewares {
		if mid.RateLimit == nil || mid.RateLimit.Redis == nil || mid.RateLimit.Redis.TLS == nil {
			continue
		}
		fmt.Printf("[C3 part2] %s dynamic tls.cert=%q tls.key=%q\n", name, mid.RateLimit.Redis.TLS.Cert, mid.RateLimit.Redis.TLS.Key)
		_, rtConf, err := evalBuildHandler(t, conf, provider, nil, name)
		require.Error(t, err)
		info := evalMiddlewareInfo(rtConf, name)
		require.NotNil(t, info)
		for _, txt := range evalErrorTexts(err) {
			fmt.Printf("[C3 part2] %s returned err text: %s\n", name, txt)
		}
		for _, e := range info.Err {
			fmt.Printf("[C3 part2] %s stored info.Err: %s\n", name, e)
		}
		all := strings.Join(append(evalErrorTexts(err), info.Err...), " ")
		fmt.Printf("[C3 part2] %s diagnostics contain resolved path: %v\n", name, strings.Contains(all, dir))
	}
}
