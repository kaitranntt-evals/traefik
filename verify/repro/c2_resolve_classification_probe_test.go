// Run (claim branch evalon/traefik-se-59e46302 only, it uses that branch's dynamic.Secrets API): cp verify/repro/c2_resolve_classification_probe_test.go <worktree>/pkg/server/middleware/ && (cd <worktree> && go test -tags verify -count=1 -v -run '^TestVerifyC2_InlineCredentialClassification$' ./pkg/server/middleware)

//go:build verify

package middleware

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

func TestVerifyC2_InlineCredentialClassification(t *testing.T) {
	const inlinePassword = "urn:k8s:secret:this-is-my-literal-redis-password"

	fmt.Printf("C2 IsSecretReference(%q) = %v\n", inlinePassword, dynamic.IsSecretReference(inlinePassword))

	for _, test := range []struct {
		desc    string
		secrets dynamic.Secrets
		value   string
	}{
		{desc: "nil bindings, urn-prefixed literal", secrets: nil, value: inlinePassword},
		{desc: "empty bindings, urn-prefixed literal", secrets: dynamic.Secrets{}, value: inlinePassword},
		{desc: "nil bindings, ordinary literal (control)", secrets: nil, value: "ordinary-password"},
	} {
		got, err := test.secrets.Resolve(test.value)
		fmt.Printf("C2 Secrets.Resolve [%s]: value=%q err=%v\n", test.desc, got, err)
	}

	middleware := &dynamic.Middleware{RateLimit: &dynamic.RateLimit{Average: 100, Redis: &dynamic.Redis{Endpoints: []string{"127.0.0.1:6379"}, Password: inlinePassword}}}
	resolved, err := middleware.ResolveSecrets()
	fmt.Printf("C2 Middleware.ResolveSecrets (Secrets == nil: %v): resolved-nil=%v err=%v\n", middleware.Secrets == nil, resolved == nil, err)

	got, err := dynamic.Secrets(nil).Resolve(inlinePassword)
	assert.NoError(t, err, "a literal inline password must not be treated as an unresolved Secret reference")
	assert.Equal(t, inlinePassword, got)
}
