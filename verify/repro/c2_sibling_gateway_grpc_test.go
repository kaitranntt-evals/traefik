//go:build verify

// Run: cp verify/repro/c2_sibling_gateway_grpc_test.go pkg/provider/kubernetes/gateway/ && go test -tags verify -count=1 -v -run 'TestVerifyC2' ./pkg/provider/kubernetes/gateway/
// (on branch evalon/traefik-ga-5af6118e). Two sibling Gateways on the same entryPoint: A has a hostname-free listener,
// B has a foo.example.com listener; both carry a GRPCRoute for helloworld.Greeter/SayHello. The second case adds an
// unrelated long reserved listener hostname to Gateway A and re-sends the same request.
package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/k8s"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
)

const verifyC2Base = `
kind: GatewayClass
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: my-gateway-class
spec:
  controllerName: traefik.io/gateway-controller
---
apiVersion: v1
kind: Service
metadata:
  name: whoami
  namespace: default
spec:
  ports:
    - name: web
      port: 80
      targetPort: 80
  selector:
    app: whoami
---
apiVersion: discovery.k8s.io/v1
kind: EndpointSlice
metadata:
  name: whoami-abc
  namespace: default
  labels:
    kubernetes.io/service-name: whoami
addressType: IPv4
ports:
  - name: web
    port: 80
endpoints:
  - addresses:
      - 10.10.0.1
    conditions:
      ready: true
---
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: gateway-b
  namespace: default
spec:
  gatewayClassName: my-gateway-class
  listeners:
    - name: exact
      protocol: HTTP
      port: 80
      hostname: "foo.example.com"
      allowedRoutes:
        namespaces:
          from: Same
---
kind: GRPCRoute
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: grpc-b-foo
  namespace: default
spec:
  parentRefs:
    - name: gateway-b
      sectionName: exact
  rules:
    - matches:
        - method:
            service: "helloworld.Greeter"
            method: "SayHello"
      backendRefs:
        - name: whoami
          port: 80
---
kind: GRPCRoute
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: grpc-a-anyhost
  namespace: default
spec:
  parentRefs:
    - name: gateway-a
      sectionName: catchall
  rules:
    - matches:
        - method:
            service: "helloworld.Greeter"
            method: "SayHello"
      backendRefs:
        - name: whoami
          port: 80
`

func verifyC2GatewayA(reservedHostname string) string {
	doc := `
kind: Gateway
apiVersion: gateway.networking.k8s.io/v1
metadata:
  name: gateway-a
  namespace: default
spec:
  gatewayClassName: my-gateway-class
  listeners:
    - name: catchall
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: Same
`
	if reservedHostname != "" {
		doc += `    - name: reserved
      protocol: HTTP
      port: 80
      hostname: "` + reservedHostname + `"
      allowedRoutes:
        namespaces:
          from: Same
`
	}
	return doc
}

func verifyC2Load(t *testing.T, docs ...string) *dynamic.Configuration {
	t.Helper()

	var k8sObjects, gwObjects []runtime.Object
	for _, obj := range k8s.MustParseYaml([]byte(strings.Join(docs, "\n---\n"))) {
		if obj.GetObjectKind().GroupVersionKind().Group == gatev1.GroupName {
			gwObjects = append(gwObjects, obj)
			continue
		}
		k8sObjects = append(k8sObjects, obj)
	}

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), newGatewaySimpleClientSet(t, gwObjects...))
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	eventCh, err := client.WatchAll(nil, stop)
	require.NoError(t, err)
	select {
	case <-eventCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for informer sync")
	}

	p := Provider{EntryPoints: map[string]Entrypoint{"web": {Address: ":80"}}, client: client}
	conf, _, err := p.loadConfigurationFromGateways(t.Context())
	require.NoError(t, err)
	return conf
}

func verifyC2Serve(t *testing.T, conf *dynamic.Configuration, rawURL string) string {
	t.Helper()

	parser, err := httpmuxer.NewSyntaxParser()
	require.NoError(t, err)
	muxer := httpmuxer.NewMuxer(parser, nil)
	for name, router := range conf.HTTP.Routers {
		require.NoError(t, muxer.AddRoute(router.Rule, router.RuleSyntax, router.Priority, "", http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("X-Router", name)
		})))
	}

	req := httptest.NewRequest(http.MethodPost, rawURL, http.NoBody)
	req.Header.Set("Content-Type", "application/grpc")
	rw := httptest.NewRecorder()
	requestdecorator.New(nil).ServeHTTP(rw, req, muxer.ServeHTTP)
	return rw.Header().Get("X-Router")
}

func TestVerifyC2_siblingGatewayGRPCPriority(t *testing.T) {
	const reserved = "reserved-listener-hostname-unrelated-to-foo-example-com.internal.example.org"

	// Part 1 - rule construction: the exclusion text is counted in the priority.
	rule, priority := buildGRPCMatchRule(nil, nil, gatev1.GRPCRouteMatch{})
	t.Logf("construction: no exclusion  -> rule=%q priority=%d len(rule)=%d", rule, priority, len(rule))
	ruleEx, priorityEx := buildGRPCMatchRule(nil, []gatev1.Hostname{reserved}, gatev1.GRPCRouteMatch{})
	t.Logf("construction: with exclusion -> rule=%q priority=%d len(rule)=%d", ruleEx, priorityEx, len(ruleEx))
	assert.Equal(t, len(ruleEx), priorityEx)
	assert.Greater(t, priorityEx, priority)

	// Part 2 - request selection across two sibling Gateways.
	for _, reservedHostname := range []string{"", reserved} {
		name := "gateway-a-without-reserved-listener"
		if reservedHostname != "" {
			name = "gateway-a-with-reserved-listener"
		}
		t.Run(name, func(t *testing.T) {
			conf := verifyC2Load(t, verifyC2Base, verifyC2GatewayA(reservedHostname))
			for routerName, router := range conf.HTTP.Routers {
				t.Logf("router %s priority=%d rule=%s", routerName, router.Priority, router.Rule)
			}

			got := verifyC2Serve(t, conf, "http://foo.example.com/helloworld.Greeter/SayHello")
			t.Logf("request POST http://foo.example.com/helloworld.Greeter/SayHello -> router %q", got)
			assert.Contains(t, got, "grpc-b-foo", "foo.example.com must stay on Gateway B's hostname-specific route")
		})
	}
}
