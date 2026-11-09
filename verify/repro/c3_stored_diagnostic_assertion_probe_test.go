// Run (claim branch evalon/traefik-se-6205ead6): cp verify/repro/c3_stored_diagnostic_assertion_probe_test.go <worktree>/pkg/server/middleware/ && (cd <worktree> && go test -tags verify -count=1 -v -run '^TestVerifyC3_' ./pkg/server/middleware)

//go:build verify

package middleware

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

// verifyC3Recorder stands for *testing.T so that an assertion can be evaluated without failing the probe.
type verifyC3Recorder struct{ failed bool }

func (r *verifyC3Recorder) Errorf(string, ...any) { r.failed = true }

// TestVerifyC3_CallPathLeavesErrEmpty replays the setup and call of TestBuilder_buildConstructor_pluginSecretReference
// (case "decoding error quoting the value") and prints the Err slice its last assertion inspects.
func TestVerifyC3_CallPathLeavesErrEmpty(t *testing.T) {
	const (
		ref    = "urn:k8s:secret:pluginsecret:token"
		secret = "secret-token"
	)

	middleware := &dynamic.Middleware{
		Plugin:  map[string]dynamic.PluginConf{"test": {"token": ref, "nested": map[string]any{"token": ref}}},
		Secrets: map[string]string{ref: secret},
	}
	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"plugin": middleware}},
	})

	pluginBuilder := pluginBuilderFunc(func(pName string, config map[string]any, middlewareName string) (plugins.Constructor, error) {
		return nil, fmt.Errorf("failed to decode configuration: 'token' expected type 'int', got unconvertible type 'string', value: '%v'", config["token"])
	})
	builder := NewBuilder(rtConf.Middlewares, nil, pluginBuilder)

	_, err := builder.buildConstructor(t.Context(), "plugin")
	require.Error(t, err)

	errs := rtConf.Middlewares["plugin"].Err
	fmt.Printf("C3 call path buildConstructor: returned error = %q\n", err)
	fmt.Printf("C3 call path buildConstructor: MiddlewareInfo.Err = %#v (len %d)\n", errs, len(errs))

	rec := &verifyC3Recorder{}
	assert.NotContains(rec, errs, secret)
	fmt.Printf("C3 the test's assertion on that Err: rejected=%v\n", rec.failed)

	assert.Empty(t, errs, "buildConstructor never records an error, only BuildMiddlewareChain calls AddError")

	// The path a router takes: only here is a diagnostic stored, and the branch's test never goes through it.
	_, chainErr := builder.BuildMiddlewareChain(t.Context(), []string{"plugin"}).Then(http.NotFoundHandler())
	require.Error(t, chainErr)
	fmt.Printf("C3 call path BuildMiddlewareChain: MiddlewareInfo.Err = %q\n", rtConf.Middlewares["plugin"].Err)
}

// TestVerifyC3_AssertionAgainstDefectiveStates evaluates the test's stored-diagnostic assertion,
// assert.NotContains(t, rtConf.Middlewares["plugin"].Err, test.secret), against defective Err states.
func TestVerifyC3_AssertionAgainstDefectiveStates(t *testing.T) {
	const (
		ref         = "urn:k8s:secret:pluginsecret:token"
		secret      = "secret-token"
		expectedErr = "plugin: failed to decode configuration: 'token' expected type 'int', got unconvertible type 'string', value: '" + ref + "'"
	)

	states := []struct {
		desc      string
		errs      []string
		defective bool
	}{
		{desc: "part 1: nil Err (nothing recorded)", errs: nil, defective: true},
		{desc: "part 1: empty Err (nothing recorded)", errs: []string{}, defective: true},
		{desc: "part 2: Err entry embedding the secret value", errs: []string{"plugin: failed to decode configuration: 'token' expected type 'int', got unconvertible type 'string', value: '" + secret + "'"}, defective: true},
		{desc: "control: Err entry equal to the bare secret value", errs: []string{secret}, defective: true},
		{desc: "control: sanitized Err entry", errs: []string{expectedErr}, defective: false},
	}

	for _, state := range states {
		notContains := &verifyC3Recorder{}
		assert.NotContains(notContains, state.errs, secret)

		strict := &verifyC3Recorder{}
		assert.Equal(strict, []string{expectedErr}, state.errs)

		fmt.Printf("C3 %-52s | defective=%-5v | assert.NotContains(Err, secret) rejects=%-5v | assert.Equal([]string{expectedErr}, Err) rejects=%v\n",
			state.desc, state.defective, notContains.failed, strict.failed)

		if state.defective {
			assert.True(t, notContains.failed, "the stored-diagnostic assertion accepts a defective state: %s", state.desc)
		}
	}
}
