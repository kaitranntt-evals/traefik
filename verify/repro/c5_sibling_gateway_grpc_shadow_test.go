//go:build verify

// Run: copy next to the eval fixtures in pkg/provider/kubernetes/gateway/ on branch evalon/traefik-ga-cbb7ae82, then
//   go test -tags verify -count=1 -run 'TestVerifyC5' -v ./pkg/provider/kubernetes/gateway/
package gateway_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/gateway"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

func TestVerifyC5(t *testing.T) {
	secret := gateway.EvalIsoTLSSecretExported(t, "supersecret")
	eps := map[string]gateway.Entrypoint{"websecure": {Address: ":443"}}
	grpcHdr := map[string]string{":method": http.MethodPost, "Content-Type": "application/grpc"}

	for _, strict := range []bool{false, true} {
		t.Run(map[bool]string{false: "strict=false", true: "strict=true"}[strict], func(t *testing.T) {
			// Two overlapping multi-listener Gateways: both own *.example.com and foo.example.com on port 443,
			// each with a distinct gRPC method per listener.
			conf := gateway.EvalIsoLoadExported(t, eps, []k8sruntime.Object{secret},
				gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
				gateway.EvalIsoHTTPSGatewayExported("gw-a", "supersecret", "wildcard=*.example.com", "exact=foo.example.com"),
				gateway.EvalIsoHTTPSGatewayExported("gw-b", "supersecret", "wildcard=*.example.com", "exact=foo.example.com"),
				gateway.EvalIsoGRPCRouteExported("grpc-a-wild", "gw-a", "wildcard", "SayAWild"),
				gateway.EvalIsoGRPCRouteExported("grpc-a-exact", "gw-a", "exact", "SayAExact"),
				gateway.EvalIsoGRPCRouteExported("grpc-b-wild", "gw-b", "wildcard", "SayBWild"),
				gateway.EvalIsoGRPCRouteExported("grpc-b-exact", "gw-b", "exact", "SayBExact"),
			)

			var names []string
			for n := range conf.HTTP.Routers {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				r := conf.HTTP.Routers[n]
				tlsOpts := "<nil>"
				if r.TLS != nil {
					tlsOpts = r.TLS.Options
				}
				t.Logf("ROUTER %s: prio=%d parents=%v eps=%v tlsOptions=%s svc=%s rule=%s", n, r.Priority, r.ParentRefs, r.EntryPoints, tlsOpts, r.Service, r.Rule)
			}

			serve := evalServeHTTPS(t, conf, "websecure", strict)
			for _, rq := range []struct{ sni, host, method string }{
				{"foo.example.com", "foo.example.com", "SayAExact"},
				{"foo.example.com", "foo.example.com", "SayBExact"},
				{"bar.example.com", "bar.example.com", "SayAWild"},
				{"bar.example.com", "bar.example.com", "SayBWild"},
				{"foo.example.com", "foo.example.com", "SayAWild"}, // exact listener must not fall through to wildcard
				{"bar.example.com", "foo.example.com", "SayAExact"}, // crossed SNI/Host
			} {
				code, body := serve(rq.sni, rq.host, "/helloworld.Greeter/"+rq.method, grpcHdr)
				t.Logf("REQ sni=%-16s host=%-16s method=%-10s -> %d %s", rq.sni, rq.host, rq.method, code, strings.TrimSpace(body))
			}
		})
	}
}

// TestVerifyC5_HTTP repeats the overlapping multi-listener scenario over plain HTTP, where no TLS
// options are involved, to isolate the parent-router capture mechanism.
func TestVerifyC5_HTTP(t *testing.T) {
	eps := map[string]gateway.Entrypoint{"web": {Address: ":80"}}
	grpcHdr := map[string]string{":method": http.MethodPost, "Content-Type": "application/grpc"}

	conf := gateway.EvalIsoLoadExported(t, eps, nil,
		gateway.EvalIsoWhoamiYAML, gateway.EvalIsoClassYAML,
		gateway.EvalIsoGatewayExported("gw-a", "wildcard=*.example.com", "exact=foo.example.com"),
		gateway.EvalIsoGatewayExported("gw-b", "wildcard=*.example.com", "exact=foo.example.com"),
		gateway.EvalIsoGRPCRouteExported("grpc-a-wild", "gw-a", "wildcard", "SayAWild"),
		gateway.EvalIsoGRPCRouteExported("grpc-a-exact", "gw-a", "exact", "SayAExact"),
		gateway.EvalIsoGRPCRouteExported("grpc-b-wild", "gw-b", "wildcard", "SayBWild"),
		gateway.EvalIsoGRPCRouteExported("grpc-b-exact", "gw-b", "exact", "SayBExact"),
	)

	var names []string
	for n := range conf.HTTP.Routers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		r := conf.HTTP.Routers[n]
		t.Logf("ROUTER %s: prio=%d parents=%v eps=%v svc=%s rule=%s", n, r.Priority, r.ParentRefs, r.EntryPoints, r.Service, r.Rule)
	}

	serve := evalServeHTTP(t, conf, "web")
	for _, rq := range []struct{ host, method string }{
		{"foo.example.com", "SayAExact"},
		{"foo.example.com", "SayBExact"},
		{"bar.example.com", "SayAWild"},
		{"bar.example.com", "SayBWild"},
		{"foo.example.com", "SayAWild"},
	} {
		code, body := serve(rq.host, "/helloworld.Greeter/"+rq.method, grpcHdr)
		t.Logf("REQ host=%-16s method=%-10s -> %d %s", rq.host, rq.method, code, strings.TrimSpace(body))
	}
}
