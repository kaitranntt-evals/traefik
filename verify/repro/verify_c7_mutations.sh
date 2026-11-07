#!/usr/bin/env sh
# Run from a checkout of evalon/traefik-se-cf8f03b1: sh verify/repro/verify_c7_mutations.sh
# Applies four mutations to the redaction code one at a time and runs the committed tests after each.
# Mutations A/B/C stay green (no committed test covers wrapped causes, escaped payloads, empty payloads);
# mutation D fails TestBuilder_pluginErrorHidesSecretValues (stored diagnostics ARE covered).
set -eu
run() { go test -count=1 ./pkg/server/middleware ./pkg/config/dynamic 2>&1 | tail -3; }
echo "### baseline"; run

echo "### mutation A: hidden error keeps the credential-bearing original as a wrapped cause (same message)"
python3 - <<'PY'
p='pkg/server/middleware/secrets.go'; s=open(p).read()
s=s.replace('var errSecretHidden = errors.New("error hidden because its message contains a secret value")',
 'var errSecretHidden = errors.New("error hidden because its message contains a secret value")\n\ntype leakyWrap struct {\n\tmsg   string\n\tcause error\n}\n\nfunc (e *leakyWrap) Error() string { return e.msg }\nfunc (e *leakyWrap) Unwrap() error { return e.cause }')
s=s.replace('\t\t\treturn errors.New(refErr.Error())','\t\t\treturn &leakyWrap{msg: refErr.Error(), cause: err}')
s=s.replace('\treturn errSecretHidden\n}','\treturn &leakyWrap{msg: errSecretHidden.Error(), cause: err}\n}')
open(p,'w').write(s)
PY
run; git checkout -- pkg/server/middleware/secrets.go

echo "### mutation B: Secrets.Reveals ignores values containing quotes or backslashes"
sed -i 's|if value != "" \&\& strings.Contains(text, value) {|if value != "" \&\& !strings.ContainsAny(value, "\\"\\\\") \&\& strings.Contains(text, value) {|' pkg/config/dynamic/secrets.go
run; git checkout -- pkg/config/dynamic/secrets.go

echo "### mutation C: hideSecretValues skips sanitation when any secret value is empty"
python3 - <<'PY'
p='pkg/server/middleware/secrets.go'; s=open(p).read()
s=s.replace('\tif err == nil || !secrets.Reveals(err.Error()) {\n\t\treturn err\n\t}','\tif err == nil || !secrets.Reveals(err.Error()) {\n\t\treturn err\n\t}\n\tfor _, v := range secrets {\n\t\tif v == "" {\n\t\t\treturn err\n\t\t}\n\t}')
open(p,'w').write(s)
PY
run; git checkout -- pkg/server/middleware/secrets.go

echo "### mutation D: also store the raw plugin error in info.Err (control: stored diagnostics are asserted)"
python3 - <<'PY'
p='pkg/server/middleware/middlewares.go'; s=open(p).read()
old='\t\tplug, err := b.pluginBuilder.Build(pluginType, rawPluginConfig, middlewareName)\n\t\tif err != nil {\n'
assert old in s
open(p,'w').write(s.replace(old, old+'\t\t\tinfo.AddError(err, true)\n'))
PY
run; git checkout -- pkg/server/middleware/middlewares.go
