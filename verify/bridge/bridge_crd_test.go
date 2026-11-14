package crd

import (
	"context"

	"github.com/traefik/traefik/v3/pkg/config/dynamic"
)

// evalPublishLikeProvide mirrors, line for line, what Provider.Provide does on this branch before hashing and emitting a configuration.
func evalPublishLikeProvide(ctx context.Context, p *Provider, client Client) *dynamic.Configuration {
	conf, secrets := p.loadConfigurationAndSecrets(ctx, client)
	dynamic.Secrets.Replace(ProviderName, secrets)
	conf.SecretsDigest = secrets.Digest()
	return conf
}
