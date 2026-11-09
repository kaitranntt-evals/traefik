// Run: bash verify/repro/c3_run.sh <worktree of evalon/traefik-ga-a6535cd8> <dir of the extracted eval_tests.zip>  (this file is the in-package exporter shim of c3_testlocal_status_vs_production_test.go)

//go:build verify

package gateway

import (
	"net/http"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

// VerifyC3Load exposes the branch's own fixture loader (authored test helper, unmodified).
func VerifyC3Load(t *testing.T, fixture string, entryPoints map[string]Entrypoint) *dynamic.Configuration {
	t.Helper()
	return loadListenerIsolationConfiguration(t, fixture, entryPoints)
}

// VerifyC3TestLocalStatus replays, verbatim, the status computation of the authored
// TestListenerIsolation_HTTPSDetectMisdirectedRequests loop body using the authored helpers
// buildListenerIsolationMuxer / buildSNITLSOptions / serveListenerIsolationRequest.
func VerifyC3TestLocalStatus(t *testing.T, conf *dynamic.Configuration, ep string) func(serverName, host, path string) (int, string) {
	t.Helper()

	mux := buildListenerIsolationMuxer(t, conf, ep)
	sniOptions := buildSNITLSOptions(t, conf)

	return func(serverName, host, path string) (int, string) {
		routerName := serveListenerIsolationRequest(mux, "https://"+host+path, nil)
		if routerName == "" {
			return 0, ""
		}

		router := conf.HTTP.Routers[routerName]

		statusCode := http.StatusOK
		switch {
		case sniOptions(serverName) != router.TLS.Options:
			statusCode = http.StatusMisdirectedRequest
		case conf.HTTP.Services[router.Service].Weighted != nil && conf.HTTP.Services[router.Service].Weighted.Services[0].Status != nil:
			statusCode = *conf.HTTP.Services[router.Service].Weighted.Services[0].Status
		}

		return statusCode, routerName
	}
}
