package gateway

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	"github.com/traefik/traefik/v3/pkg/middlewares/snicheck"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ktypes "k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
	"sigs.k8s.io/gateway-api/pkg/features"
)

func TestHTTPRouteListenerIsolation(t *testing.T) {
	testRouteListenerIsolation(t, kindHTTPRoute)
}

func TestGRPCRouteListenerIsolation(t *testing.T) {
	testRouteListenerIsolation(t, kindGRPCRoute)
}

func testRouteListenerIsolation(t *testing.T, kind string) {
	t.Helper()

	testCases := []struct {
		desc           string
		hostname       *gatev1.Hostname
		otherHostname  gatev1.Hostname
		routeHostnames []gatev1.Hostname
		allowed        []string
		denied         []string
	}{
		{
			desc:          "catch-all excludes exact listener without routes",
			otherHostname: "api.example.com",
			allowed:       []string{"example.com", "other.example.com"},
			denied:        []string{"api.example.com", "API.EXAMPLE.COM:443"},
		},
		{
			desc:          "catch-all excludes wildcard listener without routes",
			otherHostname: "*.example.com",
			allowed:       []string{"example.com", "other.com"},
			denied:        []string{"api.example.com", "deep.api.example.com", "API.EXAMPLE.COM.:443"},
		},
		{
			desc:          "wildcard excludes exact listener without routes",
			hostname:      new(gatev1.Hostname("*.example.com")),
			otherHostname: "api.example.com",
			allowed:       []string{"other.example.com", "deep.api.example.com"},
			denied:        []string{"api.example.com", "example.com"},
		},
		{
			desc:          "wildcard excludes more specific wildcard",
			hostname:      new(gatev1.Hostname("*.example.com")),
			otherHostname: "*.foo.example.com",
			allowed:       []string{"foo.example.com", "other.example.com"},
			denied:        []string{"api.foo.example.com", "deep.api.foo.example.com"},
		},
		{
			desc:          "exact listener keeps precedence over wildcard",
			hostname:      new(gatev1.Hostname("api.example.com")),
			otherHostname: "*.example.com",
			allowed:       []string{"api.example.com"},
			denied:        []string{"other.example.com"},
		},
		{
			desc:          "more specific wildcard keeps precedence",
			hostname:      new(gatev1.Hostname("*.foo.example.com")),
			otherHostname: "*.example.com",
			allowed:       []string{"api.foo.example.com", "deep.api.foo.example.com"},
			denied:        []string{"foo.example.com"},
		},
		{
			desc:           "route hostname cannot override listener selection",
			hostname:       new(gatev1.Hostname("*.example.com")),
			otherHostname:  "*.foo.example.com",
			routeHostnames: []gatev1.Hostname{"api.foo.example.com"},
			denied:         []string{"api.foo.example.com"},
		},
		{
			desc:           "exclusions apply to every route hostname",
			otherHostname:  "api.example.com",
			routeHostnames: []gatev1.Hostname{"api.example.com", "other.example.com"},
			allowed:        []string{"other.example.com"},
			denied:         []string{"api.example.com", "unlisted.example.com"},
		},
	}

	for _, protocol := range []gatev1.ProtocolType{gatev1.HTTPProtocolType, gatev1.HTTPSProtocolType} {
		t.Run(string(protocol), func(t *testing.T) {
			for _, test := range testCases {
				t.Run(test.desc, func(t *testing.T) {
					t.Parallel()

					listeners := []gatev1.Listener{
						{Name: "target", Port: 80, Protocol: protocol, Hostname: test.hostname},
						{Name: "other", Port: 80, Protocol: protocol, Hostname: new(test.otherHostname)},
					}
					route := listenerIsolationRoute(kind, "route", "target", test.routeHostnames)
					conf, report := loadListenerIsolationConfiguration(t, listeners, route)
					handler := listenerIsolationHandler(t, conf, "web")

					statuses := report.httpRoutes
					if kind == kindGRPCRoute {
						statuses = report.grpcRoutes
					}
					status := statuses[ktypes.NamespacedName{Namespace: "default", Name: "route"}]
					require.Len(t, status.Parents, 1)
					assert.True(t, meta.IsStatusConditionTrue(status.Parents[0].Conditions, string(gatev1.RouteConditionAccepted)))
					gatewayStatus := report.gateways[ktypes.NamespacedName{Namespace: "default", Name: "gateway"}]
					assert.EqualValues(t, 1, gatewayStatus.Listeners[0].AttachedRoutes)
					assert.Zero(t, gatewayStatus.Listeners[1].AttachedRoutes)

					for _, host := range slices.Concat(test.allowed, test.denied) {
						t.Run(host, func(t *testing.T) {
							req := httptest.NewRequest(http.MethodGet, "http://"+host+"/service/method", http.NoBody)
							if protocol == gatev1.HTTPSProtocolType {
								req.TLS = &tls.ConnectionState{ServerName: req.URL.Hostname()}
							}
							response := httptest.NewRecorder()
							handler.ServeHTTP(response, req)
							want := http.StatusNotFound
							if slices.Contains(test.allowed, host) {
								want = http.StatusOK
							}
							assert.Equal(t, want, response.Code)
						})
					}
				})
			}
		})
	}
}

