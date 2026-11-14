// Shared in-package loader for the verify repros. Copy next to the provider sources (pkg/provider/kubernetes/gateway/).
package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	kscheme "k8s.io/client-go/kubernetes/scheme"
	gatev1 "sigs.k8s.io/gateway-api/apis/v1"
	gatefake "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned/fake"
)

// VerifyCert returns a self-signed certificate (PEM cert, PEM key) with the given common name.
func VerifyCert(t *testing.T, cn string, dnsNames ...string) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

// VerifySecret returns a kubernetes TLS secret holding a self-signed certificate.
func VerifySecret(t *testing.T, name, cn string, dnsNames ...string) runtime.Object {
	t.Helper()
	cert, key := VerifyCert(t, cn, dnsNames...)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Type:       corev1.SecretTypeTLS,
		Data:       map[string][]byte{corev1.TLSCertKey: cert, corev1.TLSPrivateKeyKey: key},
	}
}

// VerifyLoad runs the provider on the given documents and returns its configuration and a status summary.
func VerifyLoad(t *testing.T, entryPoints map[string]Entrypoint, extra []runtime.Object, docs ...string) (*dynamic.Configuration, []string) {
	t.Helper()
	require.NoError(t, gatev1.Install(kscheme.Scheme))

	k8sObjects := slices.Clone(extra)
	var gwObjects []runtime.Object
	for _, obj := range k8s.MustParseYaml([]byte(strings.Join(docs, "\n---\n"))) {
		if obj.GetObjectKind().GroupVersionKind().Group == gatev1.GroupName {
			gwObjects = append(gwObjects, obj)
			continue
		}
		k8sObjects = append(k8sObjects, obj)
	}

	gwClient := gatefake.NewSimpleClientset(gwObjects...)
	for _, obj := range gwObjects {
		if gw, ok := obj.(*gatev1.Gateway); ok {
			_, err := gwClient.GatewayV1().Gateways(gw.Namespace).Create(t.Context(), gw, metav1.CreateOptions{})
			require.NoError(t, err)
		}
	}

	client := newClientImpl(kubefake.NewClientset(k8sObjects...), gwClient)
	stop := make(chan struct{})
	t.Cleanup(func() { close(stop) })
	eventCh, err := client.WatchAll(nil, stop)
	require.NoError(t, err)
	select {
	case <-eventCh:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for informer sync")
	}

	p := Provider{EntryPoints: entryPoints, client: client}
	conf, report, err := p.loadConfigurationFromGateways(t.Context())
	require.NoError(t, err)

	var status []string
	conds := func(cs []metav1.Condition) string {
		var out []string
		for _, c := range cs {
			out = append(out, fmt.Sprintf("%s=%s(%s)", c.Type, c.Status, c.Reason))
		}
		sort.Strings(out)
		return strings.Join(out, " ")
	}
	for name, gw := range report.gateways {
		for _, l := range gw.Listeners {
			status = append(status, fmt.Sprintf("listener %s/%s attachedRoutes=%d %s", name.Name, l.Name, l.AttachedRoutes, conds(l.Conditions)))
		}
	}
	routes := map[string]map[string]gatev1.RouteStatus{"HTTPRoute": {}, "GRPCRoute": {}, "TLSRoute": {}}
	for n, s := range report.httpRoutes {
		routes["HTTPRoute"][n.Name] = s
	}
	for n, s := range report.grpcRoutes {
		routes["GRPCRoute"][n.Name] = s
	}
	for n, s := range report.tlsRoutes {
		routes["TLSRoute"][n.Name] = s
	}
	for kind, byName := range routes {
		for name, s := range byName {
			for _, parent := range s.Parents {
				section := ""
				if parent.ParentRef.SectionName != nil {
					section = string(*parent.ParentRef.SectionName)
				}
				status = append(status, fmt.Sprintf("%s %s parent=%s/%s %s", kind, name, parent.ParentRef.Name, section, conds(parent.Conditions)))
			}
		}
	}
	sort.Strings(status)
	return conf, status
}
