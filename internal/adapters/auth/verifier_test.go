package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ynov/equipment-loans/internal/adapters/auth"
	"github.com/ynov/equipment-loans/internal/domain"
)

const (
	issuer   = "https://iam.example/realms/loans"
	audience = "loans-api"
	kid      = "k1"
)

func newKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func jwksServer(t *testing.T, pub *rsa.PublicKey) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "use": "sig", "alg": "RS256", "kid": kid,
		"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func baseClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"iss":   issuer,
		"aud":   audience,
		"sub":   "user-1",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"roles": []string{"student"},
	}
}

func sign(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newVerifier(t *testing.T, pub *rsa.PublicKey, rolesClaim string) *auth.Verifier {
	t.Helper()
	srv := jwksServer(t, pub)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	v, err := auth.NewVerifier(ctx, auth.Config{Issuer: issuer, Audience: audience, JWKSURL: srv.URL, RolesClaim: rolesClaim})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func with(c jwt.MapClaims, k string, v any) jwt.MapClaims {
	out := jwt.MapClaims{}
	for key, val := range c {
		out[key] = val
	}
	if v == nil {
		delete(out, k)
	} else {
		out[k] = v
	}
	return out
}

func TestAC9VerifyRejects(t *testing.T) {
	key := newKey(t)
	other := newKey(t)
	pubDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	v := newVerifier(t, &key.PublicKey, "")

	noneTok := jwt.NewWithClaims(jwt.SigningMethodNone, baseClaims())
	noneTok.Header["kid"] = kid
	none, err := noneTok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"garbage", "not-a-jwt"},
		{"expired", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "exp", time.Now().Add(-time.Hour).Unix()))},
		{"missing exp", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "exp", nil))},
		{"wrong iss", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "iss", "https://evil"))},
		{"wrong aud", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "aud", "other"))},
		{"wrong aud array", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "aud", []string{"a", "b"}))},
		{"other key", sign(t, jwt.SigningMethodRS256, other, baseClaims())},
		{"alg none", none},
		{"hs256 with public key", sign(t, jwt.SigningMethodHS256, pubPEM, baseClaims())},
		{"empty sub", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "sub", ""))},
		{"missing sub", sign(t, jwt.SigningMethodRS256, key, with(baseClaims(), "sub", nil))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.Verify(context.Background(), tt.token)
			if !errors.Is(err, auth.ErrUnauthenticated) {
				t.Fatalf("got %v, want ErrUnauthenticated", err)
			}
		})
	}
}

func TestAC9VerifyAccepts(t *testing.T) {
	key := newKey(t)
	tests := []struct {
		name       string
		rolesClaim string
		claims     jwt.MapClaims
		wantRoles  []domain.Role
	}{
		{"default roles claim", "", baseClaims(), []domain.Role{domain.RoleStudent}},
		{"aud array", "", with(baseClaims(), "aud", []string{"x", audience}), []domain.Role{domain.RoleStudent}},
		{"dotted path", "realm_access.roles", with(baseClaims(), "realm_access", map[string]any{"roles": []string{"staff", "admin"}}), []domain.Role{domain.RoleStaff, domain.RoleAdmin}},
		{"single string role", "", with(baseClaims(), "roles", "staff"), []domain.Role{domain.RoleStaff}},
		{"unknown roles ignored", "", with(baseClaims(), "roles", []string{"wizard", "student", "root"}), []domain.Role{domain.RoleStudent}},
		{"only unknown roles", "", with(baseClaims(), "roles", []string{"wizard"}), nil},
		{"missing roles claim", "", with(baseClaims(), "roles", nil), nil},
		{"missing dotted path", "realm_access.roles", baseClaims(), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := newVerifier(t, &key.PublicKey, tt.rolesClaim)
			id, err := v.Verify(context.Background(), sign(t, jwt.SigningMethodRS256, key, tt.claims))
			if err != nil {
				t.Fatal(err)
			}
			if id.Sub != "user-1" {
				t.Fatalf("sub = %q", id.Sub)
			}
			if len(id.Roles) != len(tt.wantRoles) {
				t.Fatalf("roles = %v, want %v", id.Roles, tt.wantRoles)
			}
			for i, r := range tt.wantRoles {
				if id.Roles[i] != r {
					t.Fatalf("roles = %v, want %v", id.Roles, tt.wantRoles)
				}
			}
		})
	}
}
