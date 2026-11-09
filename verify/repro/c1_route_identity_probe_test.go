// Run: bash verify/repro/c1_run.sh <worktree of evalon/traefik-ga-c8563c30, -b52f8b88 or -12fa8560>  (copies this file into pkg/provider/kubernetes/gateway, runs go test -tags verify -run TestVerifyC1, then replays every exchange against the authored tests)

//go:build verify

package gateway

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// verifyC1Load loads a fixture exactly the way the authored listener-isolation tests do.
func verifyC1Load(t *testing.T, fixture string) *dynamic.Configuration {
	t.Helper()

	k8sObjects, gwObjects := readResources(t, []string{"services.yml", fixture})

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), newGatewaySimpleClientSet(t, gwObjects...))

	eventCh, err := client.WatchAll(nil, make(chan struct{}))
	require.NoError(t, err)
	<-eventCh

	p := Provider{EntryPoints: map[string]Entrypoint{"web": {Address: ":80"}}, client: client}

	conf, _, err := p.loadConfigurationFromGateways(t.Context())
	require.NoError(t, err)

	return conf
}

// verifyC1RouterFor returns the key of the router generated for the HTTPRoute/GRPCRoute attached to the given listener
// (the authored fixtures name those routes "attache[sd]-to-<listener>").
func verifyC1RouterFor(t *testing.T, conf *dynamic.Configuration, listener string) string {
	t.Helper()

	var found []string
	for name := range conf.HTTP.Routers {
		if strings.Contains(name, "-default-attached-to-"+listener+"-gw-") || strings.Contains(name, "-default-attaches-to-"+listener+"-gw-") {
			found = append(found, name)
		}
	}
	require.Len(t, found, 1, "routers generated for the route attached to listener %q", listener)

	return found[0]
}

// verifyC1SwapRules exchanges the complete matching side (rule, priority, TLS) of the broader and the more specific
// backend routers, leaving each router key and backend service in place.
func verifyC1SwapRules(t *testing.T, conf *dynamic.Configuration, broader, specific string) {
	t.Helper()

	b := conf.HTTP.Routers[verifyC1RouterFor(t, conf, broader)]
	s := conf.HTTP.Routers[verifyC1RouterFor(t, conf, specific)]
	b.Rule, s.Rule = s.Rule, b.Rule
	b.Priority, s.Priority = s.Priority, b.Priority
	b.TLS, s.TLS = s.TLS, b.TLS
}

// verifyC1Hook is injected by c1_run.sh right after the authored tests load their configuration.
// It is a no-op unless VERIFY_C1_SWAP_CONF lists listener pairs, e.g. "empty-hostname,abc-foo-example-com;empty-hostname,wildcard-example-com":
// the first pair whose two backend routers both exist in the loaded configuration is exchanged.
func verifyC1Hook(t *testing.T, conf *dynamic.Configuration) {
	t.Helper()

	has := func(listener string) bool {
		for name := range conf.HTTP.Routers {
			if strings.Contains(name, "-default-attached-to-"+listener+"-gw-") || strings.Contains(name, "-default-attaches-to-"+listener+"-gw-") {
				return true
			}
		}
		return false
	}

	for _, pair := range strings.Split(os.Getenv("VERIFY_C1_SWAP_CONF"), ";") {
		listeners := strings.Split(pair, ",")
		if len(listeners) != 2 || !has(listeners[0]) || !has(listeners[1]) {
			continue
		}
		verifyC1SwapRules(t, conf, listeners[0], listeners[1])
		t.Logf("VERIFY hook: exchanged rule/priority/TLS between the backend routers of listeners %q and %q", listeners[0], listeners[1])
		return
	}
}

func verifyC1Dispatch(t *testing.T, conf *dynamic.Configuration) func(method, host, path string) (int, string) {
	t.Helper()

	parser, err := httpmuxer.NewSyntaxParser()
	require.NoError(t, err)

	muxer := httpmuxer.NewMuxer(parser, nil)
	muxer.SetDefaultHandler(http.NotFoundHandler())
	for name, router := range conf.HTTP.Routers {
		require.Empty(t, router.ParentRefs, "router %s is not flat", name)

		priority := router.Priority
		if priority == 0 {
			priority = httpmuxer.GetRulePriority(router.Rule)
		}
		require.NoError(t, muxer.AddRoute(router.Rule, router.RuleSyntax, priority, "", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("X-Verify-Router", name)
		})))
	}

	decorator := requestdecorator.New(nil)

	return func(method, host, path string) (int, string) {
		req := httptest.NewRequest(method, "http://"+host+path, nil)
		rec := httptest.NewRecorder()
		// The Host matchers read the canonical host the production request decorator stores in the request context.
		decorator.ServeHTTP(rec, req, muxer.ServeHTTP)
		return rec.Code, rec.Header().Get("X-Verify-Router")
	}
}

func verifyC1Dump(t *testing.T, conf *dynamic.Configuration) {
	t.Helper()

	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for name := range conf.HTTP.Routers {
			if !yield(name) {
				return
			}
		}
	}) {
		router := conf.HTTP.Routers[name]
		fmt.Printf("ROUTER   key=%s service=%s priority=%d rule=%s\n", name, router.Service, router.Priority, router.Rule)
	}
}

func verifyC1Probe(t *testing.T, fixture, method, broader, specific, broaderPath, specificPath, specificHost string) {
	t.Helper()

	for _, exchanged := range []bool{false, true} {
		conf := verifyC1Load(t, fixture)
		if exchanged {
			verifyC1SwapRules(t, conf, broader, specific)
		}

		fmt.Printf("CONFIG   fixture=%s rule-exchange-in-loaded-config=%v\n", fixture, exchanged)
		verifyC1Dump(t, conf)

		broaderKey := verifyC1RouterFor(t, conf, broader)
		specificKey := verifyC1RouterFor(t, conf, specific)
		tag := func(key string) string {
			switch key {
			case "":
				return "<no router>"
			case broaderKey:
				return "BROADER-BACKEND-ROUTER(" + broader + ")"
			case specificKey:
				return "SPECIFIC-BACKEND-ROUTER(" + specific + ")"
			}
			return "other:" + key
		}

		serve := verifyC1Dispatch(t, conf)
		for _, req := range []struct{ host, path string }{
			{"unrelated.test", broaderPath},
			{specificHost, broaderPath},
			{specificHost, specificPath},
			{"unrelated.test", specificPath},
		} {
			code, key := serve(method, req.host, req.path)
			rule := ""
			if key != "" {
				rule = conf.HTTP.Routers[key].Rule
			}
			fmt.Printf("DISPATCH %s http://%s%s -> %d servedBy=%s rule=%s\n", method, req.host, req.path, code, tag(key), rule)
		}
	}
}

func TestVerifyC1_HTTPRouteDispatchIdentity(t *testing.T) {
	verifyC1Probe(t, "httproute/with_listener_isolation.yml", http.MethodGet,
		"empty-hostname", "abc-foo-example-com", "/empty-hostname", "/abc-foo-example-com", "abc.foo.example.com")
}

func TestVerifyC1_GRPCRouteDispatchIdentity(t *testing.T) {
	verifyC1Probe(t, "grpcroute/with_listener_isolation.yml", http.MethodPost,
		"empty-hostname", "wildcard-example-com", "/foo/bar", "/foo/bar", "x.example.com")
}
