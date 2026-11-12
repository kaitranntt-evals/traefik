//go:build verify

// Run: cp verify/repro/verify_c3_resolvedrefs_test.go pkg/provider/kubernetes/gateway/ (next to the eval fixtures eval_isolation_test.go, eval_https_test.go, export_eval_test.go) && go test -tags verify -count=1 -v -run '^TestVerifyC3_' ./pkg/provider/kubernetes/gateway

package gateway

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
	gatefake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

// verifyC3Load mirrors the eval fixture loader evalIsoLoadWithStatus, but keeps the fake Gateway API
// clientset so the status report can be flushed and the published route status read back,
// and attaches a debug logger so the loader execution is traced.
func verifyC3Load(t *testing.T, docs ...string) (published map[string][]gatev1.RouteParentStatus, routers []string, logs string) {
	t.Helper()

	require.NoError(t, gatev1.Install(kscheme.Scheme))

	var k8sObjects, gwObjects []runtime.Object
	for _, obj := range k8s.MustParseYaml([]byte(strings.Join(docs, "\n---\n"))) {
		if obj.GetObjectKind().GroupVersionKind().Group == gatev1.GroupName {
			gwObjects = append(gwObjects, obj)
			continue
		}
		k8sObjects = append(k8sObjects, obj)
	}

	gwClient := gatefake.NewSimpleClientset(gwObjects...)
	for _, obj := range gwObjects {
		if gw, ok := obj.(*gatev1.Gateway); ok {
			_, err := gwClient.GatewayV1().Gateways(gw.Namespace).Create(t.Context(), gw, metav1.CreateOptions{})
			require.NoError(t, err)
		}
	}

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), gwClient)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })

	eventCh, err := client.WatchAll(nil, stop)
	require.NoError(t, err)

	select {
	case <-eventCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for informer sync")
	}

	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.DebugLevel)
	ctx := logger.WithContext(t.Context())

	p := Provider{EntryPoints: map[string]Entrypoint{"web": {Address: ":80"}}, StatusAddress: &StatusAddress{IP: "1.2.3.4"}, client: client}
	conf, report, err := p.loadConfigurationFromGateways(ctx)
	require.NoError(t, err)

	for name, r := range conf.HTTP.Routers {
		routers = append(routers, fmt.Sprintf("%s rule=%s service=%s", name, r.Rule, r.Service))
	}
	slices.Sort(routers)

	// Publish the statuses exactly as the provider loop does, then read them back from the API.
	report.Flush(ctx, client)

	published = map[string][]gatev1.RouteParentStatus{}
	hrs, err := gwClient.GatewayV1().HTTPRoutes("default").List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	for _, r := range hrs.Items {
		published["HTTPRoute/"+r.Name] = r.Status.Parents
	}
	grs, err := gwClient.GatewayV1().GRPCRoutes("default").List(t.Context(), metav1.ListOptions{})
	require.NoError(t, err)
	for _, r := range grs.Items {
		published["GRPCRoute/"+r.Name] = r.Status.Parents
	}

	return published, routers, buf.String()
}

func verifyC3Route(kind, name, section, backend string, hostnames ...string) string {
	var doc string
	if kind == "http" {
		doc = evalIsoRouteWithService(name, "my-gateway", section, "/x", backend)
	} else {
		doc = strings.Replace(evalIsoGRPCRoute(name, "my-gateway", section, "SayHello"), "- name: whoami", "- name: "+backend, 1)
	}
	if len(hostnames) > 0 {
		doc = evalIsoWithHostnames(doc, hostnames...)
	}
	return doc
}

func TestVerifyC3_ResolvedRefsOnEmptyHostnameIntersection(t *testing.T) {
	twoListeners := evalIsoGateway("my-gateway", "wildcard=*.example.com", "precise=abc.example.com")
	oneListener := evalIsoGateway("my-gateway", "wildcard=*.example.com")

	for _, tc := range []struct {
		desc      string
		gateway   string
		section   string
		backend   string
		hostnames []string
	}{
		{desc: "CLAIM outside.test + missing Service, listeners *.example.com + abc.example.com", gateway: twoListeners, backend: "missing-svc", hostnames: []string{"outside.test"}},
		{desc: "CONTROL outside.test + existing Service whoami", gateway: twoListeners, backend: "whoami", hostnames: []string{"outside.test"}},
		{desc: "CONTROL bar.example.com (intersects wildcard listener) + missing Service", gateway: twoListeners, backend: "missing-svc", hostnames: []string{"bar.example.com"}},
		{desc: "CONTROL no route hostnames + missing Service", gateway: twoListeners, backend: "missing-svc"},
		{desc: "EXTRA outside.test + missing Service, single listener *.example.com (no isolation involved)", gateway: oneListener, backend: "missing-svc", hostnames: []string{"outside.test"}},
		{desc: "EXTRA abc.example.com with sectionName=wildcard (shadowed there, but the precise listener iteration still validates) + missing Service", gateway: twoListeners, section: "wildcard", backend: "missing-svc", hostnames: []string{"abc.example.com"}},
	} {
		for _, kind := range []string{"http", "grpc"} {
			t.Run(kind+"/"+tc.desc, func(t *testing.T) {
				name := kind + "-probe"
				published, routers, logs := verifyC3Load(t, evalIsoWhoamiYAML, evalIsoClassYAML, tc.gateway,
					verifyC3Route(kind, name, tc.section, tc.backend, tc.hostnames...))

				key := "HTTPRoute/" + name
				if kind == "grpc" {
					key = "GRPCRoute/" + name
				}
				parents := published[key]
				fmt.Printf("C3 | %s | %s | hostnames=%v backend=%s section=%q\n", kind, tc.desc, tc.hostnames, tc.backend, tc.section)
				if len(parents) == 0 {
					fmt.Printf("C3 |   published status: <no parents>\n")
				}
				for _, p := range parents {
					for _, c := range p.Conditions {
						fmt.Printf("C3 |   published %s parent=%s: %s=%s reason=%s message=%q\n", key, p.ParentRef.Name, c.Type, c.Status, c.Reason, c.Message)
					}
				}
				for _, r := range routers {
					fmt.Printf("C3 |   router %s\n", r)
				}
				for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
					if strings.Contains(line, "Skipping") || strings.Contains(line, "missing-svc") || strings.Contains(line, "not found") || strings.Contains(line, "annot load") {
						fmt.Printf("C3 |   log %s\n", line)
					}
				}
			})
		}
	}
}
