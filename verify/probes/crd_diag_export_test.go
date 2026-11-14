// Run (on a worktree of evalon/traefik-se-df0ea15d): cp verify/probes/crd_diag_*_test.go <worktree>/pkg/provider/kubernetes/crd/ && cd <worktree> && go test -race -count=1 -v -run '^TestVerifyCRDDiag' ./pkg/provider/kubernetes/crd
package crd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// VerifyPublish runs the same three statements Provider.Provide runs on this branch before hashing and emitting a configuration.
func VerifyPublish(t *testing.T, mw *traefikv1alpha1.Middleware, secret *corev1.Secret) *dynamic.Configuration {
	t.Helper()
	client := newClientImpl(kubefake.NewClientset(secret), traefikcrdfake.NewClientset(mw))
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	p := Provider{}
	require.NoError(t, p.Init())
	conf, secrets := p.loadConfigurationAndSecrets(t.Context(), client)
	dynamic.Secrets.Replace(ProviderName, secrets)
	conf.SecretsDigest = secrets.Digest()
	return conf
}
