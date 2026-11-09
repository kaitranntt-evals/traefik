// Run: copy to pkg/middlewares/gatewayapi/misdirected/ on evalon/traefik-ga-968ccc91, then `go test -tags verify -count=1 -v -run TestVerifyC2Classification ./pkg/middlewares/gatewayapi/misdirected/`.

//go:build verify

package misdirected

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
)

// TestVerifyC2Classification prints, for each Host header, the listener the misdirected-request
// middleware classifies the Host and the SNI into, and whether the production HTTP `Host` matcher
// of the exact listener's routers matches the same request.
func TestVerifyC2Classification(t *testing.T) {
	// Listener hostnames exactly as loadMisdirectedRequestDetection emits them for a Gateway with
	// a hostname-less listener and an `abc.example.com` listener on one entry point.
	conf := dynamic.MisdirectedRequest{Hostnames: []string{"*", "abc.example.com"}}

	parser, err := httpmuxer.NewSyntaxParser()
	if err != nil {
		t.Fatal(err)
	}

	for _, sni := range []string{"xyz.example.com", "abc.example.com"} {
		for _, host := range []string{"abc.example.com", "abc.example.com.", "ABC.Example.COM.:443", "xyz.example.com", "xyz.example.com."} {
			var (
				canonical, hostListener, sniListener string
				exactMatcher, reached                bool
			)

			next := http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
				reached = true
				rw.WriteHeader(http.StatusOK)
			})
			mw := NewMisdirectedRequest(t.Context(), next, conf, "probe").(*misdirectedRequest)

			// The rule the provider builds for the routes of the exact listener.
			exactMux := httpmuxer.NewMuxer(parser, nil)
			if err := exactMux.AddRoute("Host(`abc.example.com`)", "default", 0, "probe", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				exactMatcher = true
			})); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodGet, "https://"+host+"/", nil)
			req.TLS = &tls.ConnectionState{ServerName: sni}

			rec := httptest.NewRecorder()
			// requestdecorator is the entry point middleware storing types.CanonicalDomain(Host) in the context.
			requestdecorator.New(nil).ServeHTTP(rec, req, func(rw http.ResponseWriter, r *http.Request) {
				canonical = requestdecorator.GetCanonicalHost(r.Context())
				hostListener, _ = mw.matchListener(requestHost(r))
				sniListener, _ = mw.matchListener(sni)
				exactMux.ServeHTTP(httptest.NewRecorder(), r)
				mw.ServeHTTP(rw, r)
			})

			t.Logf("VERIFY-C2 SNI=%-16s Host=%-22q canonicalHost=%-20q middleware{hostListener=%-17q sniListener=%-17q status=%d nextCalled=%-5t} HostMatcher(`abc.example.com`)=%t",
				sni, host, canonical, hostListener, sniListener, rec.Code, reached, exactMatcher)
		}
	}
}
