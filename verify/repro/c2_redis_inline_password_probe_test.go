// Run (portable across base, control and claim branch): cp verify/repro/c2_redis_inline_password_probe_test.go <worktree>/pkg/server/middleware/ && (cd <worktree> && go test -tags verify -count=1 -v -run '^TestVerifyC2_RedisInlineURNPrefixedPassword$' ./pkg/server/middleware)

//go:build verify

package middleware

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	ptypes "github.com/traefik/paerser/types"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
)

// verifyC2FakeRedis records every RESP command it receives; HELLO is refused so that the client falls back to AUTH,
// and the rate limiting script always allows the request.
type verifyC2FakeRedis struct {
	ln net.Listener

	mu       sync.Mutex
	commands [][]string
}

func newVerifyC2FakeRedis(t *testing.T) *verifyC2FakeRedis {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	f := &verifyC2FakeRedis{ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn)
		}
	}()

	return f
}

func (f *verifyC2FakeRedis) serve(conn net.Conn) {
	defer conn.Close()

	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadString('\n')
		if err != nil || !strings.HasPrefix(line, "*") {
			return
		}
		n, _ := strconv.Atoi(strings.TrimSpace(line[1:]))

		args := make([]string, 0, n)
		for range n {
			head, err := r.ReadString('\n')
			if err != nil {
				return
			}
			size, _ := strconv.Atoi(strings.TrimSpace(head[1:]))
			buf := make([]byte, size+2)
			if _, err := io.ReadFull(r, buf); err != nil {
				return
			}
			args = append(args, string(buf[:size]))
		}

		f.mu.Lock()
		f.commands = append(f.commands, args)
		f.mu.Unlock()

		reply := "+OK\r\n"
		switch {
		case len(args) > 0 && strings.EqualFold(args[0], "hello"):
			reply = "-ERR unknown command 'HELLO'\r\n"
		case len(args) > 0 && (strings.EqualFold(args[0], "evalsha") || strings.EqualFold(args[0], "eval")):
			// The token bucket script answers {allowed, delay}.
			reply = "*2\r\n$4\r\ntrue\r\n$1\r\n0\r\n"
		}
		if _, err := conn.Write([]byte(reply)); err != nil {
			return
		}
	}
}

func (f *verifyC2FakeRedis) authCommands() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var auth [][]string
	for _, c := range f.commands {
		if len(c) > 0 && (strings.EqualFold(c[0], "auth") || strings.EqualFold(c[0], "hello")) {
			auth = append(auth, c)
		}
	}
	return auth
}

func TestVerifyC2_RedisInlineURNPrefixedPassword(t *testing.T) {
	// A literal password written inline (file/HTTP/label providers): nothing was loaded from a Kubernetes Secret.
	const inlinePassword = "urn:k8s:secret:this-is-my-literal-redis-password"

	fake := newVerifyC2FakeRedis(t)
	timeout := ptypes.Duration(2 * time.Second)

	rtConf := runtime.NewConfig(dynamic.Configuration{
		HTTP: &dynamic.HTTPConfiguration{
			Middlewares: map[string]*dynamic.Middleware{
				"rl": {
					RateLimit: &dynamic.RateLimit{
						Average: 100,
						Burst:   10,
						Redis: &dynamic.Redis{
							Endpoints:    []string{fake.ln.Addr().String()},
							Password:     inlinePassword,
							DialTimeout:  &timeout,
							ReadTimeout:  &timeout,
							WriteTimeout: &timeout,
						},
					},
				},
			},
		},
	})

	builder := NewBuilder(rtConf.Middlewares, nil, nil)

	// Construction path, step 1: buildConstructor.
	constructor, constructorErr := builder.buildConstructor(t.Context(), "rl")
	fmt.Printf("C2 buildConstructor error: %v\n", constructorErr)

	var newErr error
	if constructorErr == nil {
		_, newErr = constructor(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		fmt.Printf("C2 constructor(next) [ratelimiter.New -> newRedisLimiter] error: %v\n", newErr)
	}

	// Construction path, step 2: the chain a router builds, which records the error shown by the API.
	handler, chainErr := builder.BuildMiddlewareChain(t.Context(), []string{"rl"}).Then(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.WriteHeader(http.StatusOK)
	}))
	fmt.Printf("C2 BuildMiddlewareChain(...).Then error: %v\n", chainErr)
	fmt.Printf("C2 runtime MiddlewareInfo: status=%q err=%q\n", rtConf.Middlewares["rl"].Status, rtConf.Middlewares["rl"].Err)
	fmt.Printf("C2 configuration password after build: %q\n", rtConf.Middlewares["rl"].RateLimit.Redis.Password)

	if chainErr == nil {
		req := httptest.NewRequest(http.MethodGet, "http://localhost/", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		fmt.Printf("C2 request through the limiter: HTTP %d\n", rec.Code)
	}

	auth := fake.authCommands()
	fmt.Printf("C2 credentials received by the Redis server (client options in effect): %q\n", auth)

	require.NoError(t, constructorErr, "buildConstructor must accept a literal inline password")
	require.NoError(t, newErr)
	require.NoError(t, chainErr, "the middleware chain must build")
	assert.Empty(t, rtConf.Middlewares["rl"].Err)

	var sawLiteral bool
	for _, c := range auth {
		if c[len(c)-1] == inlinePassword {
			sawLiteral = true
		}
	}
	assert.True(t, sawLiteral, "the Redis client must authenticate with the literal inline password")
}
