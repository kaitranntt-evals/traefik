package dynamic

import (
	"sync"
)

var GlobalSecretRegistry sync.Map

func StoreSecret(urn, plaintext string) string {
	GlobalSecretRegistry.Store(urn, plaintext)
	return urn
}

func LookupSecret(urn string) (string, bool) {
	val, ok := GlobalSecretRegistry.Load(urn)
	if !ok {
		return "", false
	}
	return val.(string), true
}
