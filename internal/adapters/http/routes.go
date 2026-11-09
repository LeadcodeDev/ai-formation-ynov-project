package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/service"
)

const maxBodyBytes = 1 << 20

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func decodeBody(w http.ResponseWriter, r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("%w: invalid request body: %w", domain.ErrInvalidInput, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing data")
		}
		return fmt.Errorf("%w: invalid request body: %w", domain.ErrInvalidInput, err)
	}
	return nil
}

func (h *handler) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) listItems(w http.ResponseWriter, r *http.Request) {
	filter := domain.ItemFilter{}
	if raw := r.URL.Query().Get("available"); raw != "" {
		only, err := strconv.ParseBool(raw)
		if err != nil {
			h.fail(w, r, fmt.Errorf("%w: available must be a boolean", domain.ErrInvalidInput))
			return
		}
		filter.OnlyAvailable = only
	}
	views, err := h.svc.ListItems(r.Context(), identityFrom(r.Context()), filter)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toItemDTOs(views))
}

func (h *handler) borrowItem(w http.ResponseWriter, r *http.Request) {
	var req borrowRequest
	if err := decodeBody(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	cmd := service.BorrowItem{ItemID: domain.ItemID(req.ItemID), BorrowerSub: req.BorrowerSub}
	loan, err := h.svc.BorrowItem(r.Context(), identityFrom(r.Context()), cmd)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toLoanDTO(loan))
}

func (h *handler) returnLoan(w http.ResponseWriter, r *http.Request) {
	var req returnRequest
	if err := decodeBody(w, r, &req); err != nil {
		h.fail(w, r, err)
		return
	}
	cmd := service.ReturnLoan{LoanID: domain.LoanID(r.PathValue("id")), State: domain.ReturnState(req.State)}
	loan, err := h.svc.ReturnLoan(r.Context(), identityFrom(r.Context()), cmd)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoanDTO(loan))
}

func (h *handler) extendLoan(w http.ResponseWriter, r *http.Request) {
	id := domain.LoanID(r.PathValue("id"))
	loan, err := h.svc.ExtendLoan(r.Context(), identityFrom(r.Context()), id)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoanDTO(loan))
}

func (h *handler) myLoans(w http.ResponseWriter, r *http.Request) {
	loans, err := h.svc.MyLoans(r.Context(), identityFrom(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoanDTOs(loans))
}

func (h *handler) overdueLoans(w http.ResponseWriter, r *http.Request) {
	loans, err := h.svc.OverdueLoans(r.Context(), identityFrom(r.Context()))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toLoanDTOs(loans))
}
