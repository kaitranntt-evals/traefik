// Run: cp verify/repro/c4_alt_quote_redaction_test.go pkg/server/middleware/verify_probe_c4_test.go && go test -tags verify -count=1 -run '^TestVerifyC4_' -v ./pkg/server/middleware (on branch evalon/traefik-se-0d33c7f2)
//go:build verify

package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

type quotingPluginBuilder struct {
	format string
}

func (b *quotingPluginBuilder) Build(_ string, config map[string]any, _ string) (plugins.Constructor, error) {
	return nil, fmt.Errorf(b.format, config["token"])
}

func TestVerifyC4_PluginErrorQuoteStyles(t *testing.T) {
	const ref = "urn:k8s:secret:plugin:token"
	const secret = "s3cr3t-value-XYZ"

	tests := []struct {
		name   string
		format string
	}{
		{name: "double_quoted_%q", format: "invalid token %q"},
		{name: "single_quoted", format: "invalid token '%s'"},
		{name: "unquoted", format: "invalid token %s"},
		{name: "backtick_quoted", format: "invalid token `%s`"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configs := map[string]*runtime.MiddlewareInfo{
				"plugin": {
					Middleware: &dynamic.Middleware{
						Plugin:       map[string]dynamic.PluginConf{"test": {"token": ref}},
						SecretValues: map[string]string{ref: secret},
					},
				},
			}

			b := NewBuilder(configs, nil, &quotingPluginBuilder{format: test.format})
			chain := b.BuildMiddlewareChain(context.Background(), []string{"plugin"})
			_, err := chain.Then(http.NotFoundHandler())
			if err == nil {
				t.Fatal("expected build error")
			}

			stored := strings.Join(configs["plugin"].Err, " | ")
			t.Logf("runtime error : %s", err)
			t.Logf("stored API err: %s", stored)
			t.Logf("status        : %s", configs["plugin"].Status)
			if strings.Contains(stored, secret) {
				t.Errorf("LEAK: stored diagnostic contains resolved Secret value %q", secret)
			}
			if !strings.Contains(stored, ref) {
				t.Errorf("stored diagnostic does not mention the reference %q", ref)
			}
		})
	}
}
