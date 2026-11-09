package main

import (
	"errors"
	"os"

	"github.com/ynov/equipment-loans/internal/adapters/auth"
)

type config struct {
	port        string
	mcpPort     string
	databaseURL string
	auth        auth.Config
}

func loadConfig() (config, error) {
	cfg := config{
		port:        getenv("PORT", "8080"),
		mcpPort:     getenv("MCP_PORT", ""),
		databaseURL: os.Getenv("DATABASE_URL"),
		auth: auth.Config{
			Issuer:     os.Getenv("OIDC_ISSUER"),
			Audience:   os.Getenv("OIDC_AUDIENCE"),
			JWKSURL:    os.Getenv("OIDC_JWKS_URL"),
			RolesClaim: getenv("ROLES_CLAIM", "roles"),
		},
	}
	return cfg, cfg.validate()
}

func (c config) validate() error {
	var errs []error
	for name, value := range map[string]string{
		"DATABASE_URL":  c.databaseURL,
		"OIDC_ISSUER":   c.auth.Issuer,
		"OIDC_AUDIENCE": c.auth.Audience,
		"OIDC_JWKS_URL": c.auth.JWKSURL,
	} {
		if value == "" {
			errs = append(errs, errors.New(name+" is required"))
		}
	}
	return errors.Join(errs...)
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
