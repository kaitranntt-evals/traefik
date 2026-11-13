// Run: cp verify/repro/c3456_resolve_trace_test.go pkg/server/verify_c3456_resolve_trace_test.go && go test -tags verify -count=1 -v -run TestVerify_ResolveTrace ./pkg/server
//go:build verify

package server

import (
	"fmt"
	"sort"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

// TestVerify_ResolveTrace prints what resolveParentChildDomains collects for external router
// trees and what resolveHTTPTLSOptions (strict mode off) then decides.
func TestVerify_ResolveTrace(t *testing.T) {
	gwExact := func() map[string]*dynamic.Router {
		return map[string]*dynamic.Router{
			"listener@kubernetesgateway": {EntryPoints: []string{"websecure"}, Rule: "Host(`foo.example.com`)", TLS: &dynamic.RouterTLSConfig{Options: "listener"}},
			"gw-app@kubernetesgateway":   {Rule: "Host(`foo.example.com`) && Path(`/gw`)", ParentRefs: []string{"listener@kubernetesgateway"}, Service: "gw@kubernetesgateway"},
		}
	}
	gwWild := func() map[string]*dynamic.Router {
		return map[string]*dynamic.Router{
			"listener@kubernetesgateway": {EntryPoints: []string{"websecure"}, Rule: "Host(`**.example.com`)", TLS: &dynamic.RouterTLSConfig{Options: "listener"}},
			"gw-app@kubernetesgateway":   {Rule: "Host(`**.example.com`) && Path(`/gw`)", ParentRefs: []string{"listener@kubernetesgateway"}, Service: "gw@kubernetesgateway"},
		}
	}
	root := func(opts string) *dynamic.Router {
		return &dynamic.Router{EntryPoints: []string{"websecure"}, Rule: "PathPrefix(`/other`)", TLS: &dynamic.RouterTLSConfig{Options: opts}}
	}
	for _, tc := range []struct {
		name string
		gw   func() map[string]*dynamic.Router
		ext  map[string]*dynamic.Router
	}{
		{"control: child Host(foo.example.com) / gateway exact", gwExact, map[string]*dynamic.Router{
			"ext-root@file": root("custom"), "ext-child@file": {Rule: "Host(`foo.example.com`)", ParentRefs: []string{"ext-root@file"}, Service: "s@file"}}},
		{"C3/C5: grandchild Host(foo.example.com) behind intermediate / gateway exact", gwExact, map[string]*dynamic.Router{
			"ext-root@file": root("custom"), "ext-mid@file": {Rule: "Method(`GET`)", ParentRefs: []string{"ext-root@file"}},
			"ext-grandchild@file": {Rule: "Host(`foo.example.com`)", ParentRefs: []string{"ext-mid@file"}, Service: "s@file"}}},
		{"C4: child Host(*.example.com) / gateway exact foo.example.com", gwExact, map[string]*dynamic.Router{
			"ext-root@file": root("custom"), "ext-child@file": {Rule: "Host(`*.example.com`)", ParentRefs: []string{"ext-root@file"}, Service: "s@file"}}},
		{"C4/C6: child Host(*.example.com) / gateway **.example.com", gwWild, map[string]*dynamic.Router{
			"ext-root@file": root("custom"), "ext-child@file": {Rule: "Host(`*.example.com`)", ParentRefs: []string{"ext-root@file"}, Service: "s@file"}}},
		{"control: flat external Host(*.example.com) / gateway **.example.com", gwWild, map[string]*dynamic.Router{
			"ext@file": {EntryPoints: []string{"websecure"}, Rule: "Host(`*.example.com`) && Path(`/other`)", TLS: &dynamic.RouterTLSConfig{Options: "custom"}, Service: "s@file"}}},
	} {
		all := tc.gw()
		for n, r := range tc.ext {
			all[n] = r
		}
		ep := map[string]*dynamic.Router{}
		for n, r := range all {
			if len(r.EntryPoints) > 0 {
				ep[n] = r
			}
		}
		fmt.Printf("CASE %s\n", tc.name)
		cd := resolveParentChildDomains(ep, all)
		var keys []string
		for k := range ep {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  childDomains[%s] = %v\n", k, cd[k])
		}
		got := resolveHTTPTLSOptions(all, false)
		keys = keys[:0]
		for k := range got {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if got[k].TLS != nil {
				fmt.Printf("  resolved %-48s TLS.ResolvedOptions=%s\n", k, got[k].TLS.ResolvedOptions)
			}
		}
	}
}
