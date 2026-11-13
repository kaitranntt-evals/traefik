//go:build verify

// Run: instrumentation for c8_plugin_diagnostic_probe_test.go — cp this file to pkg/provider/kubernetes/crd/ (see the run line there).
package crd

import (
	"context"
	"os"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	traefikcrdfake "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/generated/clientset/versioned/fake"
	traefikv1alpha1 "github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd/traefikio/v1alpha1"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
)

// VerifyLoadFixtureWithSecretValue runs the unmodified production loader, Provider.loadConfigurationFromCRD, on a fixture whose Secrets have data[key] set to value.
func VerifyLoadFixtureWithSecretValue(ctx context.Context, file, key, value string) (*dynamic.Configuration, error) {
	if err := traefikv1alpha1.AddToScheme(kscheme.Scheme); err != nil {
		return nil, err
	}
	var k8sObjects, crdObjects []runtime.Object
	for _, file := range []string{file} {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, obj := range k8s.MustParseYaml(content) {
			switch obj.GetObjectKind().GroupVersionKind().Kind {
			case "Secret":
				obj.(*corev1.Secret).Data[key] = []byte(value)
				k8sObjects = append(k8sObjects, obj)
			case "Service", "Endpoints", "EndpointSlice", "Node", "ConfigMap":
				k8sObjects = append(k8sObjects, obj)
			default:
				crdObjects = append(crdObjects, obj)
			}
		}
	}
	client := newClientImpl(kubefake.NewClientset(k8sObjects...), traefikcrdfake.NewClientset(crdObjects...))
	stopCh := make(chan struct{})
	defer close(stopCh)
	eventCh, err := client.WatchAll(nil, stopCh)
	if err != nil {
		return nil, err
	}
	<-eventCh
	p := Provider{}
	return p.loadConfigurationFromCRD(ctx, client), nil
}
