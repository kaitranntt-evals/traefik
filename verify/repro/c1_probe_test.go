//go:build verify

// Run: copy into <worktree a2edef14>/pkg/provider/kubernetes/crd/ then `go test -tags verify -count=1 -run TestVerifyC1 -v ./pkg/provider/kubernetes/crd/` (c1_deep_plugin_mutation.sh does this).
package crd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	kubefake "k8s.io/client-go/kubernetes/fake"

	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
)

// Prints the Secret values the provider bound for each plugin fixture shape.
func TestVerifyC1(t *testing.T) {
	for _, f := range []string{"with_plugin_read_secret.yml", "with_plugin_deep_read_secret.yml", "with_plugin_read_array_of_secret.yml", "with_plugin_read_array_of_map_contain_secret.yml"} {
		k8sObjects, crdObjects := readResources(t, []string{"services.yml", f})
		client := newClientImpl(kubefake.NewClientset(k8sObjects...), traefikcrdfake.NewClientset(crdObjects...))
		stop := make(chan struct{})
		events, err := client.WatchAll(nil, stop)
		require.NoError(t, err)
		<-events
		p := Provider{}
		require.NoError(t, p.Init())
		conf := p.loadConfigurationFromCRD(t.Context(), client)
		for name, m := range conf.HTTP.Middlewares {
			sv := m.SecretValues()
			for _, ref := range sv.References() {
				v, _ := sv.Value(ref)
				fmt.Printf("BOUND %s %s %s = %q\n", f, name, ref, v)
			}
		}
		close(stop)
	}
}
