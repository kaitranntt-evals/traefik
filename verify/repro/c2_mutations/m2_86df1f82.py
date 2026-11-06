p='pkg/config/dynamic/secrets.go'; s=open(p).read()
old='''	config := m.DeepCopy()
	config.Secrets = nil
	resolveSecrets(reflect.ValueOf(config), m.Secrets)'''
new='''	config := m.DeepCopy()
	config.Secrets = nil
	// MUTATION M2: drop non-secret siblings.
	if config.ForwardAuth != nil {
		config.ForwardAuth.Address = ""
	}
	if config.RateLimit != nil && config.RateLimit.Redis != nil {
		config.RateLimit.Redis.Endpoints = nil
	}
	for _, plugin := range config.Plugin {
		for key, value := range plugin {
			if str, ok := value.(string); ok {
				if _, isRef := m.Secrets[str]; !isRef {
					delete(plugin, key)
				}
			}
		}
	}
	resolveSecrets(reflect.ValueOf(config), m.Secrets)'''
assert old in s; open(p,'w').write(s.replace(old,new))
