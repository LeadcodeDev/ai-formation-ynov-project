package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

const identityKey = "identity"

type Service interface {
	ListItems(ctx context.Context, actor domain.Identity, filter domain.ItemFilter) ([]domain.ItemView, error)
	BorrowItem(ctx context.Context, actor domain.Identity, cmd service.BorrowItem) (domain.Loan, error)
	ReturnLoan(ctx context.Context, actor domain.Identity, cmd service.ReturnLoan) (domain.Loan, error)
	ExtendLoan(ctx context.Context, actor domain.Identity, id domain.LoanID) (domain.Loan, error)
	MyLoans(ctx context.Context, actor domain.Identity) ([]domain.Loan, error)
	OverdueLoans(ctx context.Context, actor domain.Identity) ([]domain.Loan, error)
}

type Verifier interface {
	Verify(ctx context.Context, token string) (domain.Identity, error)
}

func NewHandler(svc Service, verifier Verifier, logger *slog.Logger) http.Handler {
	server := newServer(svc, logger)
	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
	opts := &auth.RequireBearerTokenOptions{AllowMissingExpiration: true}
	return auth.RequireBearerToken(tokenVerifier(verifier, logger), opts)(streamable)
}

func tokenVerifier(verifier Verifier, logger *slog.Logger) auth.TokenVerifier {
	return func(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		identity, err := verifier.Verify(ctx, token)
		if err != nil {
			logger.Info("mcp token rejected", "error", err)
			return nil, fmt.Errorf("%w: authentication failed", auth.ErrInvalidToken)
		}
		return &auth.TokenInfo{UserID: identity.Sub, Extra: map[string]any{identityKey: identity}}, nil
	}
}

func identityFrom(req *mcp.CallToolRequest) (domain.Identity, error) {
	if req.Extra == nil || req.Extra.TokenInfo == nil {
		return domain.Identity{}, errUnauthenticated
	}
	identity, ok := req.Extra.TokenInfo.Extra[identityKey].(domain.Identity)
	if !ok {
		return domain.Identity{}, errUnauthenticated
	}
	return identity, nil
}
