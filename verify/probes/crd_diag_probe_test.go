// Run (on a worktree of evalon/traefik-se-df0ea15d): cp verify/probes/crd_diag_*_test.go <worktree>/pkg/provider/kubernetes/crd/ && cd <worktree> && go test -race -count=1 -v -run '^TestVerifyCRDDiag' ./pkg/provider/kubernetes/crd
package crd_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	"github.com/traefik/traefik/v3/pkg/server/middleware"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerifyCRDDiag(t *testing.T) {
	const marker = "CONFIDENTIAL"
	build := func(digest bool, content string) (string, string) {
		spec := traefikv1alpha1.MiddlewareSpec{BasicAuth: &traefikv1alpha1.BasicAuth{Secret: "users"}}
		if digest {
			spec = traefikv1alpha1.MiddlewareSpec{DigestAuth: &traefikv1alpha1.DigestAuth{Secret: "users"}}
		}
		conf := crd.VerifyPublish(t,
			&traefikv1alpha1.Middleware{ObjectMeta: metav1.ObjectMeta{Name: "auth", Namespace: "default"}, Spec: spec},
			&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "users", Namespace: "default"}, Data: map[string][]byte{"users": []byte(content)}})
		public, _ := json.Marshal(conf.HTTP.Middlewares["default-auth"])
		name := "default-auth@kubernetescrd"
		rt := map[string]*runtime.MiddlewareInfo{name: {Middleware: conf.HTTP.Middlewares["default-auth"]}}
		_, err := middleware.NewBuilder(rt, nil, nil).BuildMiddlewareChain(t.Context(), []string{name}).
			Then(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		if err == nil {
			return string(public), "<nil>"
		}
		return string(public), err.Error()
	}

	for _, digest := range []bool{false, true} {
		kind, valid := "BasicAuth", []string{"alice:$apr1$aaaa$bbbb", "bob:$apr1$cccc$dddd"}
		if digest {
			kind, valid = "DigestAuth", []string{"alice:traefik:0123456789abcdef0123456789abcdef", "bob:traefik:fedcba9876543210fedcba9876543210"}
		}
		for _, idx := range []int{0, 1, 2} {
			public, err := build(digest, strings.Join(slices.Insert(slices.Clone(valid), idx, marker+"malformed"), "\n")+"\n")
			t.Logf("%s malformed Secret entry at index %d: public=%s err=%q leaks=%v", kind, idx, public, err, strings.Contains(err+public, marker))
		}
		public, err := build(digest, "urn:k8s:secret:"+marker+"name:key\n")
		t.Logf("%s Secret entry starting with the URN prefix: public=%s err=%q leaks=%v", kind, public, err, strings.Contains(err+public, marker))
		public, err = build(digest, strings.Join(valid, "\n")+"\n")
		t.Logf("%s valid control: public=%s err=%q", kind, public, err)
	}
}
