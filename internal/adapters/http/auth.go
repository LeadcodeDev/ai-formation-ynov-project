package httpapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/ynov/equipment-loans/internal/domain"
)

type identityKey struct{}

func identityFrom(ctx context.Context) domain.Identity {
	id, _ := ctx.Value(identityKey{}).(domain.Identity)
	return id
}

func bearerToken(r *http.Request) (string, bool) {
	scheme, token, found := strings.Cut(r.Header.Get("Authorization"), " ")
	token = strings.TrimSpace(token)
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", false
	}
	return token, true
}

func authenticate(verifier Verifier, h *handler, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			h.unauthenticated(w, "missing bearer token")
			return
		}
		identity, err := verifier.Verify(r.Context(), token)
		if err != nil {
			h.unauthenticated(w, "invalid token")
			return
		}
		if len(identity.Roles) == 0 {
			writeError(w, http.StatusForbidden, "forbidden", "no recognised role")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	})
}

func (h *handler) unauthenticated(w http.ResponseWriter, message string) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	writeError(w, http.StatusUnauthorized, "unauthenticated", message)
}
