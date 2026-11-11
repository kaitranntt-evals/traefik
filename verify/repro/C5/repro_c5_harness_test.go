// Run: sh verify/repro/C5_secretvalue_nested_plugin_input.sh (it copies this file into pkg/provider/kubernetes/crd/ of evalon/traefik-se-2af317a2).
package crd

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// LoadReproConfiguration runs the production CRD loader (loadConfigurationFromCRD) against fake clientsets.
func LoadReproConfiguration(t *testing.T, k8sObjects, crdObjects []runtime.Object) *dynamic.Configuration {
	t.Helper()

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), traefikcrdfake.NewSimpleClientset(crdObjects...))

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	p := &Provider{}
	require.NoError(t, p.Init())

	return p.loadConfigurationFromCRD(t.Context(), client)
}
