// Run: sh verify/repro/C10_replaced_bindings.sh (it copies this file into pkg/provider/kubernetes/crd/ of evalon/traefik-se-9fafd9a4).
package crd

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// VerifyHarness drives the real CRD provider loader (loadConfigurationFromCRD) against fake clientsets,
// and lets a test change Kubernetes objects between two loads, as Kubernetes events do in Provide.
type VerifyHarness struct {
	Kube   *kubefake.Clientset
	CRD    *traefikcrdfake.Clientset
	client *clientWrapper
	p      *Provider
}

func NewVerifyHarness(t *testing.T, paths []string, extra ...runtime.Object) *VerifyHarness {
	t.Helper()

	k8sObjects, crdObjects := readResources(t, paths)
	for _, obj := range extra {
		switch o := obj.(type) {
		case *corev1.Secret:
			replaced := false
			for i, existing := range k8sObjects {
				if s, ok := existing.(*corev1.Secret); ok && s.Name == o.Name && s.Namespace == o.Namespace {
					k8sObjects[i] = o
					replaced = true
				}
			}
			if !replaced {
				k8sObjects = append(k8sObjects, o)
			}
		case *traefikv1alpha1.Middleware:
			crdObjects = append(crdObjects, o)
		default:
			k8sObjects = append(k8sObjects, o)
		}
	}

	h := &VerifyHarness{Kube: kubefake.NewClientset(k8sObjects...), CRD: traefikcrdfake.NewSimpleClientset(crdObjects...)}
	h.client = newClientImpl(h.Kube, h.CRD)

	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	eventCh, err := h.client.WatchAll(nil, stopCh)
	require.NoError(t, err)
	<-eventCh

	h.p = &Provider{AllowCrossNamespace: true}
	require.NoError(t, h.p.Init())

	return h
}

// Load runs the production loader, exactly as Provider.Provide does on each Kubernetes event.
func (h *VerifyHarness) Load(ctx context.Context) *dynamic.Configuration {
	return h.p.loadConfigurationFromCRD(ctx, h.client)
}

func (h *VerifyHarness) SetSecret(t *testing.T, ns, name string, data map[string][]byte) {
	t.Helper()

	sec, err := h.Kube.CoreV1().Secrets(ns).Get(context.Background(), name, metav1.GetOptions{})
	if err == nil {
		sec = sec.DeepCopy()
		sec.Data = data
		_, err = h.Kube.CoreV1().Secrets(ns).Update(context.Background(), sec, metav1.UpdateOptions{})
	} else {
		_, err = h.Kube.CoreV1().Secrets(ns).Create(context.Background(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}, Data: data}, metav1.CreateOptions{})
	}
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		cached, found, err := h.client.GetSecret(ns, name)
		if err != nil || !found || cached == nil || len(cached.Data) != len(data) {
			return false
		}
		for k, v := range data {
			if string(cached.Data[k]) != string(v) {
				return false
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond)
}

func (h *VerifyHarness) DeleteSecret(t *testing.T, ns, name string) {
	t.Helper()

	require.NoError(t, h.Kube.CoreV1().Secrets(ns).Delete(context.Background(), name, metav1.DeleteOptions{}))
	require.Eventually(t, func() bool {
		_, found, _ := h.client.GetSecret(ns, name)
		return !found
	}, 5*time.Second, 5*time.Millisecond)
}

// UpdateMiddleware applies mutate to the named Middleware CRD and waits for the informer cache to observe it.
func (h *VerifyHarness) UpdateMiddleware(t *testing.T, ns, name string, mutate func(*traefikv1alpha1.Middleware), observed func(*traefikv1alpha1.Middleware) bool) {
	t.Helper()

	mw, err := h.CRD.TraefikV1alpha1().Middlewares(ns).Get(context.Background(), name, metav1.GetOptions{})
	require.NoError(t, err)
	mw = mw.DeepCopy()
	mutate(mw)
	_, err = h.CRD.TraefikV1alpha1().Middlewares(ns).Update(context.Background(), mw, metav1.UpdateOptions{})
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		for _, cached := range h.client.GetMiddlewares() {
			if cached.Namespace == ns && cached.Name == name {
				return observed(cached)
			}
		}
		return false
	}, 5*time.Second, 5*time.Millisecond)
}

func (h *VerifyHarness) DeleteMiddleware(t *testing.T, ns, name string) {
	t.Helper()

	require.NoError(t, h.CRD.TraefikV1alpha1().Middlewares(ns).Delete(context.Background(), name, metav1.DeleteOptions{}))
	require.Eventually(t, func() bool {
		for _, cached := range h.client.GetMiddlewares() {
			if cached.Namespace == ns && cached.Name == name {
				return false
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond)
}
