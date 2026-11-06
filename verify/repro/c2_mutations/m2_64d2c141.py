p='pkg/config/dynamic/secrets.go'; s=open(p).read()
old='''	for _, plugin := range c.Plugin {
		for key, value := range plugin {
			plugin[key] = resolveSecretValue(value, resolve)
		}
	}
	return c'''
new='''	// MUTATION M2: drop non-secret siblings.
	if c.ForwardAuth != nil {
		c.ForwardAuth.Address = ""
	}
	if c.RateLimit != nil && c.RateLimit.Redis != nil {
		c.RateLimit.Redis.Endpoints = nil
	}
	for _, plugin := range c.Plugin {
		for key, value := range plugin {
			if str, ok := value.(string); ok {
				if _, isRef := secrets[str]; !isRef {
					delete(plugin, key)
				}
			}
		}
	}
	for _, plugin := range c.Plugin {
		for key, value := range plugin {
			plugin[key] = resolveSecretValue(value, resolve)
		}
	}
	return c'''
assert old in s; open(p,'w').write(s.replace(old,new))
