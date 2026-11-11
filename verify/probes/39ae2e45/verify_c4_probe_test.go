// Audit instrumentation (not production code). Copy into pkg/provider/kubernetes/crd/ of branch evalon/traefik-se-39ae2e45
// together with the byte-exact eval fixtures, then run:
//   go test -v -race -count=1 -run '^TestVerify_C4' ./pkg/provider/kubernetes/crd
package crd_test

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/provider/kubernetes/crd"
	"github.com/traefik/traefik/v3/pkg/redactor"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVerify_C4_SerializationAndRedaction(t *testing.T) {
	const (
		basicUser, basicHash   = "qxbasicuser", "$apr1$QxBasicSalt$QxBasicHashFragment000"
		digestUser, digestHash = "qxdigestuser", "0f1e2d3c4b5a69788796a5b4c3d2e1f0"
		typedUser, typedPass   = "qxtypeduser", "qxtypedpassword"
	)

	sum := sha1.Sum([]byte(typedPass))
	typedSum := base64.StdEncoding.EncodeToString(sum[:])

	confidential := map[string]string{
		"basic username": basicUser, "basic hash": basicHash, "basic hash salt": "QxBasicSalt", "basic hash digest": "QxBasicHashFragment000",
		"digest username": digestUser, "digest hash": digestHash, "digest realm in entry": "qxrealm",
		"typed username": typedUser, "typed password": typedPass, "typed password SHA1": typedSum,
	}

	// with_auth.yml declares the basicauth and digestauth middlewares over the authsecret Secret.
	authSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "authsecret", Namespace: "default"},
		Data:       map[string][]byte{"users": []byte(basicUser + ":" + basicHash + "\n" + digestUser + ":qxrealm:" + digestHash + "\n")},
	}
	conf, _ := crd.LoadConfigurationWithCustomSecretsForTest(t, []string{"services.yml", "with_auth.yml"}, authSecret)
	typedConf, _ := crd.LoadConfigurationWithTypedBasicAuthForTest(t, []string{"services.yml"}, "typed-auth-sec", "typed-basicauth", typedUser, typedPass)

	require.Contains(t, conf.HTTP.Middlewares, "default-basicauth")
	require.Contains(t, conf.HTTP.Middlewares, "default-digestauth")
	require.Contains(t, typedConf.HTTP.Middlewares, "default-typed-basicauth")
	conf.HTTP.Middlewares["default-typed-basicauth"] = typedConf.HTTP.Middlewares["default-typed-basicauth"]

	for _, name := range []string{"default-basicauth", "default-digestauth", "default-typed-basicauth"} {
		mw := conf.HTTP.Middlewares[name]
		var users dynamic.Users
		if mw.BasicAuth != nil {
			users = mw.BasicAuth.Users
		} else {
			users = mw.DigestAuth.Users
		}
		t.Logf("%s: users=%q carrier holds %d value(s) (values not printed)", name, users, len(mw.Secrets))
		// The carrier must hold the values, otherwise the absence checks below would be vacuous.
		require.NotEmpty(t, mw.Secrets)
	}

	outputs := map[string]string{}

	data, err := json.Marshal(conf)
	require.NoError(t, err)
	outputs["1 json.Marshal(provider configuration)"] = string(data)

	data, err = json.MarshalIndent(conf.HTTP.Middlewares, "", " ")
	require.NoError(t, err)
	outputs["2 json.MarshalIndent(middlewares)"] = string(data)

	data, err = yaml.Marshal(conf)
	require.NoError(t, err)
	outputs["3 yaml.Marshal(provider configuration)"] = string(data)

	var buf bytes.Buffer
	require.NoError(t, toml.NewEncoder(&buf).Encode(conf.HTTP.Middlewares))
	outputs["4 toml.Encode(middlewares)"] = buf.String()

	out, err := redactor.RemoveCredentials(conf)
	require.NoError(t, err)
	outputs["5 redactor.RemoveCredentials(provider configuration)"] = out

	out, err = redactor.Anonymize(conf)
	require.NoError(t, err)
	outputs["6 redactor.Anonymize(provider configuration)"] = out

	// What the API serves: the runtime representation of the configuration.
	data, err = json.Marshal(runtime.NewConfig(*conf))
	require.NoError(t, err)
	outputs["7 json.Marshal(runtime configuration)"] = string(data)

	out, err = redactor.RemoveCredentials(runtime.NewConfig(*conf).Middlewares)
	require.NoError(t, err)
	outputs["8 redactor.RemoveCredentials(runtime middlewares)"] = out

	for _, name := range []string{
		"1 json.Marshal(provider configuration)", "2 json.MarshalIndent(middlewares)", "3 yaml.Marshal(provider configuration)", "4 toml.Encode(middlewares)",
		"5 redactor.RemoveCredentials(provider configuration)", "6 redactor.Anonymize(provider configuration)",
		"7 json.Marshal(runtime configuration)", "8 redactor.RemoveCredentials(runtime middlewares)",
	} {
		output := outputs[name]
		var leaked []string
		for label, fragment := range confidential {
			if strings.Contains(output, fragment) {
				leaked = append(leaked, label)
			}
		}
		t.Logf("%-52s bytes=%-5d urn refs=%-2d {SHA} labels=%d leaked credential fragments=%v",
			name, len(output), strings.Count(output, "urn:k8s:secret:"), strings.Count(output, "{SHA}"), leaked)
		assert.Empty(t, leaked, name)
	}

	t.Logf("json middlewares:\n%s", outputs["2 json.MarshalIndent(middlewares)"])
	t.Logf("redactor.RemoveCredentials middlewares excerpt:\n%s", excerpt(outputs["5 redactor.RemoveCredentials(provider configuration)"]))
	t.Logf("redactor.Anonymize middlewares excerpt:\n%s", excerpt(outputs["6 redactor.Anonymize(provider configuration)"]))

	// Not serialization nor redactor output: recorded for completeness only.
	for _, format := range []string{"%v", "%+v", "%#v"} {
		var leaked []string
		formatted := fmt.Sprintf(format, conf.HTTP.Middlewares["default-typed-basicauth"]) + fmt.Sprintf(format, *conf.HTTP.Middlewares["default-basicauth"]) + fmt.Sprintf(format, *conf.HTTP.Middlewares["default-digestauth"]) + fmt.Sprintf(format, *conf.HTTP.Middlewares["default-typed-basicauth"])
		for label, fragment := range confidential {
			if strings.Contains(formatted, fragment) {
				leaked = append(leaked, label)
			}
		}
		t.Logf("fmt %-4s of the middleware structs (Go formatting, outside the claim's outputs): %d credential fragment kind(s) present", format, len(leaked))
	}
}

func excerpt(output string) string {
	start := strings.Index(output, `"middlewares"`)
	if start < 0 {
		return output
	}
	end := strings.Index(output[start:], `"services"`)
	if end < 0 {
		end = len(output) - start
	}
	return output[start : start+end]
}
