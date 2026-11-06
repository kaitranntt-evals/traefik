// Run (C1 helper): copy next to c1_rejected_listener_misdirection_test.go into pkg/provider/kubernetes/gateway/ (package gateway, exports the status report to the external test package).
package gateway

import (
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
)

// EvalIsoLoadWithStatusExported exposes the loader together with the listener status report.
func EvalIsoLoadWithStatusExported(t *testing.T, entryPoints map[string]Entrypoint, extra []runtime.Object, docs ...string) (*dynamic.Configuration, *statusReport) {
	t.Helper()
	return evalIsoLoadWithStatus(t, entryPoints, extra, docs...)
}

// ListenersExported returns the listeners of every selected Gateway.
func (s *statusReport) ListenersExported() []gatewayListener {
	var out []gatewayListener
	for _, gw := range s.gatewayListeners {
		out = append(out, gw.listeners...)
	}
	return out
}

// HostnameExported returns the listener hostname, or "" when it has none.
func (l gatewayListener) HostnameExported() string {
	return string(ptr.Deref(l.Hostname, ""))
}
