//go:build verify

// Run (branch evalon/traefik-se-d5bacf39, eval fixtures provisioned in pkg/provider/kubernetes/crd/):
//   cp verify/repro/c2_plugin_alias_probe_test.go <worktree>/pkg/provider/kubernetes/crd/zz_verify_c2_test.go
//   go test -tags verify -count=1 -run TestVerifyC2 -v ./pkg/provider/kubernetes/crd/
package crd_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
)

// verifyRecorder is a PluginsBuilder that records the constructor argument and mutates every container in it.
type verifyRecorder struct {
	got     map[string]any
	secrets dynamic.Secrets
	fail    bool
}

func (r *verifyRecorder) Build(_ string, config map[string]any, secrets dynamic.Secrets, _ string) (plugins.Constructor, error) {
	r.got = config
	r.secrets = secrets
	verifyMutate(config)
	if r.fail {
		return nil, fmt.Errorf("verify: builder failure")
	}
	return func(_ context.Context, next http.Handler) (http.Handler, error) { return next, nil }, nil
}

func verifyMutate(v any) {
	switch n := v.(type) {
	case map[string]any:
		for k, c := range n {
			verifyMutate(c)
			if _, ok := c.(string); ok {
				n[k] = "MUTATED"
			}
		}
		n["verify_added"] = "MUTATED"
	case []any:
		for i, c := range n {
			verifyMutate(c)
			if _, ok := c.(string); ok {
				n[i] = "MUTATED"
			}
		}
	}
}

// verifyContainers collects the pointers of every map/slice reachable in v.
func verifyContainers(v any, out map[uintptr]bool) {
	switch n := v.(type) {
	case map[string]any:
		out[reflect.ValueOf(n).Pointer()] = true
		for _, c := range n {
			verifyContainers(c, out)
		}
	case dynamic.PluginConf:
		verifyContainers(map[string]any(n), out)
	case []any:
		if len(n) > 0 {
			out[reflect.ValueOf(n).Pointer()] = true
		}
		for _, c := range n {
			verifyContainers(c, out)
		}
	}
}

func TestVerifyC2(t *testing.T) {
	for _, file := range []string{"with_plugin_deep_read_secret.yml", "with_plugin_read_array_of_secret.yml", "with_plugin_read_array_of_map_contain_secret.yml", "with_plugin_read_secret.yml"} {
		for _, mode := range []string{"recorder-success", "recorder-failure", "yaegi-success", "yaegi-constructor-error", "yaegi-decode-error", "CONTROL-recorder-secrets-stripped"} {
			conf, provider := crd.LoadConfigurationAndProviderForTest(t, []string{"services.yml", file})
			published := conf.HTTP.Middlewares["default-test-secret"]
			require.NotNil(t, published)
			require.NotEmpty(t, published.Secrets, "provider-loaded Secret bindings")
			pc := published.Plugin["test-secret"]
			pc["mutation"] = []any{"unchanged", map[string]any{"literal": "unchanged", "values": []any{"unchanged"}}}

			var builder any
			rec := &verifyRecorder{}
			switch mode {
			case "recorder-success":
				builder = rec
			case "recorder-failure":
				rec.fail = true
				builder = rec
			case "CONTROL-recorder-secrets-stripped":
				published.Secrets = nil
				builder = rec
			case "yaegi-success":
				builder = newEvalLocalPluginBuilder(t)
			case "yaegi-constructor-error":
				pc["eval_fail_constructor"] = true
				builder = newEvalLocalPluginBuilder(t)
			case "yaegi-decode-error":
				pc["eval_reject"] = "not-an-integer"
				builder = newEvalLocalPluginBuilder(t)
			}

			before, err := json.Marshal(published.Plugin)
			require.NoError(t, err)
			shared := map[uintptr]bool{}
			verifyContainers(pc, shared)

			for i := range 2 {
				_, rtConf, buildErr := evalBuildHandler(t, conf, provider, builder, "default-test-secret")
				after, err := json.Marshal(published.Plugin)
				require.NoError(t, err)
				rt, err := json.Marshal(evalMiddlewareInfo(rtConf, "default-test-secret").Plugin)
				require.NoError(t, err)
				aliases := -1
				if rec.got != nil {
					aliases = 0
					got := map[uintptr]bool{}
					verifyContainers(rec.got, got)
					for p := range got {
						if shared[p] {
							aliases++
						}
					}
				}
				fmt.Printf("C2 %-48s %-34s build#%d err=%v aliasedContainers=%d publishedUnchanged=%v runtimeUnchanged=%v\n",
					file, mode, i+1, buildErr != nil, aliases, string(before) == string(after), string(before) == string(rt))
			}
		}
	}
}
