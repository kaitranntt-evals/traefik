//go:build verify

// Run (branch evalon/traefik-se-22e29d38): cp verify/repro/c7_collision_warning_test.go <worktree>/pkg/provider/kubernetes/crd/zz_verify_c7_test.go && go test -tags verify -count=1 -run TestVerifyC7 -v ./pkg/provider/kubernetes/crd/
package crd

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// The legacy naming flattens namespace "a-b"/name "c" and namespace "a"/name "b-c" to the same identifier "a-b-c".
func TestVerifyC7(t *testing.T) {
	const users = "test:$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/\n"

	secret := func(ns, value string) runtime.Object {
		return &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "authsecret", Namespace: ns},
			Data:       map[string][]byte{"users": []byte(value)},
		}
	}
	mw := func(ns, name string, spec traefikv1alpha1.MiddlewareSpec) runtime.Object {
		return &traefikv1alpha1.Middleware{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Spec: spec}
	}
	basicAuth := traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "authsecret"}}
	addPrefix := traefikv1alpha1.MiddlewareSpec{AddPrefix: &dynamic.AddPrefix{Prefix: "/x"}}

	cases := []struct {
		name string
		k8s  []runtime.Object
		crd  []runtime.Object
	}{
		{
			name: "secret-backed equivalent pair (same users in both Secrets)",
			k8s:  []runtime.Object{secret("a-b", users), secret("a", users)},
			crd:  []runtime.Object{mw("a-b", "c", basicAuth), mw("a", "b-c", basicAuth)},
		},
		{
			name: "CONTROL equivalent pair without Secret",
			crd:  []runtime.Object{mw("a-b", "c", addPrefix), mw("a", "b-c", addPrefix)},
		},
		{
			name: "CONTROL secret-backed pair with different Secret values",
			k8s:  []runtime.Object{secret("a-b", users), secret("a", "other:$apr1$d9hr9HBB$4HxwgUir3HP4EsggP/QNo0\n")},
			crd:  []runtime.Object{mw("a-b", "c", basicAuth), mw("a", "b-c", basicAuth)},
		},
	}

	for _, test := range cases {
		fmt.Printf("C7 === %s\n", test.name)

		// Each resource loaded alone gives the comparison inputs once initialization is complete.
		var alone []*dynamic.Middleware
		for _, obj := range test.crd {
			conf, _ := verifyC7Load(t, test.k8s, []runtime.Object{obj})
			require.Len(t, conf.HTTP.Middlewares, 1)
			m := conf.HTTP.Middlewares["a-b-c"]
			require.NotNil(t, m)
			fmt.Printf("C7   loaded alone: %s/%s -> id a-b-c basicAuth=%+v addPrefix=%+v bindings=%d\n",
				obj.(*traefikv1alpha1.Middleware).Namespace, obj.(*traefikv1alpha1.Middleware).Name, m.BasicAuth, m.AddPrefix, len(m.Secrets))
			alone = append(alone, m)
		}
		fmt.Printf("C7   complete objects DeepEqual=%v\n", reflect.DeepEqual(alone[0], alone[1]))

		conf, logs := verifyC7Load(t, test.k8s, test.crd)
		fmt.Printf("C7   loaded together: middlewares=%d collisionWarnings=%d\n", len(conf.HTTP.Middlewares), strings.Count(logs, "Name collision"))
		for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
			if strings.Contains(line, "Name collision") {
				fmt.Printf("C7   LOG %s\n", line)
			}
		}
	}
}

func verifyC7Load(t *testing.T, k8sObjects, crdObjects []runtime.Object) (*dynamic.Configuration, string) {
	t.Helper()

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), traefikcrdfake.NewClientset(crdObjects...))
	eventCh, err := client.WatchAll(nil, make(chan struct{}))
	require.NoError(t, err)
	<-eventCh

	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	p := Provider{}
	return p.loadConfigurationFromCRD(logger.WithContext(t.Context()), client), buf.String()
}
