//go:build verify

// Run: cp verify/repro/c1_gatewayclass_supported_features_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -race -count=1 -run '^TestVerifyC1_' -v ./pkg/provider/kubernetes/gateway

package gateway

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
	gatefake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func verifyC1Class(t *testing.T, stored gatev1.GatewayClassStatus) (*clientWrapper, *gatefake.Clientset) {
	t.Helper()
	gc := &gatev1.GatewayClass{
		ObjectMeta: metav1.ObjectMeta{Name: "my-gateway-class", Generation: 1},
		Spec:       gatev1.GatewayClassSpec{ControllerName: controllerName},
		Status:     stored,
	}
	gwClient := gatefake.NewSimpleClientset(gc)
	client := newClientImpl(kubefake.NewClientset(), gwClient)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	eventCh, err := client.WatchAll(nil, stop)
	require.NoError(t, err)
	select {
	case <-eventCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for informer sync")
	}
	return client, gwClient
}

func verifyC1Names(fs []gatev1.SupportedFeature) []string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Name))
	}
	return out
}

func verifyC1Accepted(message string, when time.Time) metav1.Condition {
	return metav1.Condition{
		Type:               string(gatev1.GatewayClassConditionStatusAccepted),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: 1,
		Reason:             "Handled",
		Message:            message,
		LastTransitionTime: metav1.NewTime(when),
	}
}

// Direct call: same condition fields (only LastTransitionTime differs), different SupportedFeatures.
func TestVerifyC1_UpdateGatewayClassStatus_FeaturesOnlyChange(t *testing.T) {
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	stored := gatev1.GatewayClassStatus{
		Conditions:        []metav1.Condition{verifyC1Accepted("Handled by Traefik controller", old)},
		SupportedFeatures: []gatev1.SupportedFeature{{Name: "GatewayPort8080"}},
	}
	client, gwClient := verifyC1Class(t, stored)

	requested := gatev1.GatewayClassStatus{
		Conditions:        []metav1.Condition{verifyC1Accepted("Handled by Traefik controller", time.Now())},
		SupportedFeatures: []gatev1.SupportedFeature{{Name: "GatewayHTTPListenerIsolation"}, {Name: "GatewayHTTPSListenerDetectMisdirectedRequests"}, {Name: "GatewayPort8080"}},
	}
	require.NoError(t, client.UpdateGatewayClassStatus(t.Context(), "my-gateway-class", requested))

	got, err := gwClient.GatewayV1().GatewayClasses().Get(t.Context(), "my-gateway-class", metav1.GetOptions{})
	require.NoError(t, err)
	t.Logf("stored SupportedFeatures after UpdateGatewayClassStatus: %v", verifyC1Names(got.Status.SupportedFeatures))
	assert.Equal(t, verifyC1Names(requested.SupportedFeatures), verifyC1Names(got.Status.SupportedFeatures), "requested feature list should be persisted")
}

// Control: a changed condition Message triggers the write, and features come along.
func TestVerifyC1_UpdateGatewayClassStatus_ConditionChangeWritesFeatures(t *testing.T) {
	old := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	stored := gatev1.GatewayClassStatus{
		Conditions:        []metav1.Condition{verifyC1Accepted("old message", old)},
		SupportedFeatures: []gatev1.SupportedFeature{{Name: "GatewayPort8080"}},
	}
	client, gwClient := verifyC1Class(t, stored)

	requested := gatev1.GatewayClassStatus{
		Conditions:        []metav1.Condition{verifyC1Accepted("Handled by Traefik controller", time.Now())},
		SupportedFeatures: []gatev1.SupportedFeature{{Name: "GatewayHTTPListenerIsolation"}, {Name: "GatewayPort8080"}},
	}
	require.NoError(t, client.UpdateGatewayClassStatus(t.Context(), "my-gateway-class", requested))

	got, err := gwClient.GatewayV1().GatewayClasses().Get(t.Context(), "my-gateway-class", metav1.GetOptions{})
	require.NoError(t, err)
	t.Logf("stored SupportedFeatures after UpdateGatewayClassStatus: %v", verifyC1Names(got.Status.SupportedFeatures))
	assert.Equal(t, verifyC1Names(requested.SupportedFeatures), verifyC1Names(got.Status.SupportedFeatures))
}

// Upgrade path through the provider: the GatewayClass already carries the Accepted condition written by the
// pre-upgrade controller (same generation), plus the pre-upgrade feature list (without the two new features).
func TestVerifyC1_ProviderUpgrade_NewFeaturesNotAdvertised(t *testing.T) {
	var preUpgrade []gatev1.SupportedFeature
	for _, f := range SupportedFeatures() {
		if f == features.GatewayHTTPListenerIsolationFeature.Name || f == features.GatewayHTTPSListenerDetectMisdirectedRequestsFeature.Name {
			continue
		}
		preUpgrade = append(preUpgrade, gatev1.SupportedFeature{Name: gatev1.FeatureName(f)})
	}
	slices.SortFunc(preUpgrade, func(a, b gatev1.SupportedFeature) int { return strings.Compare(string(a.Name), string(b.Name)) })

	stored := gatev1.GatewayClassStatus{
		Conditions:        []metav1.Condition{verifyC1Accepted("Handled by Traefik controller", time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))},
		SupportedFeatures: preUpgrade,
	}
	client, gwClient := verifyC1Class(t, stored)

	p := Provider{EntryPoints: map[string]Entrypoint{"web": {Address: ":80"}}, client: client}
	_, report, err := p.loadConfigurationFromGateways(t.Context())
	require.NoError(t, err)
	report.Flush(t.Context(), client)

	got, err := gwClient.GatewayV1().GatewayClasses().Get(t.Context(), "my-gateway-class", metav1.GetOptions{})
	require.NoError(t, err)
	names := verifyC1Names(got.Status.SupportedFeatures)
	t.Logf("stored SupportedFeatures after upgrade reconcile (%d entries): %v", len(names), names)
	assert.Contains(t, names, "GatewayHTTPListenerIsolation")
	assert.Contains(t, names, "GatewayHTTPSListenerDetectMisdirectedRequests")
}
