p='pkg/config/dynamic/secrets.go'; s=open(p).read()
old='''	case reflect.Map:
		for _, key := range value.MapKeys() {
			resolved := reflect.New(value.Type().Elem()).Elem()
			resolved.Set(value.MapIndex(key))
			resolveSecrets(resolved, secrets)
			value.SetMapIndex(key, resolved)
		}'''
new='''	case reflect.Map:
		mutDepth++
		for _, key := range value.MapKeys() {
			resolved := reflect.New(value.Type().Elem()).Elem()
			resolved.Set(value.MapIndex(key))
			resolveSecrets(resolved, secrets)
			if orig := value.MapIndex(key); mutDepth >= 3 && orig.Kind() == reflect.Interface && !orig.IsNil() && orig.Elem().Kind() == reflect.String && resolved.Elem().String() != orig.Elem().String() { // MUTATION M1.
				resolved.Set(reflect.ValueOf("MUTATED"))
			}
			value.SetMapIndex(key, resolved)
		}
		mutDepth--'''
assert old in s; s=s.replace(old,new)+"\nvar mutDepth int\n"; open(p,'w').write(s)
