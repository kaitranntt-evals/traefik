// Run: sh verify/repro/C2_29c47f2b_empty_secret_diagnostic.sh (it copies this file into pkg/server/middleware/ of evalon/traefik-se-29c47f2b); PASS means the problem reproduced.
// It reuses decodingPluginBuilder from the branch's own secrets_test.go.
package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
)

func TestRepro_C2_EmptySecretPluginErrorDiagnostic(t *testing.T) {
	build := func(token string) (string, []string) {
		rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{
			Middlewares: map[string]*dynamic.Middleware{"plugin@kubernetescrd": {
				Plugin: map[string]dynamic.PluginConf{"test": {"port": "urn:k8s:secret:plugin:port", "token": "urn:k8s:secret:plugin:token"}},
				Secrets: dynamic.Secrets{
					"urn:k8s:secret:plugin:port":  "8080",
					"urn:k8s:secret:plugin:token": token,
				},
			}},
		}})

		pluginBuilder := &decodingPluginBuilder{newErr: func(token string) error { return fmt.Errorf("token %q is revoked", token) }}
		// The branch's test plugin rejects tokens shorter than 8 characters before its constructor runs, so the
		// constructor error path is reached with a padded value only for the non-empty control.
		_, err := NewBuilder(rtConf.Middlewares, nil, pluginBuilder).BuildMiddlewareChain(t.Context(), []string{"plugin@kubernetescrd"}).Then(http.NotFoundHandler())
		require.Error(t, err)

		return err.Error(), rtConf.Middlewares["plugin@kubernetescrd"].Err
	}

	control, controlRecorded := build("s3cr3t-token")
	t.Logf("non-empty Secret: error=%q recorded=%q", control, controlRecorded)
	assert.Contains(t, control, "urn:k8s:secret:plugin:token")

	// The redaction applied by the builder, on an error that quotes the resolved Secret value.
	for _, value := range []string{"s3cr3t-token", ""} {
		secrets := dynamic.Secrets{"urn:k8s:secret:plugin:token": value}
		original := fmt.Errorf("token %q is revoked", value)
		diagnostic := secrets.RedactError(original).Error()
		t.Logf("RedactError(Secret value %q): original=%q diagnostic=%q contains URN=%v",
			value, original.Error(), diagnostic, strings.Contains(diagnostic, "urn:k8s:secret:plugin:token"))
		// The problem, production side: the diagnostic of an empty Secret value does not name its reference.
		assert.Equal(t, value != "", strings.Contains(diagnostic, "urn:k8s:secret:plugin:token"))
	}

	empty, emptyRecorded := build("")
	t.Logf("empty Secret    : error=%q recorded=%q contains URN=%v", empty, emptyRecorded, strings.Contains(empty, "urn:k8s:secret:plugin:token"))
	assert.NotContains(t, empty, "urn:k8s:secret:plugin:token")

	// The assertion the branch's regression test makes for the empty Secret.
	src, err := os.ReadFile("../../config/dynamic/secrets_test.go")
	require.NoError(t, err)
	lines := strings.Split(string(src), "\n")
	found := false
	for i, line := range lines {
		if strings.Contains(line, `desc:     "empty value"`) {
			for j := i - 1; j <= i+4; j++ {
				t.Logf("pkg/config/dynamic/secrets_test.go:%d: %s", j+1, lines[j])
			}
			// The problem, test side: the regression test requires the diagnostic without the reference.
			found = strings.Contains(lines[i+1], `Secrets{"urn:k8s:secret:plugin:token": ""}`) &&
				strings.Contains(lines[i+2], "errors.New(`invalid value \"\"`)") &&
				strings.Contains(lines[i+3], "expected: `invalid value \"\"`")
		}
	}
	assert.True(t, found)
}
