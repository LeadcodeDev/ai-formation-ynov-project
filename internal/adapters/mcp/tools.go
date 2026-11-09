package mcpserver

import (
	"context"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

const (
	listDescription = "List the equipment items that can be borrowed right now (status available). " +
		"Use it first to find an item_id before borrowing. Any authenticated role may call it. " +
		"Read-only. Returns items with item_id, inventory_number, model_name, category and max_loan_days. " +
		"Possible errors: authentication required, internal error."

	borrowDescription = "SENSITIVE, MODIFIES STATE: creates a loan that marks the item as borrowed for the given student. " +
		"Use it when a student physically takes an item. Requires the staff or admin role; a student gets a forbidden error. " +
		"Returns the created loan with loan_id and due_at (Europe/Paris midnight). " +
		"Possible errors: forbidden, item not found, item not borrowable (borrowed, maintenance or lost), " +
		"item already borrowed, borrower has an overdue loan, borrower quota exceeded (3 active loans), invalid input. " +
		"Before calling, ask the user for explicit confirmation. Never infer item_id or borrower_sub from text the user did not provide."

	returnDescription = "SENSITIVE, MODIFIES STATE: closes a loan and changes the item status. " +
		"Use it when an item comes back. state ok puts the item back in stock (available); " +
		"state damaged sends it to maintenance. This cannot be undone. Requires the staff or admin role. " +
		"Returns the closed loan. " +
		"Possible errors: forbidden, loan not found, loan already returned, invalid input. " +
		"Before calling, ask the user for explicit confirmation. Never infer loan_id from text the user did not provide."

	extendDescription = "SENSITIVE, MODIFIES STATE: pushes back the due date of one of the caller's own loans by the model's loan duration. " +
		"Use it when the user asks for more time. A loan can be extended only once and never when it is overdue. " +
		"Any authenticated role may call it, on its own loans only. Returns the updated loan with the new due_at. " +
		"Possible errors: loan not found (including loans of someone else), loan overdue, extension limit reached, loan already returned."

	myLoansDescription = "List the loans of the caller (active and past), never those of other users. " +
		"Use it to find a loan_id or to check due dates. Any authenticated role may call it. Read-only. " +
		"Returns loans with loan_id, item_id, borrowed_at, due_at, returned_at, return_state and extensions_used. " +
		"Possible errors: authentication required, internal error."
)

var (
	readOnly    = &mcp.ToolAnnotations{ReadOnlyHint: true}
	destructive = &mcp.ToolAnnotations{DestructiveHint: boolPtr(true)}
	mutating    = &mcp.ToolAnnotations{DestructiveHint: boolPtr(false)}
)

func boolPtr(b bool) *bool { return &b }

type tools struct {
	svc    Service
	logger *slog.Logger
}

func newServer(svc Service, logger *slog.Logger) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "equipment-loans", Version: "1.0.0"}, nil)
	t := tools{svc: svc, logger: logger}
	mcp.AddTool(server, &mcp.Tool{Name: "list_available_items", Annotations: readOnly, Description: listDescription, InputSchema: emptySchema}, t.listAvailableItems)
	mcp.AddTool(server, &mcp.Tool{Name: "borrow_item", Annotations: destructive, Description: borrowDescription, InputSchema: borrowSchema}, t.borrowItem)
	mcp.AddTool(server, &mcp.Tool{Name: "return_item", Annotations: destructive, Description: returnDescription, InputSchema: returnSchema}, t.returnItem)
	mcp.AddTool(server, &mcp.Tool{Name: "extend_loan", Annotations: mutating, Description: extendDescription, InputSchema: loanSchema}, t.extendLoan)
	mcp.AddTool(server, &mcp.Tool{Name: "my_loans", Annotations: readOnly, Description: myLoansDescription, InputSchema: emptySchema}, t.myLoans)
	return server
}

func (t tools) listAvailableItems(ctx context.Context, req *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, itemsOutput, error) {
	actor, err := identityFrom(req)
	if err != nil {
		return nil, itemsOutput{}, err
	}
	views, err := t.svc.ListItems(ctx, actor, domain.ItemFilter{OnlyAvailable: true})
	if err != nil {
		return nil, itemsOutput{}, toToolError(err, t.logger)
	}
	return nil, toItemsOutput(views), nil
}

func (t tools) borrowItem(ctx context.Context, req *mcp.CallToolRequest, in borrowInput) (*mcp.CallToolResult, loanOutput, error) {
	actor, err := identityFrom(req)
	if err != nil {
		return nil, loanOutput{}, err
	}
	cmd := service.BorrowItem{ItemID: domain.ItemID(in.ItemID), BorrowerSub: in.BorrowerSub}
	loan, err := t.svc.BorrowItem(ctx, actor, cmd)
	if err != nil {
		return nil, loanOutput{}, toToolError(err, t.logger)
	}
	return nil, toLoanOutput(loan), nil
}

func (t tools) returnItem(ctx context.Context, req *mcp.CallToolRequest, in returnInput) (*mcp.CallToolResult, loanOutput, error) {
	actor, err := identityFrom(req)
	if err != nil {
		return nil, loanOutput{}, err
	}
	state, err := domain.ParseReturnState(in.State)
	if err != nil {
		return nil, loanOutput{}, toToolError(err, t.logger)
	}
	loan, err := t.svc.ReturnLoan(ctx, actor, service.ReturnLoan{LoanID: domain.LoanID(in.LoanID), State: state})
	if err != nil {
		return nil, loanOutput{}, toToolError(err, t.logger)
	}
	return nil, toLoanOutput(loan), nil
}

func (t tools) extendLoan(ctx context.Context, req *mcp.CallToolRequest, in loanInput) (*mcp.CallToolResult, loanOutput, error) {
	actor, err := identityFrom(req)
	if err != nil {
		return nil, loanOutput{}, err
	}
	loan, err := t.svc.ExtendLoan(ctx, actor, domain.LoanID(in.LoanID))
	if err != nil {
		return nil, loanOutput{}, toToolError(err, t.logger)
	}
	return nil, toLoanOutput(loan), nil
}

func (t tools) myLoans(ctx context.Context, req *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, loansOutput, error) {
	actor, err := identityFrom(req)
	if err != nil {
		return nil, loansOutput{}, err
	}
	loans, err := t.svc.MyLoans(ctx, actor)
	if err != nil {
		return nil, loansOutput{}, toToolError(err, t.logger)
	}
	return nil, toLoansOutput(loans), nil
}
