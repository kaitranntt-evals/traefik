// Run: companion of c2_typed_basicauth_newline_test.go — cp verify/repro/c2_export_verify.go pkg/provider/kubernetes/crd/ (exports the unexported provider helper to the probe; see that file for the test command)

//go:build verify

package crd

import (
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	corev1 "k8s.io/api/core/v1"
)

// VerifyLoadBasicAuthCredentials exposes the production helper that turns a kubernetes.io/basic-auth Secret into users and secret values.
func VerifyLoadBasicAuthCredentials(secret *corev1.Secret, secrets dynamic.Secrets) (dynamic.Users, error) {
	return loadBasicAuthCredentials(secret, secrets)
}
