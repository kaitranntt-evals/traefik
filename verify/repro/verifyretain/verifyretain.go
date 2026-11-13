// Package verifyretain is a local Yaegi plugin for claim C4: it retains every CreateConfig result and, on each
// later CreateConfig call, mutates the nested containers of the results it retained earlier.
// Run: see verify/repro/c4_retained_createconfig_test.go.
package verifyretain

import (
	"context"
	"net/http"
)

// Config has one typed int field (so a non-numeric value fails decoding) and collects everything else verbatim.
type Config struct {
	Reject int                    `mapstructure:"eval_reject"`
	Remain map[string]interface{} `mapstructure:",remain"`
}

var retained []*Config

// CreateConfig retains its result.
func CreateConfig() *Config {
	for _, c := range retained {
		mutate(c.Remain, true)
	}
	c := &Config{}
	retained = append(retained, c)
	return c
}

func mutate(value interface{}, top bool) {
	if node, ok := value.(map[string]interface{}); ok {
		for key, nested := range node {
			if _, leaf := nested.(string); leaf {
				if !top {
					node[key] = "MUTATED-BY-RETAINED-CONFIG"
				}
			} else {
				mutate(nested, false)
			}
		}
		if !top {
			node["verify_added"] = "MUTATED-BY-RETAINED-CONFIG"
		}
		return
	}
	if list, ok := value.([]interface{}); ok {
		for i, nested := range list {
			if _, leaf := nested.(string); leaf {
				list[i] = "MUTATED-BY-RETAINED-CONFIG"
			} else {
				mutate(nested, false)
			}
		}
	}
}

// New is never reached in the failed-decoding scenario.
func New(_ context.Context, next http.Handler, _ *Config, _ string) (http.Handler, error) {
	return next, nil
}