func TestHTTPSListenerMisdirectedRequests(t *testing.T) {
	for _, kind := range []string{kindHTTPRoute, kindGRPCRoute} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			listeners := []gatev1.Listener{
				{Name: "fallback", Port: 80, Protocol: gatev1.HTTPSProtocolType},
				{Name: "wildcard", Port: 80, Protocol: gatev1.HTTPSProtocolType, Hostname: new(gatev1.Hostname("*.example.com"))},
				{Name: "specific-wildcard", Port: 80, Protocol: gatev1.HTTPSProtocolType, Hostname: new(gatev1.Hostname("*.foo.example.com"))},
				{Name: "exact", Port: 80, Protocol: gatev1.HTTPSProtocolType, Hostname: new(gatev1.Hostname("api.foo.example.com"))},
			}
			var routes []runtime.Object
			for _, listener := range listeners {
				routes = append(routes, listenerIsolationRoute(kind, string(listener.Name), listener.Name, nil))
			}
			conf, _ := loadListenerIsolationConfiguration(t, listeners, routes...)
			handler := listenerIsolationHandler(t, conf, "web")

			testCases := []struct {
				sni    string
				host   string
				status int
			}{
				{sni: "example.com", host: "other.com", status: http.StatusOK},
				{sni: "foo.example.com", host: "bar.example.com", status: http.StatusOK},
				{sni: "bar.foo.example.com", host: "deep.bar.foo.example.com", status: http.StatusOK},
				{sni: "api.foo.example.com", host: "API.FOO.EXAMPLE.COM:443", status: http.StatusOK},
				{sni: "example.com", host: "foo.example.com", status: http.StatusMisdirectedRequest},
				{sni: "foo.example.com", host: "example.com", status: http.StatusMisdirectedRequest},
				{sni: "foo.example.com", host: "bar.foo.example.com", status: http.StatusMisdirectedRequest},
				{sni: "bar.foo.example.com", host: "foo.example.com", status: http.StatusMisdirectedRequest},
				{sni: "bar.foo.example.com", host: "api.foo.example.com", status: http.StatusMisdirectedRequest},
				{sni: "api.foo.example.com", host: "bar.foo.example.com", status: http.StatusMisdirectedRequest},
			}
			for _, test := range testCases {
				t.Run(test.sni+"/"+test.host, func(t *testing.T) {
					for _, path := range []string{"/service/method", "/unmatched"} {
						req := httptest.NewRequest(http.MethodGet, "https://"+test.host+path, http.NoBody)
						req.TLS = &tls.ConnectionState{ServerName: test.sni}
						response := httptest.NewRecorder()
						handler.ServeHTTP(response, req)
						want := test.status
						if path == "/unmatched" && want == http.StatusOK {
							want = http.StatusNotFound
						}
						assert.Equal(t, want, response.Code)
					}
				})
			}
		})
	}
}

func TestGatewayListenerIsolationFeatures(t *testing.T) {
	assert.Contains(t, SupportedFeatures(), features.SupportGatewayHTTPListenerIsolation)
	assert.Contains(t, SupportedFeatures(), features.FeatureName(features.SupportGatewayHTTPSListenerDetectMisdirectedRequests))

	_, report := loadListenerIsolationConfiguration(t, []gatev1.Listener{{Name: "http", Port: 80, Protocol: gatev1.HTTPProtocolType}})
	advertised := report.gatewayClasses["class"].SupportedFeatures
	assert.Contains(t, advertised, gatev1.SupportedFeature{Name: gatev1.FeatureName(features.SupportGatewayHTTPListenerIsolation)})
	assert.Contains(t, advertised, gatev1.SupportedFeature{Name: gatev1.FeatureName(features.SupportGatewayHTTPSListenerDetectMisdirectedRequests)})
}

