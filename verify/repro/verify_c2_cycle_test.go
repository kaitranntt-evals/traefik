// Run via verify/repro/c2_cycle_coverage.sh (copies this file into pkg/server/ for the run).
package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

// Same cyclic routers as the branch's subtest, plus one TLS router with an entry point,
// which is what populates routersByEntryPoint and makes the traversal run.
func TestVerifyC2_CycleReachesTraversal(t *testing.T) {
	routers := map[string]*dynamic.Router{
		"a@file":    {Rule: "Method(`GET`)", ParentRefs: []string{"b@file"}},
		"b@file":    {Rule: "Host(`foo.example.com`)", ParentRefs: []string{"a@file"}, Service: "ext@file"},
		"root@file": {EntryPoints: []string{"websecure"}, Rule: "Host(`foo.example.com`)", TLS: &dynamic.RouterTLSConfig{}},
	}
	got := resolveHTTPTLSOptions(routers, false)
	require.Contains(t, got, "a@file")
	require.Contains(t, got, "b@file")
}
