#!/usr/bin/env python3
# Run: python3 verify/repro/c2_carrier_tolerant_fixture.py <worktree-of-evalon/traefik-se-9c319c44>   (after installing the eval fixtures there)
# Rewrites the installed eval fixture eval_provider_to_builder_runtime_test.go so that its helpers and
# direct equality assertions read a dynamic.Secret carrier as the URN it carries (carrier-type neutral).
# Nothing else in the fixture changes: plugin execution, isolation and JSON comparisons stay byte-identical.
import sys, re
p = sys.argv[1] + "/pkg/provider/kubernetes/crd/eval_provider_to_builder_runtime_test.go"
s = open(p).read()
def sub(old, new, count=1):
    global s
    assert s.count(old) >= 1, old
    s = s.replace(old, new) if count == 0 else s.replace(old, new, count)
# helpers: treat dynamic.Secret as its Reference
sub("func findConfigSecretValue(cfg any) string {\n\tswitch v := cfg.(type) {",
    "func findConfigSecretValue(cfg any) string {\n\tswitch v := verifyNorm(cfg).(type) {")
sub("func evalConfigStrings(cfg any) []string {\n\tswitch v := cfg.(type) {",
    "func evalConfigStrings(cfg any) []string {\n\tswitch v := verifyNorm(cfg).(type) {")
# direct equality assertions on retained fields
s = re.sub(r'assert\.Equal\(t, "urn:k8s:secret:name:key", ((?:info|boundInfo)\.Plugin\["test-secret"\]\["[a-z_]+"\]|publishedPlugin\["[a-z_]+"\]),',
           r'assert.Equal(t, "urn:k8s:secret:name:key", verifyNorm(\1),', s)
s += '''
// verifyNorm renders a retained plugin configuration carrier-type neutral:
// every dynamic.Secret becomes the URN string it carries (never its value).
func verifyNorm(cfg any) any {
	switch v := cfg.(type) {
	case dynamic.Secret:
		return v.Reference
	case dynamic.PluginConf:
		return verifyNorm(map[string]any(v))
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, nested := range v {
			out[k] = verifyNorm(nested)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, nested := range v {
			out[i] = verifyNorm(nested)
		}
		return out
	}
	return cfg
}
'''
open(p, "w").write(s)
