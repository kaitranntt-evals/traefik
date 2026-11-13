//go:build verify

// Run (on evalon/traefik-se-7af818be): cp this file to pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -v -run '^TestVerify_C5' ./pkg/provider/kubernetes/crd
package crd

import (
	"reflect"
	"testing"

	"github.com/mitchellh/hashstructure"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Prints the two comparisons production uses to decide a reload, for the same pair of
// configurations the branch's TestLoadConfigurationFromCRD_secretUpdate asserts NotEqual on:
// the provider's hashstructure.Hash gate (kubernetes.go:159) and the watcher's reflect.DeepEqual (configurationwatcher.go:136).
func TestVerify_C5_ProductionReloadComparisons(t *testing.T) {
	paths := []string{"services.yml", "with_plugin_read_secret.yml"}
	initial := loadConfigurationWithSecrets(t, paths, nil)
	unchanged := loadConfigurationWithSecrets(t, paths, nil)
	updated := loadConfigurationWithSecrets(t, paths, func(secret *corev1.Secret) { secret.Data["key"] = []byte("rotated") })

	h := func(v any) uint64 { x, err := hashstructure.Hash(v, nil); require.NoError(t, err); return x }
	t.Logf("unchanged Secret: provider hash equal=%v, watcher DeepEqual=%v", h(initial) == h(unchanged), reflect.DeepEqual(initial, unchanged))
	t.Logf("rotated Secret:   provider hash equal=%v, watcher DeepEqual=%v", h(initial) == h(updated), reflect.DeepEqual(initial, updated))
}
