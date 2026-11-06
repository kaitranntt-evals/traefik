#!/usr/bin/env bash
# Run: in a checkout of evalon/traefik-se-a75ff6bf: bash verify/repro/c3_mutation_check.sh  (mutates middlewares.go/plugins.go, runs the committed Secret tests, restores the files)
set -euo pipefail
python3 - <<'PY'
p='pkg/server/middleware/middlewares.go'
s=open(p).read()
# Mutation 1 (stored_diagnostic_coverage): MiddlewareInfo.Err receives a Secret-bearing diagnostic, the returned error is unchanged.
s=s.replace('''			constructor, err := b.buildConstructor(constructorContext, middlewareName)
			if err != nil {
				b.configs[middlewareName].AddError(err, true)''','''			constructor, err := b.buildConstructor(constructorContext, middlewareName)
			if err != nil {
				b.configs[middlewareName].AddError(fmt.Errorf("MUTATION stored diagnostic leaks value: invalid"), true)''')
open(p,'w').write(s)
p='pkg/server/middleware/plugins.go'
s=open(p).read()
# Mutation 2 (cause_chain_coverage): the redacted error wraps a cause quoting the Secret value.
s=s.replace('''func secretReferencesError(secrets dynamic.SecretValues) error {
	return fmt.Errorf("invalid configuration: value rejected from one of the secret references %s", strings.Join(secrets.References(), ", "))
}''','''func secretReferencesError(secrets dynamic.SecretValues) error {
	return &leakyErr{msg: fmt.Sprintf("invalid configuration: value rejected from one of the secret references %s", strings.Join(secrets.References(), ", ")), cause: errors.New("MUTATION cause quotes value: invalid")}
}

type leakyErr struct {
	msg   string
	cause error
}

func (e *leakyErr) Error() string { return e.msg }
func (e *leakyErr) Unwrap() error { return e.cause }''')
open(p,'w').write(s)
PY
git diff --stat
go test -count=1 ./pkg/server/middleware ./pkg/config/dynamic || true
git checkout pkg/server/middleware/middlewares.go pkg/server/middleware/plugins.go
