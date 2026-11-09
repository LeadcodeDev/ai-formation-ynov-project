package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpapi "github.com/ynov/equipment-loans/internal/adapters/http"
	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

type fakeService struct {
	err        error
	loan       domain.Loan
	loans      []domain.Loan
	items      []domain.ItemView
	actor      domain.Identity
	filter     domain.ItemFilter
	borrow     service.BorrowItem
	ret        service.ReturnLoan
	extendedID domain.LoanID
	calls      int
}

func (f *fakeService) ListItems(_ context.Context, a domain.Identity, filter domain.ItemFilter) ([]domain.ItemView, error) {
	f.calls++
	f.actor, f.filter = a, filter
	return f.items, f.err
}

func (f *fakeService) BorrowItem(_ context.Context, a domain.Identity, c service.BorrowItem) (domain.Loan, error) {
	f.calls++
	f.actor, f.borrow = a, c
	return f.loan, f.err
}

func (f *fakeService) ReturnLoan(_ context.Context, a domain.Identity, c service.ReturnLoan) (domain.Loan, error) {
	f.calls++
	f.actor, f.ret = a, c
	return f.loan, f.err
}

func (f *fakeService) ExtendLoan(_ context.Context, a domain.Identity, id domain.LoanID) (domain.Loan, error) {
	f.calls++
	f.actor, f.extendedID = a, id
	return f.loan, f.err
}

func (f *fakeService) MyLoans(_ context.Context, a domain.Identity) ([]domain.Loan, error) {
	f.calls++
	f.actor = a
	return f.loans, f.err
}

func (f *fakeService) OverdueLoans(_ context.Context, a domain.Identity) ([]domain.Loan, error) {
	f.calls++
	f.actor = a
	return f.loans, f.err
}

type fakeVerifier struct{ tokens map[string]domain.Identity }

func (v fakeVerifier) Verify(_ context.Context, token string) (domain.Identity, error) {
	id, ok := v.tokens[token]
	if !ok {
		return domain.Identity{}, errors.New("bad token")
	}
	return id, nil
}

var (
	student = domain.Identity{Sub: "stu-1", Roles: []domain.Role{domain.RoleStudent}}
	staff   = domain.Identity{Sub: "staff-1", Roles: []domain.Role{domain.RoleStaff}}
	noRole  = domain.Identity{Sub: "nobody"}
)

func setup(svc *fakeService) (http.Handler, *strings.Builder) {
	logs := &strings.Builder{}
	logger := slog.New(slog.NewTextHandler(logs, nil))
	verifier := fakeVerifier{tokens: map[string]domain.Identity{"student": student, "staff": staff, "norole": noRole}}
	return httpapi.NewHandler(svc, verifier, logger), logs
}

func do(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func decodeErr(t *testing.T, rec *httptest.ResponseRecorder) errBody {
	t.Helper()
	var e errBody
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil {
		t.Fatalf("body %q: %v", rec.Body.String(), err)
	}
	return e
}

func TestAC10Healthz(t *testing.T) {
	h, _ := setup(&fakeService{})
	rec := do(h, http.MethodGet, "/healthz", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
		t.Fatalf("body = %s", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %s", ct)
	}
}

type route struct{ method, path, body string }

var protectedRoutes = []route{
	{"GET", "/items", ""},
	{"POST", "/loans", `{"item_id":"i","borrower_sub":"b"}`},
	{"POST", "/loans/l1/return", `{"state":"ok"}`},
	{"POST", "/loans/l1/extend", ""},
	{"GET", "/loans/me", ""},
	{"GET", "/loans/overdue", ""},
}

func TestAC9Unauthenticated(t *testing.T) {
	tests := []struct {
		name  string
		token string
		auth  string
	}{
		{"no header", "", ""},
		{"invalid token", "bogus", ""},
		{"wrong scheme", "", "Basic abc"},
		{"empty bearer", "", "Bearer "},
	}
	for _, r := range protectedRoutes {
		for _, tt := range tests {
			t.Run(r.method+" "+r.path+" "+tt.name, func(t *testing.T) {
				svc := &fakeService{}
				h, _ := setup(svc)
				req := httptest.NewRequest(r.method, r.path, strings.NewReader(r.body))
				switch {
				case tt.token != "":
					req.Header.Set("Authorization", "Bearer "+tt.token)
				case tt.auth != "":
					req.Header.Set("Authorization", tt.auth)
				}
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != http.StatusUnauthorized {
					t.Fatalf("status = %d", rec.Code)
				}
				if rec.Header().Get("WWW-Authenticate") != "Bearer" {
					t.Fatalf("WWW-Authenticate = %q", rec.Header().Get("WWW-Authenticate"))
				}
				if decodeErr(t, rec).Code != "unauthenticated" {
					t.Fatalf("body = %s", rec.Body.String())
				}
				if svc.calls != 0 {
					t.Fatal("service called")
				}
			})
		}
	}
}

func TestAC9TokenWithoutRoleIsForbidden(t *testing.T) {
	for _, r := range protectedRoutes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			svc := &fakeService{}
			h, _ := setup(svc)
			rec := do(h, r.method, r.path, "norole", r.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d", rec.Code)
			}
			if decodeErr(t, rec).Code != "forbidden" {
				t.Fatalf("body = %s", rec.Body.String())
			}
			if svc.calls != 0 {
				t.Fatal("service called")
			}
		})
	}
}