func TestHTTPSListenerUnmatchedHost(t *testing.T) {
	listeners := []gatev1.Listener{
		{Name: "first", Port: 80, Protocol: gatev1.HTTPSProtocolType, Hostname: new(gatev1.Hostname("first.example.com"))},
		{Name: "second", Port: 80, Protocol: gatev1.HTTPSProtocolType, Hostname: new(gatev1.Hostname("second.example.com"))},
	}
	conf, _ := loadListenerIsolationConfiguration(t, listeners, listenerIsolationRoute(kindHTTPRoute, "route", "first", nil))
	handler := listenerIsolationHandler(t, conf, "web")
	for _, test := range []struct {
		sni    string
		host   string
		status int
	}{
		{sni: "first.example.com", host: "unknown.example.com", status: http.StatusNotFound},
		{sni: "first.example.com", host: "second.example.com", status: http.StatusMisdirectedRequest},
		{sni: "second.example.com", host: "second.example.com", status: http.StatusNotFound},
		{sni: "second.example.com", host: "first.example.com", status: http.StatusMisdirectedRequest},
	} {
		t.Run(test.sni+"/"+test.host, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://"+test.host+"/unmatched", http.NoBody)
			req.TLS = &tls.ConnectionState{ServerName: test.sni}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			assert.Equal(t, test.status, response.Code)
		})
	}
}

func TestListenerIsolationScope(t *testing.T) {
	for _, kind := range []string{kindHTTPRoute, kindGRPCRoute} {
		t.Run(kind, func(t *testing.T) {
			for _, scope := range []string{"port", "protocol", "gateway", "invalid listener"} {
				t.Run(scope, func(t *testing.T) {
					t.Parallel()

					listeners := []gatev1.Listener{{Name: "target", Port: 80, Protocol: gatev1.HTTPProtocolType}}
					other := gatev1.Listener{Name: "other", Port: 80, Protocol: gatev1.HTTPProtocolType, Hostname: new(gatev1.Hostname("api.example.com"))}
					objects := []runtime.Object{listenerIsolationRoute(kind, "route", "target", nil)}
					switch scope {
					case "port":
						other.Port = 8080
					case "protocol":
						other.Protocol = gatev1.HTTPSProtocolType
					case "gateway":
						objects = append(objects, &gatev1.Gateway{
							ObjectMeta: metav1.ObjectMeta{Name: "another-gateway", Namespace: "default"},
							Spec: gatev1.GatewaySpec{
								GatewayClassName: "class",
								Listeners:        []gatev1.Listener{other},
							},
						})
					case "invalid listener":
						other.AllowedRoutes = &gatev1.AllowedRoutes{Kinds: []gatev1.RouteGroupKind{{Kind: "TCPRoute"}}}
					}
					if scope != "gateway" {
						listeners = append(listeners, other)
					}
					conf, _ := loadListenerIsolationConfiguration(t, listeners, objects...)
					handler := listenerIsolationHandler(t, conf, "web")
					response := httptest.NewRecorder()
					handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://api.example.com/service/method", http.NoBody))
					assert.Equal(t, http.StatusOK, response.Code)
				})
			}
		})
	}
}

func TestListenerIsolationMultipleAttachments(t *testing.T) {
	for _, kind := range []string{kindHTTPRoute, kindGRPCRoute} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()

			listeners := []gatev1.Listener{
				{Name: "fallback", Port: 80, Protocol: gatev1.HTTPProtocolType},
				{Name: "wildcard", Port: 80, Protocol: gatev1.HTTPProtocolType, Hostname: new(gatev1.Hostname("*.example.com"))},
				{Name: "exact", Port: 80, Protocol: gatev1.HTTPProtocolType, Hostname: new(gatev1.Hostname("api.example.com"))},
			}
			route := listenerIsolationRoute(kind, "route", "", []gatev1.Hostname{"api.example.com", "other.example.com", "example.com"})
			switch route := route.(type) {
			case *gatev1.HTTPRoute:
				route.Spec.ParentRefs[0].SectionName = nil
			case *gatev1.GRPCRoute:
				route.Spec.ParentRefs[0].SectionName = nil
			}
			conf, report := loadListenerIsolationConfiguration(t, listeners, route)
			require.Len(t, conf.HTTP.Routers, 3)
			for _, listener := range report.gateways[ktypes.NamespacedName{Namespace: "default", Name: "gateway"}].Listeners {
				assert.EqualValues(t, 1, listener.AttachedRoutes)
			}

			parser, err := httpmuxer.NewSyntaxParser()
			require.NoError(t, err)
			for _, host := range []string{"api.example.com", "other.example.com", "example.com"} {
				t.Run(host, func(t *testing.T) {
					matched := 0
					for _, router := range conf.HTTP.Routers {
						mux := httpmuxer.NewMuxer(parser, nil)
						require.NoError(t, mux.AddRoute(router.Rule, router.RuleSyntax, router.Priority, ProviderName, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
							matched++
						})))
						requestdecorator.New(nil).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "http://"+host+"/service/method", http.NoBody), mux.ServeHTTP)
					}
					assert.Equal(t, 1, matched)
				})
			}
		})
	}
}

