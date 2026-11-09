// Run: bash verify/repro/c3_run.sh <worktree of evalon/traefik-ga-a6535cd8> <dir of the extracted eval_tests.zip>  (copies this file, its exporter shim and the eval fixtures into pkg/provider/kubernetes/gateway and runs go test -tags verify -run TestVerifyC3)

//go:build verify

package gateway_test

import (
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
)

// TestVerifyC3_TestLocalVsProduction prints, for each row of the authored
// TestListenerIsolation_HTTPSDetectMisdirectedRequests table (plus a few extra SNI/Host pairs),
// the status the authored test computes locally next to the status produced by production
// handling (configuration watcher -> TLS-option aggregation -> router factory -> SNICheck),
// driven through the eval fixture harness evalServeHTTPSFull.
func TestVerifyC3_TestLocalVsProduction(t *testing.T) {
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
	conf := gateway.VerifyC3Load(t, "httproute/with_https_listener_detect_misdirected_requests.yml", eps)

	local := gateway.VerifyC3TestLocalStatus(t, conf, "websecure")
	prod := evalServeHTTPSFull(t, conf, nil, "websecure", false)

	rows := []struct {
		sni, host string
		asserted  int
	}{
		{"example.org", "example.org", 200},
		{"example.org", "second-example.org", 421},
		{"example.org", "unknown-example.org", 404},
		{"second-example.org", "second-example.org", 200},
		{"second-example.org", "example.org", 421},
		{"second-example.org", "unknown-example.org", 421},
		{"third-example.wildcard.org", "third-example.wildcard.org", 200},
		{"third-example.wildcard.org", "fith-example.wildcard.org", 200},
		{"third-example.wildcard.org", "fourth-example.wildcard.org", 421},
		{"third-example.wildcard.org", "second-example.org", 421},
		{"third-example.wildcard.org", "unknown-example.org", 421},
		{"fourth-example.wildcard.org", "fourth-example.wildcard.org", 200},
		{"fourth-example.wildcard.org", "fith-example.wildcard.org", 421},
		{"unknown-example.org", "example.org", 200},
		{"unknown-example.org", "unknown-example.org", 404},
		// Extra rows, not in the authored table (asserted=0).
		{"a.b.wildcard.org", "a.b.wildcard.org", 0},
		{"a.b.wildcard.org", "second-example.org", 0},
		{"second-example.org", "a.b.wildcard.org", 0},
		{"SECOND-EXAMPLE.ORG", "second-example.org", 0},
		{"second-example.org.", "second-example.org", 0},
		{"", "second-example.org", 0},
		{"", "example.org", 0},
	}

	const path = "/detect-misdirected-requests"
	diverged := 0
	for _, r := range rows {
		localStatus, routerName := local(r.sni, r.host, path)
		res := prod(r.sni, r.host, path)
		mark := ""
		if localStatus != res.Code {
			mark = "  <-- DIVERGES"
			diverged++
		}
		t.Logf("sni=%-28q host=%-28q asserted=%3d test-local=%3d production=%3d router=%s%s", r.sni, r.host, r.asserted, localStatus, res.Code, routerName, mark)
	}
	t.Logf("rows=%d diverged=%d", len(rows), diverged)
}
