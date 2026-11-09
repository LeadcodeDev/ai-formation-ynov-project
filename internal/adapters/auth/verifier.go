package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/ynov/equipment-loans/internal/domain"
)

var ErrUnauthenticated = errors.New("unauthenticated")

const defaultRolesClaim = "roles"

type Config struct {
	Issuer     string
	Audience   string
	JWKSURL    string
	RolesClaim string
}

type Verifier struct {
	keys      keyfunc.Keyfunc
	parser    *jwt.Parser
	rolesPath []string
}

func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	keys, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWKSURL})
	if err != nil {
		return nil, fmt.Errorf("load jwks: %w", err)
	}
	claim := cfg.RolesClaim
	if claim == "" {
		claim = defaultRolesClaim
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(cfg.Issuer),
		jwt.WithAudience(cfg.Audience),
		jwt.WithExpirationRequired(),
	)
	return &Verifier{keys: keys, parser: parser, rolesPath: strings.Split(claim, ".")}, nil
}

func (v *Verifier) Verify(ctx context.Context, token string) (domain.Identity, error) {
	claims := jwt.MapClaims{}
	if _, err := v.parser.ParseWithClaims(token, claims, v.keys.Keyfunc); err != nil {
		return domain.Identity{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return domain.Identity{}, fmt.Errorf("%w: missing subject", ErrUnauthenticated)
	}
	identity, err := domain.NewIdentity(sub, extractRoles(claims, v.rolesPath))
	if err != nil {
		return domain.Identity{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	return identity, nil
}

func extractRoles(claims jwt.MapClaims, path []string) []domain.Role {
	var current any = map[string]any(claims)
	for _, key := range path {
		node, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = node[key]
	}
	switch value := current.(type) {
	case string:
		return parseRoles([]string{value})
	case []any:
		names := make([]string, 0, len(value))
		for _, item := range value {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
		return parseRoles(names)
	}
	return nil
}

func parseRoles(names []string) []domain.Role {
	var roles []domain.Role
	for _, name := range names {
		if role, ok := domain.ParseRole(name); ok {
			roles = append(roles, role)
		}
	}
	return roles
}
