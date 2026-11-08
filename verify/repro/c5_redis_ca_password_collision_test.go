// Run: cp verify/repro/c5_redis_ca_password_collision_test.go pkg/server/middleware/verify_probe_c5_test.go && go test -tags verify -count=1 -run '^TestVerifyC5_' -v ./pkg/server/middleware (on branch evalon/traefik-se-120cc25c)
//go:build verify

package middleware

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/types"
)

func TestVerifyC5_RedisCAParseErrorDependsOnPassword(t *testing.T) {
	const passwordRef = "urn:k8s:secret:redis:password"

	results := map[string]string{}
	for _, password := range []string{"qxUnrelatedPassword", "CA", "config", "creating TLS config: failed to parse CA"} {
		configs := map[string]*runtime.MiddlewareInfo{
			"rl": {
				Middleware: &dynamic.Middleware{
					RateLimit: &dynamic.RateLimit{
						Average: 10,
						Burst:   1,
						Redis: &dynamic.Redis{
							Endpoints: []string{"localhost:6379"},
							Password:  passwordRef,
							TLS:       &types.ClientTLS{CA: "not-a-pem-certificate"},
						},
					},
					Secrets: dynamic.Secrets{passwordRef: password},
				},
			},
		}

		chain := NewBuilder(configs, nil, nil).BuildMiddlewareChain(context.Background(), []string{"rl"})
		_, err := chain.Then(http.NotFoundHandler())
		if err == nil {
			t.Fatalf("password %q: expected CA parse error", password)
		}

		stored := strings.Join(configs["rl"].Err, " | ")
		results[password] = stored
		t.Logf("password=%-45q runtime=%q", password, err.Error())
		t.Logf("password=%-45q stored =%q", password, stored)
	}

	baseline := results["qxUnrelatedPassword"]
	for password, stored := range results {
		if stored != baseline {
			t.Errorf("stored diagnostic changed solely because the password is %q:\n  baseline: %s\n  got     : %s", password, baseline, stored)
		}
	}
}
