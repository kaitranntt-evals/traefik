// Run: git worktree add --detach ../wt-32a38a91 <evalon/traefik-se-32a38a91>; cp verify/repro/c1_reference_key_probe_test.go ../wt-32a38a91/pkg/provider/kubernetes/crd/ && (cd ../wt-32a38a91 && go test -tags verify -count=1 -v -run '^TestVerifyC1_' ./pkg/provider/kubernetes/crd)

//go:build verify

package crd

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

const verifyC1Prefix = "urn:k8s:secret:"

func verifyC1Secret(name string, data map[string]string) *corev1.Secret {
	s := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Data: map[string][]byte{}}
	for k, v := range data {
		s.Data[k] = []byte(v)
	}
	return s
}

func verifyC1Load(t *testing.T, secrets []*corev1.Secret, mids []*traefikv1alpha1.Middleware) *dynamic.Configuration {
	t.Helper()

	var k8sObjects, crdObjects []runtime.Object
	for _, s := range secrets {
		k8sObjects = append(k8sObjects, s)
	}
	for _, m := range mids {
		crdObjects = append(crdObjects, m)
	}

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), traefikcrdfake.NewClientset(crdObjects...))
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	p := Provider{AllowCrossNamespace: true}
	require.NoError(t, p.Init())
	return p.loadConfigurationFromCRD(t.Context(), client)
}

// verifyC1Bound returns the value bound to ref in the middleware's unexported-from-JSON secret store (any map[string]string field).
func verifyC1Bound(m *dynamic.Middleware, ref string) (string, bool) {
	v := reflect.ValueOf(m).Elem()
	for i := range v.NumField() {
		if bindings, ok := v.Field(i).Interface().(map[string]string); ok {
			val, found := bindings[ref]
			return val, found
		}
	}
	return "", false
}

