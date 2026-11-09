// Run: cp verify/probes/c2_ordering_scale_test.go ../wt-c2/pkg/server/middleware/ && (cd ../wt-c2 && go test -tags verify -count=1 -v -run '^TestVerifyC2Scale_' ./pkg/server/middleware)

//go:build verify

package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

const verifyC2ScaleBuilds = 2000

func verifyC2ScaleFlat(n int) *dynamic.Middleware {
	conf := dynamic.PluginConf{}
	secrets := map[string]string{}
	for i := range n {
		ref := fmt.Sprintf("urn:k8s:secret:creds:key%02d", i)
		conf[fmt.Sprintf("opt%02d", i)] = ref
		secrets[ref] = fmt.Sprintf("value-%02d", i)
	}

	return &dynamic.Middleware{Plugin: map[string]dynamic.PluginConf{"test": conf}, Secrets: secrets}
}

// Two references at the top level and two inside a nested map, the shape of a plugin with a nested options block.
func verifyC2ScaleNested() *dynamic.Middleware {
	return &dynamic.Middleware{
		Plugin: map[string]dynamic.PluginConf{"test": {
			"opt00": "urn:k8s:secret:creds:key00",
			"opt01": "urn:k8s:secret:creds:key01",
			"nested": map[string]any{
				"opt02": "urn:k8s:secret:creds:key02",
				"opt03": "urn:k8s:secret:creds:key03",
			},
		}},
		Secrets: map[string]string{
			"urn:k8s:secret:creds:key00": "value-00",
			"urn:k8s:secret:creds:key01": "value-01",
			"urn:k8s:secret:creds:key02": "value-02",
			"urn:k8s:secret:creds:key03": "value-03",
		},
	}
}

func verifyC2IsRotationOfSorted(order []string) bool {
	sorted := slices.Sorted(slices.Values(order))
	for shift := range len(sorted) {
		if slices.Equal(order, append(slices.Clone(sorted[shift:]), sorted[:shift]...)) {
			return true
		}
	}

	return false
}

func TestVerifyC2Scale_DistinctOrderingsBySize(t *testing.T) {
	failing := &pluginBuilderMock{build: func(map[string]any) (plugins.Constructor, error) {
		return func(context.Context, http.Handler) (http.Handler, error) {
			return nil, errors.New("upstream rejected the credentials")
		}, nil
	}}

	testCases := []struct {
		desc   string
		refs   int
		config func() *dynamic.Middleware
	}{
		{desc: "flat 2 references", refs: 2, config: func() *dynamic.Middleware { return verifyC2ScaleFlat(2) }},
		{desc: "flat 3 references", refs: 3, config: func() *dynamic.Middleware { return verifyC2ScaleFlat(3) }},
		{desc: "flat 4 references", refs: 4, config: func() *dynamic.Middleware { return verifyC2ScaleFlat(4) }},
		{desc: "flat 8 references", refs: 8, config: func() *dynamic.Middleware { return verifyC2ScaleFlat(8) }},
		{desc: "flat 9 references", refs: 9, config: func() *dynamic.Middleware { return verifyC2ScaleFlat(9) }},
		{desc: "flat 12 references", refs: 12, config: func() *dynamic.Middleware { return verifyC2ScaleFlat(12) }},
		{desc: "2 top-level + 2 nested references", refs: 4, config: verifyC2ScaleNested},
	}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			// Fresh runtime configuration per build: distinct diagnostics across independent identical builds.
			distinct := map[string]struct{}{}
			rotations := 0
			for range verifyC2ScaleBuilds {
				rtConf := runtime.NewConfig(dynamic.Configuration{
					HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": test.config()}},
				})
				_, err := NewBuilder(rtConf.Middlewares, nil, failing).BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
				require.Error(t, err)

				message := err.Error()
				if _, ok := distinct[message]; ok {
					continue
				}
				distinct[message] = struct{}{}

				list := strings.TrimPrefix(message, "plugin: creating middleware with the values of ")
				list = strings.TrimSuffix(list, " failed, the error is not reported as it may disclose them")
				if verifyC2IsRotationOfSorted(strings.Split(list, ", ")) {
					rotations++
				}
			}

			// One shared runtime configuration: what MiddlewareInfo.Err accumulates when many routers use the middleware.
			rtConf := runtime.NewConfig(dynamic.Configuration{
				HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": test.config()}},
			})
			builder := NewBuilder(rtConf.Middlewares, nil, failing)
			for range verifyC2ScaleBuilds {
				_, err := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
				require.Error(t, err)
			}

			t.Logf("refs=%d builds=%d distinct_diagnostics=%d of_which_rotations_of_sorted_order=%d MiddlewareInfo.Err_entries_after_%d_shared_builds=%d",
				test.refs, verifyC2ScaleBuilds, len(distinct), rotations, verifyC2ScaleBuilds, len(rtConf.Middlewares["plugin"].Err))
		})
	}
}
