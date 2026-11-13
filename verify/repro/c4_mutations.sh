#!/bin/bash
# Run: verify/repro/c4_mutations.sh <worktree-cc522816>   (mutates pkg/server/middleware/secrets.go, runs the branch's own TestBuilder_PluginSecretReference, restores)
W=$1; F=$W/pkg/server/middleware/secrets.go
t() { (cd $W && go test -count=1 -v -run '^TestBuilder_PluginSecretReference$' ./pkg/server/middleware/ 2>&1 | grep -E '^(--- |ok|FAIL)|expected:|actual  :' ); }
echo "=== baseline"; t
cp $F /tmp/c4.bak
echo "=== M1: references inside lists are not resolved"
python3 - $F <<'PY'
import sys; p=sys.argv[1]; s=open(p).read(); o=s
s=s.replace("			v[i] = resolvePluginValue(v[i], secrets, found)","			_ = i")
assert s!=o; open(p,'w').write(s)
PY
t; cp /tmp/c4.bak $F
echo "=== M2: unbound (literal) list siblings are dropped to empty"
python3 - $F <<'PY'
import sys; p=sys.argv[1]; s=open(p).read(); o=s
s=s.replace("""			v[i] = resolvePluginValue(v[i], secrets, found)""","""			if str, ok := v[i].(string); ok {
				if _, bound := secrets.Resolve(str); !bound {
					v[i] = ""
					continue
				}
			}
			v[i] = resolvePluginValue(v[i], secrets, found)""")
assert s!=o; open(p,'w').write(s)
PY
t; cp /tmp/c4.bak $F
