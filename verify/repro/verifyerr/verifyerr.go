// Package verifyerr is a local Yaegi plugin for claims C5/C6: its constructor fails with selectable error shapes
// that carry the credential it received. Run: see verify/repro/c5_c6_plugin_error_probe_test.go.
package verifyerr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Config selects the error shape through Mode.
type Config struct {
	Mode   string `mapstructure:"mode"`
	Secret string `mapstructure:"secret"`
	User   string `mapstructure:"user"`
}

// CreateConfig returns the configuration to decode into.
func CreateConfig() *Config { return &Config{} }

type opaqueError struct {
	message string
	cause   error
}

func (e *opaqueError) Error() string { return e.message }
func (e *opaqueError) Unwrap() error { return e.cause }

// cachedErr is the state of the "cached" mode: the first credential-bearing error, returned unchanged afterwards.
var cachedErr error

// New fails according to Mode.
func New(_ context.Context, next http.Handler, config *Config, _ string) (http.Handler, error) {
	cause := fmt.Errorf("underlying failure with secret: %s", config.Secret)
	switch config.Mode {
	case "interpreted-unwrap":
		// A plugin-defined error type hiding the cause behind Unwrap.
		return nil, &opaqueError{message: "verifyerr: internal error", cause: cause}
	case "native-wrap":
		// A stdlib (host) *fmt.wrapError whose message hides the cause: %.0w wraps it and prints nothing.
		return nil, fmt.Errorf("verifyerr: internal error%.0w", cause)
	case "native-join":
		return nil, errors.Join(errors.New("verifyerr: internal error"), cause)
	case "cached":
		if cachedErr == nil && !strings.HasPrefix(config.Secret, "urn:k8s:secret:") {
			cachedErr = fmt.Errorf("verifyerr: rejected credential %q", config.Secret)
		}
		if cachedErr != nil {
			return nil, cachedErr
		}
		return next, nil
	case "cached-always":
		// Caches whatever error the first invocation produced, whatever it received.
		if cachedErr == nil {
			cachedErr = fmt.Errorf("verifyerr: rejected credential %q", config.Secret)
		}
		return nil, cachedErr
	}
	return next, nil
}
