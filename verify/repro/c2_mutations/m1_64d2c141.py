p='pkg/config/dynamic/secrets.go'; s=open(p).read()
old='''	case map[string]any:
		for key := range v {
			v[key] = resolveSecretValue(v[key], resolve)
		}'''
new='''	case map[string]any:
		for key := range v {
			orig := v[key]
			v[key] = resolveSecretValue(v[key], resolve)
			if _, ok := orig.(string); ok && v[key] != orig { // MUTATION M1: corrupt nested resolved values.
				v[key] = "MUTATED"
			}
		}'''
assert old in s; open(p,'w').write(s.replace(old,new))
