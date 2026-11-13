#!/bin/bash
# Run: verify/repro/c3_mutations.sh <worktree-177cb630> <worktree-e230a658>   (each mutation is applied to production code, the branch's own tests are run, then the file is restored)
A=$1; B=$2
run() { # dir label file pycode pkgs runregex
  local dir=$1 label=$2 file=$3 py=$4 pkgs=$5 rx=$6
  cp "$dir/$file" /tmp/c3.bak
  python3 - "$dir/$file" <<PY
import sys
p=sys.argv[1]; s=open(p).read(); o=s
$py
assert s!=o, "mutation did not apply"
open(p,'w').write(s)
PY
  echo "=== [$label]"
  (cd "$dir" && go test -count=1 -run "$rx" $pkgs 2>&1 | grep -E '^(--- FAIL|    --- FAIL|ok|FAIL|PASS)' )
  cp /tmp/c3.bak "$dir/$file"
}
echo "##### 177cb630 baseline"; (cd $A && go test -count=1 -v -run 'TestResolveSecretUserEntries|TestBuilderSecretForwardAuthTLS|TestResolveSecretsCopy|TestBuilderSecretAuthentication|TestBuilderSecretAuthErrors' ./pkg/server/middleware/ 2>&1 | grep -E '^(--- |ok|FAIL)')
F=pkg/server/middleware/secrets.go
run $A "177 M1 resolveSecretUsers drops literal (unbound) siblings" $F 's=s.replace("""		} else {
			resolved = append(resolved, user)
		}""","""		}""")' ./pkg/server/middleware/ 'Secret'
run $A "177 M2 resolveSecretUsers leaves references unresolved" $F 's=s.replace("resolved = append(resolved, value...)","_ = value\n\t\t\tresolved = append(resolved, user)")' ./pkg/server/middleware/ 'Secret'
run $A "177 M3 resolveSecrets blanks the non-secret ForwardAuth.Address" $F 's=s.replace("""	resolveSecretValue(reflect.ValueOf(resolved), config.Secrets)
""","""	resolveSecretValue(reflect.ValueOf(resolved), config.Secrets)
	if resolved.ForwardAuth != nil {
		resolved.ForwardAuth.Address = ""
	}
""")' ./pkg/server/middleware/ 'Secret'
run $A "177 M4 nested ForwardAuth.TLS secrets left unresolved" $F 's=s.replace("""	resolveSecretValue(reflect.ValueOf(resolved), config.Secrets)
""","""	resolveSecretValue(reflect.ValueOf(resolved), config.Secrets)
	if config.ForwardAuth != nil && config.ForwardAuth.TLS != nil {
		resolved.ForwardAuth.TLS.Cert, resolved.ForwardAuth.TLS.Key = config.ForwardAuth.TLS.Cert, config.ForwardAuth.TLS.Key
	}
""")' ./pkg/server/middleware/ 'Secret'

echo "##### e230a658 baseline"; (cd $B && go test -count=1 -v -run 'TestSecretValues_ResolveUsers|TestSecretValues_ResolvePluginConf|TestBuilder_PluginSecretReference|TestBuilder_BasicAuthSecretReference|TestBuilder_AuthErrorDoesNotRevealSecretValues' ./pkg/config/dynamic/ ./pkg/server/middleware/ 2>&1 | grep -E '^(--- |ok|FAIL)')
F=pkg/config/dynamic/secrets.go
P="./pkg/config/dynamic/ ./pkg/server/middleware/"
run $B "e23 M1 ResolveUsers drops literal (unbound) siblings" $F 's=s.replace("""		if !ok {
			resolved = append(resolved, user)
			continue""","""		if !ok {
			continue""")' "$P" 'Secret'
run $B "e23 M2 ResolveUsers leaves references unresolved" $F 's=s.replace("""				resolved = append(resolved, line)""","""				_ = line
				resolved = append(resolved, user)""")' "$P" 'Secret'
run $B "e23 M3 ResolvePluginConf drops non-string siblings" $F 's=s.replace("""	default:
		return v
	}""","""	default:
		return nil
	}""")' "$P" 'Secret'
run $B "e23 M4 ResolvePluginConf does not resolve inside nested lists" $F 's=s.replace("resolved[i] = s.resolveAny(val)","resolved[i] = val")' "$P" 'Secret'