func TestAC9AC10ErrorMapping(t *testing.T) {
	tests := []struct {
		err    error
		status int
		code   string
	}{
		{domain.ErrInvalidInput, 400, "invalid_input"},
		{domain.ErrForbidden, 403, "forbidden"},
		{domain.ErrModelNotFound, 404, "model_not_found"},
		{domain.ErrItemNotFound, 404, "item_not_found"},
		{domain.ErrLoanNotFound, 404, "loan_not_found"},
		{domain.ErrItemNotBorrowable, 409, "item_not_borrowable"},
		{domain.ErrItemAlreadyBorrowed, 409, "item_already_borrowed"},
		{domain.ErrQuotaExceeded, 409, "quota_exceeded"},
		{domain.ErrBorrowerHasOverdueLoan, 409, "borrower_has_overdue_loan"},
		{domain.ErrLoanAlreadyReturned, 409, "loan_already_returned"},
		{domain.ErrLoanOverdue, 409, "loan_overdue"},
		{domain.ErrExtensionLimitReached, 409, "extension_limit_reached"},
		{fmt.Errorf("secret-detail: %w", domain.ErrQuotaExceeded), 409, "quota_exceeded"},
		{errors.New("secret db password leaked"), 500, "internal"},
	}
	for _, tt := range tests {
		for _, r := range protectedRoutes {
			t.Run(tt.code+" "+r.method+" "+r.path, func(t *testing.T) {
				svc := &fakeService{err: tt.err}
				h, logs := setup(svc)
				rec := do(h, r.method, r.path, "staff", r.body)
				if rec.Code != tt.status {
					t.Fatalf("status = %d, want %d", rec.Code, tt.status)
				}
				e := decodeErr(t, rec)
				if e.Code != tt.code {
					t.Fatalf("code = %q, want %q", e.Code, tt.code)
				}
				if strings.Contains(e.Message, "secret") {
					t.Fatalf("detail leaked in message %q", e.Message)
				}
				if tt.status == 500 {
					if strings.Contains(rec.Body.String(), "secret") {
						t.Fatal("internal detail leaked")
					}
					if !strings.Contains(logs.String(), "secret db password leaked") {
						t.Fatalf("error not logged: %s", logs.String())
					}
				}
			})
		}
	}
}

func sampleLoan() domain.Loan {
	paris, _ := time.LoadLocation("Europe/Paris")
	return domain.Loan{
		ID:          "l1",
		ItemID:      "i1",
		BorrowerSub: "stu-1",
		BorrowedAt:  time.Date(2026, 1, 5, 10, 0, 0, 0, paris),
		DueAt:       time.Date(2026, 1, 13, 0, 0, 0, 0, paris),
	}
}

