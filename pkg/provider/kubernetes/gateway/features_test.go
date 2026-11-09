package gateway

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func TestSupportedFeatures_listenerIsolation(t *testing.T) {
	supportedFeatures := SupportedFeatures()

	assert.Contains(t, supportedFeatures, features.SupportGatewayHTTPListenerIsolation)
	assert.Contains(t, supportedFeatures, features.FeatureName(features.SupportGatewayHTTPSListenerDetectMisdirectedRequests))
}
