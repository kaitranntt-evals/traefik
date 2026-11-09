package snicheck

import (
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/middlewares/requestdecorator"
	httpmuxer "github.com/traefik/traefik/v3/pkg/muxer/http"
	"github.com/traefik/traefik/v3/pkg/tcp"
)

// SNICheck is an HTTP handler that checks whether the TLS configuration for the server name is the same as for the host header.
type SNICheck struct {
	next           http.Handler
	routerName     string
	tlsOptionsName string
	sniMatcher     httpmuxer.MatcherFunc
}

// New creates a new SNICheck.
func New(routerName, tlsOptionsName, sniHostRule string, next http.Handler) (*SNICheck, error) {
	var sniMatcher httpmuxer.MatcherFunc
	if sniHostRule != "" {
		parser, err := httpmuxer.NewSyntaxParser()
		if err != nil {
			return nil, fmt.Errorf("creating SNI rule parser: %w", err)
		}
		sniMatcher, err = parser.ParseMatcher("v3", sniHostRule)
		if err != nil {
			return nil, fmt.Errorf("parsing SNI host rule: %w", err)
		}
	}

	return &SNICheck{
		next:           next,
		routerName:     routerName,
		tlsOptionsName: tlsOptionsName,
		sniMatcher:     sniMatcher,
	}, nil
}

func (s SNICheck) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if req.TLS == nil {
		s.next.ServeHTTP(rw, req)
		return
	}

	if s.sniMatcher != nil {
		// Match the TLS server name with the same normalization used for the request authority.
		sniReq := (&http.Request{Host: req.TLS.ServerName}).WithContext(req.Context())
		var matched bool
		requestdecorator.New(nil).ServeHTTP(rw, sniReq, func(_ http.ResponseWriter, r *http.Request) {
			matched = s.sniMatcher(r)
		})
		if !matched {
			log.Ctx(req.Context()).Debug().
				Str("routerName", s.routerName).
				Str("req.Host", req.Host).
				Str("req.TLS.ServerName", req.TLS.ServerName).
				Msg("SNI and request authority match different Gateway listeners")
			http.Error(rw, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
			return
		}
	}

	tlsOptionsNameUsed := tcp.GetTLSOptionsName(req.Context())
	if s.tlsOptionsName != tlsOptionsNameUsed {
		log.Debug().
			Str("routerName", s.routerName).
			Str("req.Host", req.Host).
			Str("req.TLS.ServerName", req.TLS.ServerName).
			Msgf("TLS options difference: SNI:%s, Header:%s", tlsOptionsNameUsed, s.tlsOptionsName)
		http.Error(rw, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
		return
	}

	s.next.ServeHTTP(rw, req)
}
