// Run: cp verify/repro/c2_router_manager_api_errors_test.go ../wt-c2/pkg/server/router/ && (cd ../wt-c2 && go test -tags verify -count=1 -v -run '^TestVerifyC2_RouterManager' ./pkg/server/router)

//go:build verify

package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	"github.com/traefik/traefik/v3/pkg/plugins"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	"github.com/traefik/traefik/v3/pkg/server/service"
	traefiktls "github.com/traefik/traefik/v3/pkg/tls"
)

type verifyC2FailingPlugin struct{}

func (*verifyC2FailingPlugin) Build(string, map[string]any, string) (plugins.Constructor, error) {
	return func(context.Context, http.Handler) (http.Handler, error) {
		return nil, errors.New("upstream rejected the credentials")
	}, nil
}

// One configuration load: eight routers share one plugin middleware whose constructor fails.
// rtConf is the runtime configuration the API (/api/http/middlewares, /api/http/routers, /api/rawdata) serves.
func TestVerifyC2_RouterManagerAPIErrors(t *testing.T) {
	const routers = 8

	routersConfig := map[string]*dynamic.Router{}
	for i := range routers {
		routersConfig[fmt.Sprintf("app%d@kubernetescrd", i)] = &dynamic.Router{
			EntryPoints: []string{"web"},
			Rule:        fmt.Sprintf("Host(`app%d.example.com`)", i),
			Service:     "svc@kubernetescrd",
			Middlewares: []string{"plugin@kubernetescrd"},
		}
	}

	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Routers: routersConfig,
			Services: map[string]*dynamic.Service{
				"svc@kubernetescrd": {LoadBalancer: &dynamic.ServersLoadBalancer{
					Strategy: dynamic.BalancerStrategyWRR,
					Servers:  []dynamic.Server{{URL: "http://127.0.0.1:8080"}},
				}},
			},
			Middlewares: map[string]*dynamic.Middleware{
				"plugin@kubernetescrd": {
					Plugin: map[string]dynamic.PluginConf{
						"test": {
							"apiKey":       "urn:k8s:secret:plugin-creds:apiKey",
							"clientSecret": "urn:k8s:secret:plugin-creds:clientSecret",
							"password":     "urn:k8s:secret:redis-creds:password",
							"token":        "urn:k8s:secret:plugin-creds:token",
						},
					},
					Secrets: map[string]string{
						"urn:k8s:secret:plugin-creds:apiKey":       "value-a",
						"urn:k8s:secret:plugin-creds:clientSecret": "value-b",
						"urn:k8s:secret:redis-creds:password":      "value-c",
						"urn:k8s:secret:plugin-creds:token":        "value-d",
					},
				},
			},
		},
	})

	transportManager := service.NewTransportManager(nil)
	transportManager.Update(map[string]*dynamic.ServersTransport{"default@internal": {}})

	serviceManager := service.NewManager(rtConf.Services, nil, nil, transportManager, proxyBuilderMock{})
	middlewaresBuilder := middleware.NewBuilder(rtConf.Middlewares, serviceManager, &verifyC2FailingPlugin{})

	parser, err := httpmuxer.NewSyntaxParser()
	require.NoError(t, err)

	routerManager := NewManager(rtConf, serviceManager, middlewaresBuilder, nil, traefiktls.NewManager(nil), parser, []string{})
	_ = routerManager.BuildHandlers(t.Context(), []string{"web"}, false)

	info := rtConf.Middlewares["plugin@kubernetescrd"]
	out, err := json.MarshalIndent(struct {
		Status string   `json:"status"`
		Error  []string `json:"error"`
	}{Status: info.Status, Error: info.Err}, "", "  ")
	require.NoError(t, err)
	t.Logf("middleware plugin@kubernetescrd as the API reports it:\n%s", out)

	var routerErrors []string
	for _, name := range slices.Sorted(func(yield func(string) bool) {
		for name := range rtConf.Routers {
			if !yield(name) {
				return
			}
		}
	}) {
		t.Logf("router %s status=%s error=%q", name, rtConf.Routers[name].Status, rtConf.Routers[name].Err)
		routerErrors = append(routerErrors, rtConf.Routers[name].Err...)
	}
	slices.Sort(routerErrors)
	t.Logf("distinct router error strings for the one failure: %d", len(slices.Compact(routerErrors)))

	assert.Len(t, info.Err, 1, "one failing middleware must report one error")
}
