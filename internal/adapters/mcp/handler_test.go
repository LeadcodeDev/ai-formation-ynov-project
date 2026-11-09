package mcpserver_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpserver "github.com/ynov/equipment-loans/internal/adapters/mcp"
	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

type call struct {
	method string
	actor  domain.Identity
	borrow service.BorrowItem
	ret    service.ReturnLoan
	loanID domain.LoanID
}

type fakeService struct {
	mu    sync.Mutex
	calls []call
	err   error
}

func (f *fakeService) record(c call) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
	return f.err
}

func (f *fakeService) last(t *testing.T) call {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		t.Fatal("service not called")
	}
	return f.calls[len(f.calls)-1]
}

func (f *fakeService) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

var sampleLoan = domain.Loan{
	ID:          "loan-1",
	ItemID:      "item-1",
	BorrowerSub: "student-1",
	BorrowedAt:  time.Date(2025, 1, 10, 9, 0, 0, 0, time.UTC),
	DueAt:       time.Date(2025, 1, 18, 0, 0, 0, 0, time.UTC),
}

func (f *fakeService) ListItems(_ context.Context, a domain.Identity, _ domain.ItemFilter) ([]domain.ItemView, error) {
	err := f.record(call{method: "ListItems", actor: a})
	return []domain.ItemView{{
		Item:  domain.Item{ID: "item-1", InventoryNumber: "INV-1", Status: domain.ItemAvailable},
		Model: domain.EquipmentModel{Name: "Laptop", Category: "portable", MaxLoanDays: 7},
	}}, err
}

func (f *fakeService) BorrowItem(_ context.Context, a domain.Identity, cmd service.BorrowItem) (domain.Loan, error) {
	return sampleLoan, f.record(call{method: "BorrowItem", actor: a, borrow: cmd})
}

func (f *fakeService) ReturnLoan(_ context.Context, a domain.Identity, cmd service.ReturnLoan) (domain.Loan, error) {
	return sampleLoan, f.record(call{method: "ReturnLoan", actor: a, ret: cmd})
}

func (f *fakeService) ExtendLoan(_ context.Context, a domain.Identity, id domain.LoanID) (domain.Loan, error) {
	return sampleLoan, f.record(call{method: "ExtendLoan", actor: a, loanID: id})
}

func (f *fakeService) MyLoans(_ context.Context, a domain.Identity) ([]domain.Loan, error) {
	return []domain.Loan{sampleLoan}, f.record(call{method: "MyLoans", actor: a})
}

func (f *fakeService) OverdueLoans(_ context.Context, a domain.Identity) ([]domain.Loan, error) {
	return nil, f.record(call{method: "OverdueLoans", actor: a})
}

type fakeVerifier struct{}

var errBadToken = errors.New("jwks lookup failed at internal-host:9999")

func (fakeVerifier) Verify(_ context.Context, token string) (domain.Identity, error) {
	switch token {
	case "staff-token":
		return domain.Identity{Sub: "staff-1", Roles: []domain.Role{domain.RoleStaff}}, nil
	case "student-token":
		return domain.Identity{Sub: "student-1", Roles: []domain.Role{domain.RoleStudent}}, nil
	}
	return domain.Identity{}, errBadToken
}

type bearerTransport struct{ token string }

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if b.token != "" {
		r = r.Clone(r.Context())
		r.Header.Set("Authorization", "Bearer "+b.token)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func newServer(t *testing.T, svc *fakeService) *httptest.Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := httptest.NewServer(mcpserver.NewHandler(svc, fakeVerifier{}, logger))
	t.Cleanup(srv.Close)
	return srv
}