// verifyC1SelectedKeys lists the Secret data keys whose value contains the value the loader actually bound to ref.
func verifyC1SelectedKeys(s *corev1.Secret, bound string) []string {
	var keys []string
	for k, v := range s.Data {
		if strings.Contains(string(v), bound) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func TestVerifyC1_ReferenceKeyVersusLoaderSelectedKey(t *testing.T) {
	mid := func(name string, spec traefikv1alpha1.MiddlewareSpec) *traefikv1alpha1.Middleware {
		return &traefikv1alpha1.Middleware{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}, Spec: spec}
	}
	fwd := func(ca string) traefikv1alpha1.MiddlewareSpec {
		return traefikv1alpha1.MiddlewareSpec{ForwardAuth: &traefikv1alpha1.ForwardAuth{Address: "https://auth.example", TLS: &traefikv1alpha1.ClientTLSWithCAOptional{ClientTLS: traefikv1alpha1.ClientTLS{CASecret: ca}}}}
	}
	redis := func(ca string) traefikv1alpha1.MiddlewareSpec {
		return traefikv1alpha1.MiddlewareSpec{RateLimit: &traefikv1alpha1.RateLimit{Redis: &traefikv1alpha1.Redis{Endpoints: []string{"127.0.0.1:6379"}, TLS: &traefikv1alpha1.ClientTLS{CASecret: ca}}}}
	}

	secrets := []*corev1.Secret{
		verifyC1Secret("basic-auth-key", map[string]string{"auth": "alice:$apr1$aaaaaaaa$AAAAAAAAAAAAAAAAAAAAAA\nbob:$apr1$bbbbbbbb$BBBBBBBBBBBBBBBBBBBBBB\n"}),
		verifyC1Secret("basic-users-key", map[string]string{"users": "carol:$apr1$cccccccc$CCCCCCCCCCCCCCCCCCCCCC\n"}),
		verifyC1Secret("digest-htdigest-key", map[string]string{"htdigest": "dave:traefik:0123456789abcdef0123456789abcdef\n"}),
		verifyC1Secret("ca-cacrt", map[string]string{"ca.crt": "PEM-FROM-ca.crt"}),
		verifyC1Secret("ca-tlsca", map[string]string{"tls.ca": "PEM-FROM-tls.ca"}),
		verifyC1Secret("ca-custom", map[string]string{"corp-root.pem": "PEM-FROM-corp-root.pem"}),
		verifyC1Secret("ca-both", map[string]string{"tls.ca": "PEM-FROM-tls.ca-IN-BOTH", "ca.crt": "PEM-FROM-ca.crt-IN-BOTH"}),
	}
	byName := map[string]*corev1.Secret{}
	for _, s := range secrets {
		byName[s.Name] = s
	}

	mids := []*traefikv1alpha1.Middleware{
		mid("basic-auth-key", traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "basic-auth-key"}}),
		mid("basic-users-key", traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "basic-users-key"}}),
		mid("digest-htdigest-key", traefikv1alpha1.MiddlewareSpec{DigestAuth: &traefikv1alpha1.DigestAuth{Secret: "digest-htdigest-key"}}),
		mid("fwd-ca-cacrt", fwd("ca-cacrt")),
		mid("fwd-ca-tlsca", fwd("ca-tlsca")),
		mid("fwd-ca-custom", fwd("ca-custom")),
		mid("fwd-ca-both", fwd("ca-both")),
		mid("redis-ca-tlsca", redis("ca-tlsca")),
		mid("redis-ca-custom", redis("ca-custom")),
		mid("redis-ca-both", redis("ca-both")),
	}

	conf := verifyC1Load(t, secrets, mids)

	type row struct{ mw, secret, ref string }
	var rows []row
	for _, m := range mids {
		got := conf.HTTP.Middlewares["default-"+m.Name]
		require.NotNil(t, got, m.Name)
		switch {
		case got.BasicAuth != nil:
			for _, u := range got.BasicAuth.Users {
				rows = append(rows, row{m.Name, m.Spec.BasicAuth.Secret, u})
			}
		case got.DigestAuth != nil:
			for _, u := range got.DigestAuth.Users {
				rows = append(rows, row{m.Name, m.Spec.DigestAuth.Secret, u})
			}
		case got.ForwardAuth != nil:
			rows = append(rows, row{m.Name, m.Spec.ForwardAuth.TLS.CASecret, got.ForwardAuth.TLS.CA})
		case got.RateLimit != nil:
			rows = append(rows, row{m.Name, m.Spec.RateLimit.Redis.TLS.CASecret, got.RateLimit.Redis.TLS.CA})
		}
	}

	fmt.Printf("%-20s | %-34s | %-48s | %-13s | %-24s | %s\n", "middleware", "Secret.Data keys", "reference in routing configuration", "ref key", "loader-selected key", "same?")
	for _, r := range rows {
		s := byName[r.secret]
		var dataKeys []string
		for k := range s.Data {
			dataKeys = append(dataKeys, k)
		}
		sort.Strings(dataKeys)

		if !strings.HasPrefix(r.ref, verifyC1Prefix) {
			fmt.Printf("%-20s | %-34s | %-48q | %-13s | %-24s | %s\n", r.mw, strings.Join(dataKeys, ","), r.ref, "(inline)", "-", "n/a")
			continue
		}

		refKey := strings.TrimPrefix(r.ref, verifyC1Prefix+r.secret+":")
		// Implementations that do not key their bindings by reference (the control branch) are only probed with single-key Secrets.
		selected, boundNote := dataKeys, ""
		if bound, ok := verifyC1Bound(conf.HTTP.Middlewares["default-"+r.mw], r.ref); ok {
			selected = verifyC1SelectedKeys(s, bound)
			boundNote = fmt.Sprintf(", bound value: %q", strings.SplitN(bound, ":", 2)[0])
		} else {
			require.Len(t, dataKeys, 1, "no binding keyed by %s and the Secret has several keys", r.ref)
		}
		_, refKeyExists := s.Data[refKey]

		same := len(selected) == 1 && selected[0] == refKey
		fmt.Printf("%-20s | %-34s | %-48s | %-13s | %-24s | %v (ref key exists in Secret: %v%s)\n", r.mw, strings.Join(dataKeys, ","), r.ref, refKey, strings.Join(selected, ","), same, refKeyExists, boundNote)

		assert.True(t, same, "%s: reference %q names key %q but the loader selected key %v", r.mw, r.ref, refKey, selected)
	}
}
