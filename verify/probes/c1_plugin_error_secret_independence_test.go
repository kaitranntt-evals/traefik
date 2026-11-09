// Run: git worktree add --detach ../wt-c1 <evalon/traefik-se-19e992fb> && cp verify/probes/c1_plugin_error_secret_independence_test.go ../wt-c1/pkg/server/middleware/ && (cd ../wt-c1 && go test -tags verify -count=1 -v -run '^TestVerifyC1_' ./pkg/server/middleware)

//go:build verify

package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

// The three callbacks and expected diagnostics are copied verbatim from TestBuilder_PluginSecretReferenceError
// (pkg/server/middleware/secrets_test.go on evalon/traefik-se-19e992fb); only the Secret value is varied.
func TestVerifyC1_AssertedDiagnosticsAreSecretIndependent(t *testing.T) {
	const tokenRef = "urn:k8s:secret:plugin:token"

	testCases := []struct {
		desc          string
		token         string
		build         func(token string) error
		expectedError string
	}{
		{
			desc:  "decoding error quoting the value",
			token: "s3cr3t",
			build: func(token string) error {
				return fmt.Errorf("cannot decode token %q", token)
			},
			expectedError: fmt.Sprintf("plugin: cannot decode token %q", tokenRef),
		},
		{
			desc:  "value equal to a word of the error",
			token: "invalid",
			build: func(string) error {
				return errors.New("invalid token")
			},
			expectedError: "plugin: invalid token",
		},
		{
			desc:  "error only with the value",
			token: "s3cr3t",
			build: func(token string) error {
				if token == tokenRef {
					return nil
				}
				return fmt.Errorf("token %q is rejected", token)
			},
			expectedError: "plugin: invalid configuration referencing " + tokenRef + ": error details are hidden as they may contain Secret values",
		},
	}

	// The value the branch's test configures, then values sharing nothing with the fixed wording.
	otherTokens := []string{"Zq7#kP2x-9fLm", "hunter2", "correct horse battery staple", "0123456789abcdef0123456789abcdef"}

	for _, test := range testCases {
		t.Run(test.desc, func(t *testing.T) {
			// What is left of the asserted text once public metadata (the reference) is removed.
			residue := strings.ReplaceAll(test.expectedError, tokenRef, "")
			t.Logf("asserted diagnostic        : %q", test.expectedError)
			t.Logf("configured Secret value    : %q", test.token)
			t.Logf("asserted text minus the ref: %q (contains the configured value: %v)", residue, strings.Contains(residue, test.token))

			for _, token := range append([]string{test.token}, otherTokens...) {
				var seen []string
				pluginBuilder := pluginBuilderFunc(func(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
					seen = append(seen, config["token"].(string))
					if err := test.build(config["token"].(string)); err != nil {
						return nil, err
					}
					return func(_ context.Context, next http.Handler) (http.Handler, error) {
						return next, nil
					}, nil
				})

				rtConf := runtime.NewConfig(dynamic.Configuration{
					HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"secret": {
						Plugin:  map[string]dynamic.PluginConf{"test": {"token": tokenRef}},
						Secrets: map[string]string{tokenRef: token},
					}}},
				})

				_, err := NewBuilder(rtConf.Middlewares, nil, pluginBuilder).BuildMiddlewareChain(t.Context(), []string{"secret"}).Then(http.NotFoundHandler())
				require.Error(t, err)

				infoErr := rtConf.Middlewares["secret"].Err
				t.Logf("Secret=%-36q plugin saw %q -> Then() err %q ; MiddlewareInfo.Err %q", token, seen, err.Error(), infoErr)

				// Same assertion as the branch's test, for every Secret value.
				assert.EqualError(t, err, test.expectedError)
				assert.Equal(t, []string{test.expectedError}, infoErr)

				if !strings.Contains(test.expectedError, token) {
					assert.NotContains(t, err.Error(), token)
				}
			}
		})
	}
}
