p='pkg/server/middleware/secrets.go'; s=open(p).read()
old='''	resolved := config.DeepCopy()
	resolved.Secrets = nil
	resolveSecretValue(reflect.ValueOf(resolved), config.Secrets)'''
new='''	resolved := config.DeepCopy()
	resolved.Secrets = nil
	// MUTATION M2: drop non-secret siblings.
	if resolved.ForwardAuth != nil {
		resolved.ForwardAuth.Address = ""
	}
	if resolved.RateLimit != nil && resolved.RateLimit.Redis != nil {
		resolved.RateLimit.Redis.Endpoints = nil
	}
	for _, plugin := range resolved.Plugin {
		for key, value := range plugin {
			if str, ok := value.(string); ok {
				if _, isRef := config.Secrets[str]; !isRef {
					delete(plugin, key)
				}
			}
		}
	}
	resolveSecretValue(reflect.ValueOf(resolved), config.Secrets)'''
assert old in s; open(p,'w').write(s.replace(old,new))