func listenerIsolationRoute(kind, name string, section gatev1.SectionName, hostnames []gatev1.Hostname) runtime.Object {
	metadata := metav1.ObjectMeta{Name: name, Namespace: "default"}
	common := gatev1.CommonRouteSpec{ParentRefs: []gatev1.ParentReference{{Name: "gateway", SectionName: new(section)}}}
	if kind == kindHTTPRoute {
		return &gatev1.HTTPRoute{
			ObjectMeta: metadata,
			Spec: gatev1.HTTPRouteSpec{
				CommonRouteSpec: common,
				Hostnames:       hostnames,
				Rules: []gatev1.HTTPRouteRule{{Matches: []gatev1.HTTPRouteMatch{{Path: &gatev1.HTTPPathMatch{
					Type: new(gatev1.PathMatchExact), Value: new("/service/method"),
				}}}}},
			},
		}
	}

	return &gatev1.GRPCRoute{
		ObjectMeta: metadata,
		Spec: gatev1.GRPCRouteSpec{
			CommonRouteSpec: common,
			Hostnames:       hostnames,
			Rules: []gatev1.GRPCRouteRule{{Matches: []gatev1.GRPCRouteMatch{{Method: &gatev1.GRPCMethodMatch{
				Type: new(gatev1.GRPCMethodMatchExact), Service: new("service"), Method: new("method"),
			}}}}},
		},
	}
}

func loadListenerIsolationConfiguration(t *testing.T, listeners []gatev1.Listener, routes ...runtime.Object) (*dynamic.Configuration, *statusReport) {
	t.Helper()

	for i := range listeners {
		if listeners[i].Protocol == gatev1.HTTPSProtocolType {
			listeners[i].TLS = &gatev1.ListenerTLSConfig{CertificateRefs: []gatev1.SecretObjectReference{{Name: "certificate", Kind: new(gatev1.Kind("Secret")), Group: new(gatev1.Group(""))}}}
		}
	}
	objects := []runtime.Object{
		&gatev1.GatewayClass{ObjectMeta: metav1.ObjectMeta{Name: "class"}, Spec: gatev1.GatewayClassSpec{ControllerName: controllerName}},
		&gatev1.Gateway{
			ObjectMeta: metav1.ObjectMeta{Name: "gateway", Namespace: "default"},
			Spec:       gatev1.GatewaySpec{GatewayClassName: "class", Listeners: listeners},
		},
	}
	objects = append(objects, routes...)
	client := newClientImpl(kubefake.NewClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "certificate", Namespace: "default"},
		Data:       map[string][]byte{corev1.TLSCertKey: []byte(listenerCert), corev1.TLSPrivateKeyKey: []byte(listenerKey)},
	}), newGatewaySimpleClientSet(t, objects...))
	_, err := client.WatchAll(nil, t.Context().Done())
	require.NoError(t, err)

	p := Provider{
		EntryPoints: map[string]Entrypoint{"web": {Address: ":80"}, "other": {Address: ":8080"}},
		client:      client,
	}
	conf, report, err := p.loadConfigurationFromGateways(t.Context())
	require.NoError(t, err)
	return conf, report
}

func listenerIsolationHandler(t *testing.T, conf *dynamic.Configuration, entryPoint string) http.Handler {
	t.Helper()

	parser, err := httpmuxer.NewSyntaxParser()
	require.NoError(t, err)
	conf = conf.DeepCopy()
	mux := httpmuxer.NewMuxer(parser, nil)
	for name, router := range conf.HTTP.Routers {
		if router.EntryPoints[0] != entryPoint {
			continue
		}
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		if strings.HasSuffix(name, "-fallback") {
			weighted := conf.HTTP.Services[router.Service].Weighted
			require.NotNil(t, weighted)
			require.Len(t, weighted.Services, 1)
			require.NotNil(t, weighted.Services[0].Status)
			assert.Equal(t, http.StatusNotFound, *weighted.Services[0].Status)
			handler = http.NotFoundHandler()
		}
		if router.TLS != nil {
			handler, err = snicheck.New(name, "", router.TLS.SNIHostRule, handler)
			require.NoError(t, err)
		}
		require.NoError(t, mux.AddRoute(router.Rule, router.RuleSyntax, router.Priority, ProviderName, handler))
	}
	decorator := requestdecorator.New(nil)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decorator.ServeHTTP(w, r, mux.ServeHTTP)
	})
}
