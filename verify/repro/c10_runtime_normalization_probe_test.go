//go:build ignore

// Run: in a checkout of evalon/traefik-se-dfb43954: sed '/^\/\/go:build ignore$/d' verify/repro/c10_runtime_normalization_probe_test.go > pkg/server/middleware/c10_runtime_normalization_probe_test.go && go test -count=1 -v -run 'TestProbe_C10' ./pkg/server/middleware
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/testhelpers"
)

func TestProbe_C10_RuntimeNormalization(t *testing.T) {
	const urn = "urn:k8s:secret:ba:auth"
	for _, tc := range []struct{ desc, value string }{
		{"padded, no newline", "  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  "},
		{"padded, newline-terminated", "  test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/  \n"},
		{"unpadded, no newline", "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"},
	} {
		mid := &dynamic.Middleware{BasicAuth: &dynamic.BasicAuth{Users: dynamic.Users{urn}}, Secrets: map[string]string{urn: tc.value}}
		rtConf := runtime.NewConfig(dynamic.Configuration{HTTP: &dynamic.HTTPConfiguration{Middlewares: map[string]*dynamic.Middleware{"m": mid}}})
		b := NewBuilder(rtConf.Middlewares, nil, nil)
		h, err := b.BuildMiddlewareChain(t.Context(), []string{"m"}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) { rw.WriteHeader(http.StatusOK) }))
		require.NoError(t, err)
		req := testhelpers.MustNewRequest(http.MethodGet, "http://localhost", nil)
		req.SetBasicAuth("test", "test")
		rw := httptest.NewRecorder()
		h.ServeHTTP(rw, req)
		t.Logf("[%s] value=%q -> auth test/test status=%d", tc.desc, tc.value, rw.Code)
	}
}