func connect(t *testing.T, srv *httptest.Server, token string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL,
		HTTPClient: &http.Client{Transport: bearerTransport{token: token}},
		MaxRetries: -1,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func callTool(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, error) {
	t.Helper()
	return cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
}

func text(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func TestAC11ListsFiveToolsWithStrictSchemas(t *testing.T) {
	srv := newServer(t, &fakeService{})
	cs := connect(t, srv, "student-token")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"list_available_items": {},
		"borrow_item":          {"item_id", "borrower_sub"},
		"return_item":          {"loan_id", "state"},
		"extend_loan":          {"loan_id"},
		"my_loans":             {},
	}
	if len(res.Tools) != len(want) {
		t.Fatalf("got %d tools", len(res.Tools))
	}
	for _, tool := range res.Tools {
		required, ok := want[tool.Name]
		if !ok {
			t.Fatalf("unexpected tool %s", tool.Name)
		}
		if len(tool.Description) < 40 {
			t.Fatalf("%s: description too short", tool.Name)
		}
		schema := tool.InputSchema.(map[string]any)
		if schema["additionalProperties"] != false {
			t.Fatalf("%s: additionalProperties not false: %v", tool.Name, schema)
		}
		got, _ := schema["required"].([]any)
		if len(got) != len(required) {
			t.Fatalf("%s: required %v want %v", tool.Name, got, required)
		}
	}
}

func TestAC11ReturnItemSchemaHasStrictStateEnum(t *testing.T) {
	srv := newServer(t, &fakeService{})
	cs := connect(t, srv, "staff-token")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "return_item" {
			continue
		}
		props := tool.InputSchema.(map[string]any)["properties"].(map[string]any)
		enum := props["state"].(map[string]any)["enum"].([]any)
		if len(enum) != 2 || enum[0] != "ok" || enum[1] != "damaged" {
			t.Fatalf("enum %v", enum)
		}
		if !strings.Contains(tool.Description, "MODIFIES STATE") {
			t.Fatal("return_item must announce its effect")
		}
		return
	}
	t.Fatal("return_item missing")
}

func TestAC11BorrowItemDescriptionAnnouncesEffect(t *testing.T) {
	srv := newServer(t, &fakeService{})
	cs := connect(t, srv, "staff-token")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name == "borrow_item" && !strings.Contains(tool.Description, "MODIFIES STATE") {
			t.Fatal("borrow_item must announce its effect")
		}
	}
}

func TestAC11EachToolCallsServiceWithTokenIdentity(t *testing.T) {
	tests := []struct {
		tool   string
		args   map[string]any
		method string
		check  func(*testing.T, call)
	}{
		{"list_available_items", map[string]any{}, "ListItems", nil},
		{"borrow_item", map[string]any{"item_id": "item-1", "borrower_sub": "student-9"}, "BorrowItem", func(t *testing.T, c call) {
			if c.borrow.ItemID != "item-1" || c.borrow.BorrowerSub != "student-9" {
				t.Fatalf("borrow cmd %+v", c.borrow)
			}
		}},
		{"return_item", map[string]any{"loan_id": "loan-1", "state": "damaged"}, "ReturnLoan", func(t *testing.T, c call) {
			if c.ret.LoanID != "loan-1" || c.ret.State != domain.ReturnDamaged {
				t.Fatalf("return cmd %+v", c.ret)
			}
		}},
		{"extend_loan", map[string]any{"loan_id": "loan-1"}, "ExtendLoan", func(t *testing.T, c call) {
			if c.loanID != "loan-1" {
				t.Fatalf("loan id %s", c.loanID)
			}
		}},
		{"my_loans", map[string]any{}, "MyLoans", nil},
	}
	for _, tc := range tests {
		t.Run(tc.tool, func(t *testing.T) {
			svc := &fakeService{}
			cs := connect(t, newServer(t, svc), "staff-token")
			res, err := callTool(t, cs, tc.tool, tc.args)
			if err != nil || res.IsError {
				t.Fatalf("err=%v res=%v", err, res)
			}
			c := svc.last(t)
			if c.method != tc.method || c.actor.Sub != "staff-1" || !c.actor.IsStaff() {
				t.Fatalf("call %+v", c)
			}
			if res.StructuredContent == nil {
				t.Fatal("no structured content")
			}
			if tc.check != nil {
				tc.check(t, c)
			}
		})
	}
}

