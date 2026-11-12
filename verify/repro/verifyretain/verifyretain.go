// Package verifyretain is the audit-owned retaining local Yaegi plugin (run v-63ba222f); install.sh copies it to pkg/provider/kubernetes/crd/plugins-local/src/verifyretain.
//
// It keeps (a) every *Config it returned from CreateConfig, i.e. the object the
// production decoder fills, and (b) the *Config its handler was built with, and
// writes to the nested containers of those objects only AFTER the decoder and
// the New callback have returned.
package verifyretain

import (
	"context"
	"fmt"
	"net/http"
)

// Config is the typed plugin configuration.
type Config struct {
	Port    int                    `mapstructure:"port"`
	Enabled bool                   `mapstructure:"enabled"`
	Fire    bool                   `mapstructure:"fire"`
	Fail    bool                   `mapstructure:"fail"`
	Remain  map[string]interface{} `mapstructure:",remain"`
}

var retained []*Config

// CreateConfig returns the value production decodes the plugin configuration into, and keeps it.
func CreateConfig() *Config {
	config := &Config{}
	retained = append(retained, config)
	return config
}

// New never touches the configuration while it runs. A "fire" instance rewrites
// the nested containers of every configuration object retained so far; any other
// instance rewrites its own retained configuration when it serves a request
// carrying the X-Verify-Mutate header.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	if config.Fire {
		count := 0
		for _, kept := range retained {
			count += mutate(kept.Remain)
		}
		retained = nil
		return http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(rw, "%d", count)
		}), nil
	}

	if config.Fail {
		return nil, fmt.Errorf("verifyretain constructor rejected the configuration")
	}

	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		count := 0
		if req.Header.Get("X-Verify-Mutate") != "" {
			count = mutate(config.Remain)
		}
		_, _ = fmt.Fprintf(rw, "%d", count)
	}), nil
}

func mutate(value interface{}) int {
	count := 0
	if node, ok := value.(map[string]interface{}); ok {
		for key, nested := range node {
			if _, isLeaf := nested.(string); isLeaf {
				node[key] = "MUTATED-AFTER-CALLBACK"
				count++
			} else {
				count += mutate(nested)
			}
		}
		return count
	}

	if list, ok := value.([]interface{}); ok {
		for i, nested := range list {
			if _, isLeaf := nested.(string); isLeaf {
				list[i] = "MUTATED-AFTER-CALLBACK"
				count++
			} else {
				count += mutate(nested)
			}
		}
	}
	return count
}
