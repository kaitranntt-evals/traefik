// Package verifyplugin is the audit-owned local Yaegi plugin (run v-63ba222f); install.sh copies it to pkg/provider/kubernetes/crd/plugins-local/src/verifyplugin.
//
// Unlike the eval plugin, the Secret-bound values land in typed fields that the
// production decoder (pkg/plugins createConfig, mapstructure) fills: an int, a
// nested struct and a list of structs. Everything else lands in Remain.
package verifyplugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// User is one entry of the typed users list.
type User struct {
	Name   string `mapstructure:"name" json:"name"`
	Secret string `mapstructure:"secret" json:"secret"`
}

// Inner is the innermost typed nested value.
type Inner struct {
	Token string `mapstructure:"token" json:"token"`
	Label string `mapstructure:"label" json:"label"`
}

// Deep is a typed nested value.
type Deep struct {
	Inner Inner `mapstructure:"inner" json:"inner"`
}

// Config is the typed plugin configuration.
type Config struct {
	Port       int                    `mapstructure:"port" json:"port"`
	Enabled    bool                   `mapstructure:"enabled" json:"enabled"`
	Token      string                 `mapstructure:"token" json:"token"`
	Public     string                 `mapstructure:"public" json:"public"`
	Deep       Deep                   `mapstructure:"deep" json:"deep"`
	Users      []User                 `mapstructure:"users" json:"users"`
	FailFormat string                 `mapstructure:"fail_format" json:"-"`
	Remain     map[string]interface{} `mapstructure:",remain" json:"remain"`
}

// CreateConfig returns the value production decodes the plugin configuration into.
func CreateConfig() *Config {
	return &Config{}
}

// New reports the decoded configuration, then mutates every nested container it
// can reach through Remain (the aliasing probe), then optionally fails with an
// error that quotes the Secret-bound token.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	body, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	mutate(config.Remain)

	if config.FailFormat != "" {
		return nil, fmt.Errorf(config.FailFormat, config.Token)
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusOK)
		_, _ = rw.Write(body)
	}), nil
}

func mutate(value interface{}) {
	if node, ok := value.(map[string]interface{}); ok {
		for key, nested := range node {
			if _, isLeaf := nested.(string); isLeaf {
				node[key] = "MUTATED-BY-PLUGIN"
			} else {
				mutate(nested)
			}
		}
		node["injected-by-plugin"] = "MUTATED-BY-PLUGIN"
		return
	}

	if list, ok := value.([]interface{}); ok {
		for i, nested := range list {
			mutate(nested)
			if _, isLeaf := nested.(string); isLeaf {
				list[i] = "MUTATED-BY-PLUGIN"
			}
		}
	}
}
