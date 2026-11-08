//go:build verify

// Run: cp verify/repro/c3_shadowed_status_conditional_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -v -run 'TestVerifyC3' ./pkg/provider/kubernetes/gateway/
// (on branch evalon/traefik-ga-df753f72). Replays the hostname-intersection case of TestLoadHTTPRoutes_listenerIsolation
// with the "shadowed" HTTPRoute status record deleted before the assertions run.
package gateway

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
)

func TestVerifyC3_shadowedStatusRecordRemoved(t *testing.T) {
	expectedRoutes := map[string]map[string]string{
		"bar.com":             {"/empty-hostname": "attaches-to-empty-hostname"},
		"bar.example.com":     {"/wildcard-example-com": "attaches-to-wildcard-example-com"},
		"bar.foo.example.com": {"/wildcard-foo-example-com": "attaches-to-wildcard-foo-example-com"},
		"abc.foo.example.com": {"/abc-foo-example-com": "attaches-to-abc-foo-example-com"},
	}
	paths := []string{"/empty-hostname", "/wildcard-example-com", "/wildcard-foo-example-com", "/abc-foo-example-com", "/shadowed"}
	expectedRules := map[string]string{
		"attaches-to-empty-hostname":           `Host("bar.com") && (Path("/empty-hostname") || PathPrefix("/empty-hostname/"))`,
		"attaches-to-wildcard-example-com":     `Host("**.example.com") && !Host("**.foo.example.com") && (Path("/wildcard-example-com") || PathPrefix("/wildcard-example-com/"))`,
		"attaches-to-wildcard-foo-example-com": `Host("**.foo.example.com") && !Host("abc.foo.example.com") && (Path("/wildcard-foo-example-com") || PathPrefix("/wildcard-foo-example-com/"))`,
		"attaches-to-abc-foo-example-com":      `Host("abc.foo.example.com") && (Path("/abc-foo-example-com") || PathPrefix("/abc-foo-example-com/"))`,
	}

	for _, removeShadowed := range []bool{false, true} {
		name := "normal_output"
		if removeShadowed {
			name = "shadowed_status_record_removed"
		}
		t.Run(name, func(t *testing.T) {
			conf, statusReport := loadConfigurationFromFixtures(t, []string{"services.yml", "httproute/with_listener_isolation_hostname_intersection.yml"}, map[string]Entrypoint{"web": {Address: ":80"}})

			shadowedKey := ktypes.NamespacedName{Namespace: "default", Name: "shadowed"}
			_, present := statusReport.httpRoutes[shadowedKey]
			t.Logf("shadowed status record present before mutation: %v (httpRoutes keys: %d)", present, len(statusReport.httpRoutes))
			if removeShadowed {
				delete(statusReport.httpRoutes, shadowedKey)
				t.Logf("deleted shadowed status record; httpRoutes keys now: %d", len(statusReport.httpRoutes))
			}

			// --- verbatim copy of the authored assertions from TestLoadHTTPRoutes_listenerIsolation ---
			require.Len(t, conf.HTTP.Routers, len(expectedRules))
			for route, rule := range expectedRules {
				assert.Equal(t, rule, conf.HTTP.Routers[findRouter(t, conf.HTTP.Routers, "httproute-default-"+route+"-gw-")].Rule)
			}

			muxer := newHTTPMuxer(t, conf.HTTP.Routers)
			for host, routes := range expectedRoutes {
				for _, path := range paths {
					routerName := matchRouter(muxer, host, path)
					if route, ok := routes[path]; ok {
						assert.True(t, strings.HasPrefix(routerName, "httproute-default-"+route+"-gw-"), "%s%s: %s", host, path, routerName)
						continue
					}
					assert.Empty(t, routerName, "%s%s", host, path)
				}
			}

			executed := false
			if routeStatus, ok := statusReport.httpRoutes[shadowedKey]; ok {
				executed = true
				require.Len(t, routeStatus.Parents, 1)
				accepted := meta.FindStatusCondition(routeStatus.Parents[0].Conditions, string(gatev1.RouteConditionAccepted))
				require.NotNil(t, accepted)
				assert.Equal(t, metav1.ConditionTrue, accepted.Status)
			}
			t.Logf("Accepted=True assertion block executed: %v", executed)
		})
	}
}
