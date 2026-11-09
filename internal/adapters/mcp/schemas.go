package mcpserver

func objectSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "description": description}
}

var emptySchema = objectSchema([]string{}, map[string]any{})

var borrowSchema = objectSchema([]string{"item_id", "borrower_sub"}, map[string]any{
	"item_id":      stringProperty("Identifier of the item to lend, taken from the item_id field returned by list_available_items."),
	"borrower_sub": stringProperty("Subject (sub) identifier of the student who borrows the item. It is not the staff member's own identifier."),
})

var returnSchema = objectSchema([]string{"loan_id", "state"}, map[string]any{
	"loan_id": stringProperty("Identifier of the active loan being closed, taken from the loan_id field returned by borrow_item or my_loans."),
	"state": map[string]any{
		"type":        "string",
		"enum":        []string{"ok", "damaged"},
		"description": "Condition of the returned item. ok puts the item back in stock; damaged sends it to maintenance and removes it from stock.",
	},
})

var loanSchema = objectSchema([]string{"loan_id"}, map[string]any{
	"loan_id": stringProperty("Identifier of one of your own active loans, taken from the loan_id field returned by my_loans."),
})
