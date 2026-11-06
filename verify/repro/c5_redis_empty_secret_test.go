// Run (on branch evalon/traefik-se-fd1b4d78, with the eval fixture export_eval_test.go in place):
//   cp verify/repro/c5_redis_empty_secret_test.go pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -v -run '^TestVerify_C5_' ./pkg/provider/kubernetes/crd

//go:build verify

package crd_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Rotate the Redis credential Secret: username present but empty -> nonempty -> empty again.
func TestVerify_C5_RedisEmptyCredentialReference(t *testing.T) {
	for _, step := range []struct{ desc, username, password string }{
		{"empty username / empty password", "", ""},
		{"nonempty username / nonempty password", "redis-user", "redis-pass"},
		{"empty username / empty password again", "", ""},
	} {
		conf, _ := crd.LoadConfigurationWithCustomSecretsForTest(t, []string{"services.yml", "with_ratelimit.yml"}, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "redissecret", Namespace: "default"},
			Data:       map[string][]byte{"username": []byte(step.username), "password": []byte(step.password)},
		})
		require.NotNil(t, conf)
		for name, mid := range conf.HTTP.Middlewares {
			if mid.RateLimit == nil || mid.RateLimit.Redis == nil {
				continue
			}
			hash, err := crd.ProviderChangeHash(nil, conf)
			require.NoError(t, err)
			fmt.Printf("[C5 %s] %s Username=%q Password=%q hash=%v\n", step.desc, name, mid.RateLimit.Redis.Username, mid.RateLimit.Redis.Password, hash)
		}
	}
}
