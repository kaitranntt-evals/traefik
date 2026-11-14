// Run (on a worktree of evalon/traefik-se-21bb188b): cp verify/repro/c10_*_test.go <worktree>/pkg/provider/kubernetes/crd/ && cd <worktree> && go test -count=1 -v -run '^TestVerifyC10' ./pkg/provider/kubernetes/crd
package crd

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// c10RotatingClient serves version A of the Secret for the first switchAfter reads, and version B afterwards.
type c10RotatingClient struct {
	Client
	mu          sync.Mutex
	reads       int
	switchAfter int
	a, b        *corev1.Secret
}

func (c *c10RotatingClient) GetSecret(namespace, name string) (*corev1.Secret, bool, error) {
	if namespace != "default" || name != "authsecret" {
		return c.Client.GetSecret(namespace, name)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.reads++
	if c.reads > c.switchAfter {
		return c.b.DeepCopy(), true, nil
	}
	return c.a.DeepCopy(), true, nil
}

// C10Load runs one loadConfigurationFromCRD while the Secret flips from a to b after switchAfter reads, and returns the read count.
func C10Load(t *testing.T, mw *traefikv1alpha1.Middleware, a, b *corev1.Secret, switchAfter int) (*dynamic.Configuration, int) {
	t.Helper()
	client := newClientImpl(kubefake.NewClientset(a.DeepCopy()), traefikcrdfake.NewClientset(mw.DeepCopy()))
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	rot := &c10RotatingClient{Client: client, switchAfter: switchAfter, a: a, b: b}
	p := Provider{}
	require.NoError(t, p.Init())
	conf := p.loadConfigurationFromCRD(t.Context(), rot)
	return conf, rot.reads
}
