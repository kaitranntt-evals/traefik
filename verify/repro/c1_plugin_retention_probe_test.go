//go:build verify

// Run (on evalon/traefik-se-9c319c44, eval fixtures installed): cp this file to pkg/provider/kubernetes/crd/ && go test -tags verify -count=1 -v -run '^TestVerify_C1' ./pkg/provider/kubernetes/crd
package crd_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	"github.com/traefik/traefik/v3/pkg/redactor"
	"gopkg.in/yaml.v3"
)

func TestVerify_C1_PluginRetentionAndSerialization(t *testing.T) {
	for _, tc := range []struct {
		file   string
		refs   []string
		values []string
	}{
		{"with_plugin_read_secret.yml", []string{"urn:k8s:secret:name:key"}, []string{"this_is_the_secret"}},
		{"with_plugin_deep_read_secret.yml", []string{"urn:k8s:secret:name:key"}, []string{"this_is_the_very_deep_secret"}},
		{"with_plugin_read_array_of_secret.yml", []string{"urn:k8s:secret:name:key1", "urn:k8s:secret:name:key2"}, []string{"secret_data1", "secret_data2"}},
		{"with_plugin_read_array_of_map_contain_secret.yml", []string{"urn:k8s:secret:name:key1", "urn:k8s:secret:name:key2"}, []string{"admin_password", "user_password"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			conf := crd.LoadConfigurationForTest(t, []string{"services.yml", tc.file})
			mw := conf.HTTP.Middlewares["default-test-secret"]
			require.NotNil(t, mw)
			t.Logf("retained Go value (%%#v): %#v", mw.Plugin)

			outputs := map[string]string{}
			b, err := json.Marshal(conf)
			require.NoError(t, err)
			outputs["json"] = string(b)
			b, err = yaml.Marshal(mw.Plugin)
			require.NoError(t, err)
			outputs["yaml"] = string(b)
			var sb strings.Builder
			require.NoError(t, toml.NewEncoder(&sb).Encode(map[string]any(mw.Plugin["test-secret"])))
			outputs["toml"] = sb.String()
			outputs["fmt %v"] = fmt.Sprintf("%v", mw.Plugin)
			outputs["fmt %+v"] = fmt.Sprintf("%+v", mw.Plugin)
			outputs["fmt %#v"] = fmt.Sprintf("%#v", mw.Plugin)
			outputs["deepcopy json"] = func() string { b, _ := json.Marshal(mw.DeepCopy()); return string(b) }()
			outputs["redactor.RemoveCredentials"], err = redactor.RemoveCredentials(conf)
			require.NoError(t, err)
			outputs["redactor.Anonymize"], err = redactor.Anonymize(conf)
			require.NoError(t, err)

			for name, out := range outputs {
				for _, v := range tc.values {
					if strings.Contains(out, v) {
						t.Errorf("LEAK %s exposes resolved value %q", name, v)
					}
				}
				if strings.HasPrefix(name, "redactor.") {
					// The redactor blanks every plugin value (literals too) on every branch, including the reference solution; leak check only.
					t.Logf("%s: %s", name, out)
					continue
				}
				for _, r := range tc.refs {
					if !strings.Contains(out, r) {
						t.Errorf("LOST %s no longer represents %q: %s", name, r, out)
					}
				}
			}
			t.Logf("json plugin: %s", func() string { b, _ := json.Marshal(mw.Plugin); return string(b) }())
			t.Logf("yaml plugin: %s", outputs["yaml"])
		})
	}
}