func TestAC11DifferentTokensGiveDifferentIdentities(t *testing.T) {
	svc := &fakeService{}
	srv := newServer(t, svc)
	staff := connect(t, srv, "staff-token")
	student := connect(t, srv, "student-token")
	if _, err := callTool(t, staff, "my_loans", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	first := svc.last(t).actor
	if _, err := callTool(t, student, "my_loans", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	second := svc.last(t).actor
	if first.Sub != "staff-1" || second.Sub != "student-1" || second.IsStaff() {
		t.Fatalf("identities %+v %+v", first, second)
	}
}

func TestAC11InvalidInputRejectedBeforeService(t *testing.T) {
	tests := []struct {
		name string
		tool string
		args map[string]any
	}{
		{"state outside enum", "return_item", map[string]any{"loan_id": "loan-1", "state": "broken"}},
		{"missing state", "return_item", map[string]any{"loan_id": "loan-1"}},
		{"missing loan_id", "extend_loan", map[string]any{}},
		{"missing borrower_sub", "borrow_item", map[string]any{"item_id": "item-1"}},
		{"empty item_id", "borrow_item", map[string]any{"item_id": "", "borrower_sub": "s"}},
		{"unknown field", "extend_loan", map[string]any{"loan_id": "loan-1", "admin": true}},
		{"wrong type", "extend_loan", map[string]any{"loan_id": 42}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{}
			cs := connect(t, newServer(t, svc), "staff-token")
			res, err := callTool(t, cs, tc.tool, tc.args)
			if err == nil && (res == nil || !res.IsError) {
				t.Fatalf("expected rejection, got %v", res)
			}
			if svc.count() != 0 {
				t.Fatal("service reached with invalid input")
			}
		})
	}
}

func TestAC11DomainErrorBecomesIsErrorWithoutLeak(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		want    string
		notWant string
	}{
		{"forbidden", domain.ErrForbidden, "forbidden", ""},
		{"quota wrapped", errors.Join(domain.ErrQuotaExceeded, errors.New("SELECT count(*) FROM loans")), "quota", "SELECT"},
		{"unknown", errors.New("pq: connection refused 10.0.0.5:5432"), "internal error", "10.0.0.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeService{err: tc.err}
			cs := connect(t, newServer(t, svc), "staff-token")
			res, err := callTool(t, cs, "borrow_item", map[string]any{"item_id": "i", "borrower_sub": "s"})
			if err != nil {
				t.Fatal(err)
			}
			if !res.IsError {
				t.Fatal("expected IsError")
			}
			msg := text(res)
			if !strings.Contains(msg, tc.want) {
				t.Fatalf("message %q lacks %q", msg, tc.want)
			}
			if tc.notWant != "" && strings.Contains(msg, tc.notWant) {
				t.Fatalf("message leaks %q: %q", tc.notWant, msg)
			}
		})
	}
}

func TestAC11RequestWithoutBearerIsRefused(t *testing.T) {
	svc := &fakeService{}
	srv := newServer(t, svc)
	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if svc.count() != 0 {
		t.Fatal("service reached")
	}
}

func TestAC11FailingVerifierIsRefusedWithoutLeak(t *testing.T) {
	srv := newServer(t, &fakeService{})
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer garbage")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if strings.Contains(string(body), "internal-host") {
		t.Fatalf("leak: %s", body)
	}
}

func TestAC11ClientWithoutTokenCannotConnect(t *testing.T) {
	srv := newServer(t, &fakeService{})
	transport := &mcp.StreamableClientTransport{Endpoint: srv.URL, MaxRetries: -1}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	if cs, err := client.Connect(context.Background(), transport, nil); err == nil {
		_ = cs.Close()
		t.Fatal("expected connection refusal")
	}
}

func TestAC11ToolAnnotations(t *testing.T) {
	cs := connect(t, newServer(t, &fakeService{}), "staff-token")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		readOnly    bool
		destructive *bool
	}{
		"list_available_items": {true, nil},
		"my_loans":             {true, nil},
		"borrow_item":          {false, boolRef(true)},
		"return_item":          {false, boolRef(true)},
		"extend_loan":          {false, boolRef(false)},
	}
	for _, tool := range res.Tools {
		want := tests[tool.Name]
		a := tool.Annotations
		if a == nil || a.ReadOnlyHint != want.readOnly || a.IdempotentHint {
			t.Fatalf("%s: annotations %+v", tool.Name, a)
		}
		if (want.destructive == nil) != (a.DestructiveHint == nil) ||
			want.destructive != nil && *want.destructive != *a.DestructiveHint {
			t.Fatalf("%s: destructive hint %v", tool.Name, a.DestructiveHint)
		}
	}
}

func boolRef(b bool) *bool { return &b }

func TestAC11SensitiveToolsAskForConfirmation(t *testing.T) {
	cs := connect(t, newServer(t, &fakeService{}), "staff-token")
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		if tool.Name != "borrow_item" && tool.Name != "return_item" {
			continue
		}
		if !strings.Contains(tool.Description, "explicit confirmation") || !strings.Contains(tool.Description, "Never infer") {
			t.Fatalf("%s: missing confirmation guidance", tool.Name)
		}
	}
}

func TestAC11ServiceForbiddenBecomesIsErrorForbidden(t *testing.T) {
	svc := &fakeService{err: domain.ErrForbidden}
	cs := connect(t, newServer(t, svc), "student-token")
	res, err := callTool(t, cs, "return_item", map[string]any{"loan_id": "l", "state": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(text(res), "forbidden") {
		t.Fatalf("res %v", res)
	}
}
