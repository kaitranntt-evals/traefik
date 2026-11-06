p='pkg/server/middleware/secrets.go'; s=open(p).read()
old='''	case reflect.Map:
		for _, key := range value.MapKeys() {
			entry := reflect.New(value.Type().Elem()).Elem()
			entry.Set(value.MapIndex(key))
			resolveSecretValue(entry, secrets)
			value.SetMapIndex(key, entry)
		}'''
new='''	case reflect.Map:
		mutDepth++
		for _, key := range value.MapKeys() {
			entry := reflect.New(value.Type().Elem()).Elem()
			entry.Set(value.MapIndex(key))
			resolveSecretValue(entry, secrets)
			if orig := value.MapIndex(key); mutDepth >= 3 && orig.Kind() == reflect.Interface && !orig.IsNil() && orig.Elem().Kind() == reflect.String && entry.Elem().String() != orig.Elem().String() { // MUTATION M1.
				entry.Set(reflect.ValueOf("MUTATED"))
			}
			value.SetMapIndex(key, entry)
		}
		mutDepth--'''
assert old in s; s=s.replace(old,new)+"\nvar mutDepth int\n"; open(p,'w').write(s)
