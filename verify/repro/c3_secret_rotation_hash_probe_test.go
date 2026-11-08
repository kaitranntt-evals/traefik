// Run: cp verify/repro/c3_secret_rotation_hash_probe_test.go pkg/provider/kubernetes/crd/verify_probe_c3_test.go && go test -tags verify -count=1 -run '^TestVerifyC3_' -v ./pkg/provider/kubernetes/crd (on branches evalon/traefik-se-c89bfec6 and evalon/traefik-se-2839e93a, before and after adding `hash:"ignore"` to dynamic.Middleware.Secrets)
//go:build verify

package crd

import (
	"reflect"
	"testing"
	"time"

	"github.com/mitchellh/hashstructure"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// TestVerifyC3_SecretRotationHash reports whether rotating a Secret changes the value the
// CRD provider uses for change detection (hashstructure.Hash in Provider.Provide).
func TestVerifyC3_SecretRotationHash(t *testing.T) {
	k8sObjects, crdObjects := readResources(t, []string{"services.yml", "with_auth.yml"})
	kubeClient := kubefake.NewClientset(k8sObjects...)
	client := newClientImpl(kubeClient, traefikcrdfake.NewClientset(crdObjects...))
	eventCh, err := client.WatchAll(nil, t.Context().Done())
	if err != nil {
		t.Fatal(err)
	}
	<-eventCh

	p := &Provider{}
	if err := p.Init(); err != nil {
		t.Fatal(err)
	}

	before := p.loadConfigurationFromCRD(t.Context(), client)
	hashBefore, err := hashstructure.Hash(before, nil)
	if err != nil {
		t.Fatal(err)
	}

	secret, err := kubeClient.CoreV1().Secrets("default").Get(t.Context(), "authsecret", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	secret.ResourceVersion = "2"
	secret.Data["users"] = []byte("rotated:$apr1$rotated$rotatedhash\n")
	if _, err := kubeClient.CoreV1().Secrets("default").Update(t.Context(), secret, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		current, ok, err := client.GetSecret("default", "authsecret")
		if err == nil && ok && current.ResourceVersion == "2" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("secret update not observed")
		}
		time.Sleep(time.Millisecond)
	}

	after := p.loadConfigurationFromCRD(t.Context(), client)
	hashAfter, err := hashstructure.Hash(after, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("reflect.DeepEqual(before, after) = %v", reflect.DeepEqual(before, after))
	t.Logf("hashstructure.Hash before=%d after=%d changed=%v", hashBefore, hashAfter, hashBefore != hashAfter)
	if hashBefore == hashAfter {
		t.Errorf("Secret rotation did NOT change the provider change-detection hash: Provider.Provide would skip the reload")
	}
}
