//go:build verify

// Run: cp verify/repro/c7_order_probe_test.go <worktree>/pkg/provider/kubernetes/ingress-nginx/verify_c7_order_probe_test.go && (cd <worktree> && go test -tags verify -count=1 -v -run '^TestVerify_C7' ./pkg/provider/kubernetes/ingress-nginx)

package ingressnginx

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestVerify_C7_AuthMapOrder calls the production auth-map helper repeatedly on the same Secret and
// compares the order it returns, WITHOUT sorting the result first.
func TestVerify_C7_AuthMapOrder(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "basic-auth", Namespace: "default"},
		Data: map[string][]byte{
			"alice": []byte("{SHA}W6ph5Mm5Pz8GgiULbPgzG37mj9g="),
			"bob":   []byte("$apr1$H6uskkkW$IgXLP6ewTrSuBkTrqE8wj/"),
			"carol": []byte("{SHA}W6ph5Mm5Pz8GgiULbPgzG37mj9g="),
		},
	}

	orders := map[string]int{}
	sorted := 0
	for range 200 {
		users, _, err := basicAuthUsers(secret, "auth-map")
		require.NoError(t, err)
		orders[strings.Join(users, " | ")]++
		if slices.IsSorted(users) {
			sorted++
		}
	}

	keys := make([]string, 0, len(orders))
	for order := range orders {
		keys = append(keys, order)
	}
	slices.Sort(keys)
	for _, order := range keys {
		t.Logf("RESULT order seen %3d/200: %s", orders[order], order)
	}
	t.Logf("RESULT distinct orders over 200 calls = %d; calls returning sorted order = %d/200", len(orders), sorted)

	assert.Len(t, orders, 1, "the same Secret must always publish the same user order")
	assert.Equal(t, 200, sorted, "the published user order must be the sorted one")
}
