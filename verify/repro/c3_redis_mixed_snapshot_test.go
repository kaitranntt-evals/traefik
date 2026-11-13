//go:build verify

// Run (branch evalon/traefik-se-70a4758f):
//   cp verify/repro/c3_redis_mixed_snapshot_test.go <worktree>/pkg/provider/kubernetes/crd/zz_verify_c3_test.go
//   go test -tags verify -count=1 -run TestVerifyC3 -v ./pkg/provider/kubernetes/crd/
package crd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
)

// verifyRotatingClient serves Secret snapshot A for the first `after` reads of each rotated Secret, then snapshot B
// (one rotation landing in the informer cache while a single loadConfigurationFromCRD pass is running).
type verifyRotatingClient struct {
	Client
	after int
	reads map[string]int
	next  map[string]*corev1.Secret
	trace []string
}

func (c *verifyRotatingClient) GetSecret(namespace, name string) (*corev1.Secret, bool, error) {
	secret, ok, err := c.Client.GetSecret(namespace, name)
	b, rotated := c.next[name]
	if !rotated || !ok || err != nil {
		return secret, ok, err
	}
	c.reads[name]++
	if c.reads[name] > c.after {
		c.trace = append(c.trace, fmt.Sprintf("%s#%d=B", name, c.reads[name]))
		return b, true, nil
	}
	c.trace = append(c.trace, fmt.Sprintf("%s#%d=A", name, c.reads[name]))
	return secret, ok, err
}

func TestVerifyC3(t *testing.T) {
	k8sObjects, crdObjects := readResources(t, []string{"services.yml", "with_ratelimit.yml"})
	base := newClientImpl(kubefake.NewClientset(k8sObjects...), traefikcrdfake.NewClientset(crdObjects...))
	stop := make(chan struct{})
	defer close(stop)
	events, err := base.WatchAll(nil, stop)
	require.NoError(t, err)
	<-events

	// Snapshot A is the fixture (user/password, cert A/key A); snapshot B is a complete, individually valid rotation.
	next := map[string]*corev1.Secret{
		"redissecret": {Data: map[string][]byte{"username": []byte("user-B"), "password": []byte("password-B")}},
		"tlssecret": {Data: map[string][]byte{
			"tls.crt": []byte("-----BEGIN CERTIFICATE-----\nB\n-----END CERTIFICATE-----"),
			"tls.key": []byte("-----BEGIN PRIVATE KEY-----\nB\n-----END PRIVATE KEY-----"),
		}},
	}
	for name, s := range next {
		s.Name, s.Namespace = name, "default"
	}

	for after := 0; after <= 3; after++ {
		client := &verifyRotatingClient{Client: base, after: after, reads: map[string]int{}, next: next}
		p := Provider{}
		require.NoError(t, p.Init())
		conf := p.loadConfigurationFromCRD(t.Context(), client)
		mw := conf.HTTP.Middlewares["default-ratelimit"]
		if mw == nil {
			fmt.Printf("C3 rotationAfterRead=%d reads=%v middleware NOT PUBLISHED\n", after, client.trace)
			continue
		}
		r := mw.RateLimit.Redis
		fmt.Printf("C3 rotationAfterRead=%d reads=%v\n", after, client.trace)
		fmt.Printf("C3   published %s=%q %s=%q\n", r.Username, mw.Secrets[r.Username].Value, r.Password, mw.Secrets[r.Password].Value)
		fmt.Printf("C3   published %s=%q %s=%q\n", r.TLS.Cert, mw.Secrets[r.TLS.Cert].Value, r.TLS.Key, mw.Secrets[r.TLS.Key].Value)
	}
}