func TestAC10LoanJSON(t *testing.T) {
	returned := time.Date(2026, 1, 6, 9, 0, 0, 0, time.UTC)
	state := domain.ReturnDamaged
	returnedLoan := sampleLoan()
	returnedLoan.ReturnedAt = &returned
	returnedLoan.ReturnState = &state
	returnedLoan.ExtensionsUsed = 1

	tests := []struct {
		name string
		loan domain.Loan
		want string
	}{
		{"active", sampleLoan(), `{"id":"l1","item_id":"i1","borrower_sub":"stu-1","borrowed_at":"2026-01-05T09:00:00Z","due_at":"2026-01-12T23:00:00Z","returned_at":null,"return_state":null,"extensions_used":0}`},
		{"returned", returnedLoan, `{"id":"l1","item_id":"i1","borrower_sub":"stu-1","borrowed_at":"2026-01-05T09:00:00Z","due_at":"2026-01-12T23:00:00Z","returned_at":"2026-01-06T09:00:00Z","return_state":"damaged","extensions_used":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := setup(&fakeService{loan: tt.loan})
			rec := do(h, http.MethodPost, "/loans/l1/extend", "student", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.want {
				t.Fatalf("got  %s\nwant %s", got, tt.want)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content-type = %s", ct)
			}
		})
	}
}

func TestAC10ItemsJSONAndFilter(t *testing.T) {
	views := []domain.ItemView{{
		Item:  domain.Item{ID: "i1", ModelID: "m1", InventoryNumber: "INV-1", Status: domain.ItemAvailable},
		Model: domain.EquipmentModel{ID: "m1", Name: "Laptop", Category: "portable", MaxLoanDays: 7},
	}}
	tests := []struct {
		query      string
		wantOnly   bool
		wantStatus int
	}{
		{"", false, 200},
		{"?available=true", true, 200},
		{"?available=false", false, 200},
		{"?available=maybe", false, 400},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			svc := &fakeService{items: views}
			h, _ := setup(svc)
			rec := do(h, http.MethodGet, "/items"+tt.query, "student", "")
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d", rec.Code)
			}
			if tt.wantStatus != 200 {
				if decodeErr(t, rec).Code != "invalid_input" {
					t.Fatalf("body = %s", rec.Body.String())
				}
				return
			}
			want := `[{"id":"i1","inventory_number":"INV-1","status":"available","model":{"id":"m1","name":"Laptop","category":"portable","max_loan_days":7}}]`
			if got := strings.TrimSpace(rec.Body.String()); got != want {
				t.Fatalf("got %s", got)
			}
			if svc.filter.OnlyAvailable != tt.wantOnly {
				t.Fatalf("filter = %+v", svc.filter)
			}
		})
	}
}

func TestAC10EmptyListsAreArrays(t *testing.T) {
	for _, path := range []string{"/items", "/loans/me", "/loans/overdue"} {
		t.Run(path, func(t *testing.T) {
			h, _ := setup(&fakeService{})
			rec := do(h, http.MethodGet, path, "staff", "")
			if rec.Code != 200 {
				t.Fatalf("status = %d", rec.Code)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
				t.Fatalf("body = %s", got)
			}
		})
	}
}

func TestAC9MyLoansForwardsIdentity(t *testing.T) {
	svc := &fakeService{loans: []domain.Loan{sampleLoan()}}
	h, _ := setup(svc)
	rec := do(h, http.MethodGet, "/loans/me", "student", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if svc.actor.Sub != "stu-1" || !svc.actor.Has(domain.RoleStudent) {
		t.Fatalf("actor = %+v", svc.actor)
	}
}

func TestAC9StudentBorrowerSubNotFilteredByHandler(t *testing.T) {
	svc := &fakeService{err: domain.ErrForbidden}
	h, _ := setup(svc)
	rec := do(h, http.MethodPost, "/loans", "student", `{"item_id":"i1","borrower_sub":"other"}`)
	if svc.calls != 1 {
		t.Fatalf("service calls = %d, want 1", svc.calls)
	}
	if svc.actor.Sub != "stu-1" || svc.borrow.BorrowerSub != "other" || svc.borrow.ItemID != "i1" {
		t.Fatalf("actor=%+v cmd=%+v", svc.actor, svc.borrow)
	}
	if rec.Code != http.StatusForbidden || decodeErr(t, rec).Code != "forbidden" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAC10BorrowAndReturnRequests(t *testing.T) {
	svc := &fakeService{loan: sampleLoan()}
	h, _ := setup(svc)
	rec := do(h, http.MethodPost, "/loans", "staff", `{"item_id":"i1","borrower_sub":"stu-1"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("borrow status = %d", rec.Code)
	}
	rec = do(h, http.MethodPost, "/loans/l9/return", "staff", `{"state":"damaged"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("return status = %d", rec.Code)
	}
	if svc.ret.LoanID != "l9" || svc.ret.State != domain.ReturnDamaged {
		t.Fatalf("cmd = %+v", svc.ret)
	}
	do(h, http.MethodPost, "/loans/l7/extend", "student", "")
	if svc.extendedID != "l7" {
		t.Fatalf("extended = %q", svc.extendedID)
	}
}

func TestAC10InvalidBodies(t *testing.T) {
	big := `{"item_id":"` + strings.Repeat("a", 2<<20) + `","borrower_sub":"b"}`
	tests := []struct {
		name, path, body string
		status           int
	}{
		{"borrow unknown field", "/loans", `{"item_id":"i","borrower_sub":"b","x":1}`, 400},
		{"borrow malformed", "/loans", `{`, 400},
		{"borrow empty", "/loans", ``, 400},
		{"borrow trailing", "/loans", `{"item_id":"i","borrower_sub":"b"}{}`, 400},
		{"borrow wrong type", "/loans", `{"item_id":1,"borrower_sub":"b"}`, 400},
		{"borrow too large", "/loans", big, 413},
		{"return unknown field", "/loans/l1/return", `{"state":"ok","x":1}`, 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &fakeService{}
			h, _ := setup(svc)
			rec := do(h, http.MethodPost, tt.path, "staff", tt.body)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if decodeErr(t, rec).Code != "invalid_input" {
				t.Fatalf("body = %s", rec.Body.String())
			}
			if svc.calls != 0 {
				t.Fatal("service called")
			}
		})
	}
}

func TestAC10OverdueRouteNotShadowed(t *testing.T) {
	svc := &fakeService{loans: []domain.Loan{sampleLoan()}}
	h, _ := setup(svc)
	rec := do(h, http.MethodGet, "/loans/overdue", "staff", "")
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.HasPrefix(string(body), "[{") {
		t.Fatalf("body = %s", body)
	}
}

func TestAC9StudentInvalidReturnStateGets403(t *testing.T) {
	svc := &fakeService{err: domain.ErrForbidden}
	h, _ := setup(svc)
	rec := do(h, http.MethodPost, "/loans/l1/return", "student", `{"state":"foo"}`)
	if rec.Code != http.StatusForbidden || decodeErr(t, rec).Code != "forbidden" {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if svc.ret.State != "foo" || svc.actor.Sub != "stu-1" {
		t.Fatalf("actor=%+v cmd=%+v", svc.actor, svc.ret)
	}
}

func TestAC10InvalidReturnStateReachesService(t *testing.T) {
	svc := &fakeService{err: domain.ErrInvalidInput}
	h, _ := setup(svc)
	rec := do(h, http.MethodPost, "/loans/l1/return", "staff", `{"state":"broken"}`)
	if rec.Code != http.StatusBadRequest || svc.calls != 1 {
		t.Fatalf("status=%d calls=%d", rec.Code, svc.calls)
	}
}

func TestAC10DecoderDetailNotLeaked(t *testing.T) {
	h, logs := setup(&fakeService{})
	rec := do(h, http.MethodPost, "/loans", "staff", `{"item_id":"i","zzz_unknown":1}`)
	if strings.Contains(rec.Body.String(), "zzz_unknown") {
		t.Fatalf("leak: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "zzz_unknown") {
		t.Fatalf("detail not logged: %s", logs.String())
	}
}

func TestAC10SecurityHeaders(t *testing.T) {
	cases := []struct {
		method, path, token string
	}{
		{"GET", "/healthz", ""},
		{"GET", "/items", ""},
		{"GET", "/items", "student"},
		{"GET", "/loans/me", "norole"},
		{"GET", "/nope", ""},
		{"DELETE", "/loans/me", ""},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path+" "+c.token, func(t *testing.T) {
			h, _ := setup(&fakeService{})
			rec := do(h, c.method, c.path, c.token, "")
			if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("headers = %v", rec.Header())
			}
		})
	}
}

func TestAC10UnknownRoutesReturnJSON(t *testing.T) {
	tests := []struct {
		method, path string
		status       int
		allow        string
	}{
		{"GET", "/nope", 404, ""},
		{"DELETE", "/loans/me", 405, "GET"},
		{"GET", "/loans", 405, "POST"},
		{"PUT", "/healthz", 405, "GET"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			h, _ := setup(&fakeService{})
			rec := do(h, tt.method, tt.path, "staff", "")
			if rec.Code != tt.status {
				t.Fatalf("status = %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("content-type = %s", ct)
			}
			if decodeErr(t, rec).Code != "invalid_input" {
				t.Fatalf("body = %s", rec.Body.String())
			}
			if tt.allow != "" && !strings.Contains(rec.Header().Get("Allow"), tt.allow) {
				t.Fatalf("allow = %q", rec.Header().Get("Allow"))
			}
		})
	}
}
