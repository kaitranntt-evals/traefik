//go:build verify

// Run (branch evalon/traefik-se-5b5c5895, eval fixtures provisioned in pkg/provider/kubernetes/crd/):
//   mkdir -p <worktree>/pkg/provider/kubernetes/crd/plugins-local/src/verifyretain
//   cp verify/repro/verifyretain/verifyretain.go verify/repro/verifyretain/.traefik.yml <worktree>/pkg/provider/kubernetes/crd/plugins-local/src/verifyretain/
//   cp verify/repro/c4_retained_createconfig_test.go <worktree>/pkg/provider/kubernetes/crd/zz_verify_c4_test.go
//   go test -tags verify -count=1 -run TestVerifyC4 -v ./pkg/provider/kubernetes/crd/
package crd_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
)

func TestVerifyC4(t *testing.T) {
	for _, tc := range []struct {
		name   string
		file   string
		reject any
		strip  bool
	}{
		{name: "failed decoding, deep map", file: "with_plugin_deep_read_secret.yml", reject: "not-an-integer"},
		{name: "failed decoding, list of maps", file: "with_plugin_read_array_of_map_contain_secret.yml", reject: "not-an-integer"},
		{name: "failed decoding, list", file: "with_plugin_read_array_of_secret.yml", reject: "not-an-integer"},
		{name: "CONTROL successful decoding, deep map", file: "with_plugin_deep_read_secret.yml"},
	} {
		// A fresh production plugins.Builder (fresh Yaegi interpreter, so no retained state) per case.
		builder, err := plugins.NewBuilder(nil, nil, map[string]plugins.LocalDescriptor{"test-secret": {ModuleName: "verifyretain"}})
		require.NoError(t, err)

		conf, provider := crd.LoadConfigurationAndProviderForTest(t, []string{"services.yml", tc.file})
		published := conf.HTTP.Middlewares["default-test-secret"]
		require.NotNil(t, published)
		if tc.reject != nil {
			published.Plugin["test-secret"]["eval_reject"] = tc.reject
		}
		before, err := json.Marshal(published.Plugin)
		require.NoError(t, err)
		fmt.Printf("C4 [%s]\nC4   published before : %s\n", tc.name, before)

		for i := range 2 {
			_, rtConf, buildErr := evalBuildHandler(t, conf, provider, builder, "default-test-secret")
			after, err := json.Marshal(published.Plugin)
			require.NoError(t, err)
			rt, err := json.Marshal(evalMiddlewareInfo(rtConf, "default-test-secret").Plugin)
			require.NoError(t, err)
			fmt.Printf("C4   build#%d err=%v\nC4   published after  : %s\nC4   runtime (API)     : %s\nC4   unchanged=%v\n", i+1, buildErr, after, rt, string(before) == string(after))
		}
	}
}
