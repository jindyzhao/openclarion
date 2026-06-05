// Package envmap resolves deployment-managed secret references from an
// explicitly configured in-process map.
package envmap

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/openclarion/openclarion/internal/strictjson"
	"github.com/openclarion/openclarion/internal/usecases/ports"
)

const (
	maxSecretRefBytes   = 256
	maxSecretValueBytes = 8192
)

// Resolver resolves secret_ref values from a fixed startup map.
type Resolver struct {
	secrets map[string]string
}

// NewResolver validates and copies a secret reference map.
func NewResolver(secrets map[string]string) (*Resolver, error) {
	out := make(map[string]string, len(secrets))
	for ref, value := range secrets {
		if err := validateRef(ref); err != nil {
			return nil, err
		}
		if err := validateSecretValue(value); err != nil {
			return nil, err
		}
		out[ref] = value
	}
	return &Resolver{secrets: out}, nil
}

// NewResolverFromJSON builds a resolver from a strict JSON object whose keys
// are public secret references and whose values are secret strings.
func NewResolverFromJSON(raw string) (*Resolver, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return NewResolver(nil)
	}
	var secrets map[string]string
	if err := strictjson.Unmarshal([]byte(raw), &secrets); err != nil {
		return nil, fmt.Errorf("envmap secret resolver: parse secret map JSON")
	}
	return NewResolver(secrets)
}

// ResolveSecret implements ports.SecretResolver.
func (r *Resolver) ResolveSecret(ctx context.Context, ref string) (ports.Secret, error) {
	if err := ctx.Err(); err != nil {
		return ports.Secret{}, err
	}
	if r == nil {
		return ports.Secret{}, ports.ErrSecretNotFound
	}
	value, ok := r.secrets[ref]
	if !ok {
		return ports.Secret{}, ports.ErrSecretNotFound
	}
	return ports.Secret{Value: value}, nil
}

func validateRef(ref string) error {
	if ref == "" {
		return fmt.Errorf("envmap secret resolver: secret ref must be non-empty")
	}
	if len(ref) > maxSecretRefBytes {
		return fmt.Errorf("envmap secret resolver: secret ref exceeds %d bytes", maxSecretRefBytes)
	}
	if containsControlOrSpace(ref) {
		return fmt.Errorf("envmap secret resolver: secret ref must not contain whitespace or control characters")
	}
	return nil
}

func validateSecretValue(value string) error {
	if value == "" {
		return fmt.Errorf("envmap secret resolver: secret value must be non-empty")
	}
	if len(value) > maxSecretValueBytes {
		return fmt.Errorf("envmap secret resolver: secret value exceeds %d bytes", maxSecretValueBytes)
	}
	if containsControlOrSpace(value) {
		return fmt.Errorf("envmap secret resolver: secret value must not contain whitespace or control characters")
	}
	return nil
}

func containsControlOrSpace(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
