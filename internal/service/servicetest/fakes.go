package servicetest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ynov/equipment-loans/internal/domain"
	"github.com/ynov/equipment-loans/internal/ports"
)

type Clock struct {
	mu  sync.Mutex
	now time.Time
}

func NewClock(now time.Time) *Clock {
	return &Clock{now: now}
}

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) Set(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

type IDs struct {
	mu sync.Mutex
	n  int
}

func (g *IDs) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return fmt.Sprintf("loan-%d", g.n)
}

type Store struct {
	mu     sync.Mutex
	Models map[domain.ModelID]domain.EquipmentModel
	Items  map[domain.ItemID]domain.Item
	Loans  map[domain.LoanID]domain.Loan
	Locks  []string
}

func NewStore() *Store {
	return &Store{
		Models: map[domain.ModelID]domain.EquipmentModel{},
		Items:  map[domain.ItemID]domain.Item{},
		Loans:  map[domain.LoanID]domain.Loan{},
	}
}

func (s *Store) AddModel(m domain.EquipmentModel) {
	s.Models[m.ID] = m
}

func (s *Store) AddItem(i domain.Item) {
	s.Items[i.ID] = i
}

func (s *Store) AddLoan(l domain.Loan) {
	s.Loans[l.ID] = l
}

func (s *Store) InTx(ctx context.Context, fn func(ctx context.Context, repos ports.Repositories) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := cloneMap(s.Items)
	loans := cloneMap(s.Loans)
	repos := ports.Repositories{Items: &itemRepo{s: s}, Loans: &loanRepo{s: s}}
	if err := fn(ctx, repos); err != nil {
		s.Items, s.Loans = items, loans
		return err
	}
	return nil
}

func cloneMap[K comparable, V any](m map[K]V) map[K]V {
	out := make(map[K]V, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

type itemRepo struct {
	s *Store
}

type loanRepo struct {
	s *Store
}

func (r *itemRepo) List(_ context.Context, filter domain.ItemFilter) ([]domain.ItemView, error) {
	var out []domain.ItemView
	for _, it := range r.s.Items {
		if filter.OnlyAvailable && it.Status != domain.ItemAvailable {
			continue
		}
		out = append(out, domain.ItemView{Item: it, Model: r.s.Models[it.ModelID]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Item.ID < out[j].Item.ID })
	return out, nil
}

func (r *itemRepo) GetForUpdate(_ context.Context, id domain.ItemID) (domain.Item, error) {
	it, ok := r.s.Items[id]
	if !ok {
		return domain.Item{}, domain.ErrItemNotFound
	}
	return it, nil
}

func (r *itemRepo) UpdateStatus(_ context.Context, id domain.ItemID, status domain.ItemStatus) error {
	it, ok := r.s.Items[id]
	if !ok {
		return domain.ErrItemNotFound
	}
	it.Status = status
	r.s.Items[id] = it
	return nil
}

func (r *itemRepo) GetModel(_ context.Context, id domain.ModelID) (domain.EquipmentModel, error) {
	m, ok := r.s.Models[id]
	if !ok {
		return domain.EquipmentModel{}, domain.ErrModelNotFound
	}
	return m, nil
}

func (r *loanRepo) Insert(_ context.Context, loan domain.Loan) error {
	for _, l := range r.s.Loans {
		if l.ItemID == loan.ItemID && l.Active() {
			return domain.ErrItemAlreadyBorrowed
		}
	}
	r.s.Loans[loan.ID] = loan
	return nil
}

func (r *loanRepo) Update(_ context.Context, loan domain.Loan) error {
	if _, ok := r.s.Loans[loan.ID]; !ok {
		return domain.ErrLoanNotFound
	}
	r.s.Loans[loan.ID] = loan
	return nil
}

func (r *loanRepo) GetForUpdate(_ context.Context, id domain.LoanID) (domain.Loan, error) {
	l, ok := r.s.Loans[id]
	if !ok {
		return domain.Loan{}, domain.ErrLoanNotFound
	}
	return l, nil
}

func (r *loanRepo) LockBorrower(_ context.Context, sub string) error {
	r.s.Locks = append(r.s.Locks, sub)
	return nil
}

func (r *loanRepo) ListActiveByBorrower(_ context.Context, sub string) ([]domain.Loan, error) {
	return r.filter(func(l domain.Loan) bool { return l.BorrowerSub == sub && l.Active() }), nil
}

func (r *loanRepo) ListByBorrower(_ context.Context, sub string) ([]domain.Loan, error) {
	return r.filter(func(l domain.Loan) bool { return l.BorrowerSub == sub }), nil
}

func (r *loanRepo) ListActive(_ context.Context) ([]domain.Loan, error) {
	return r.filter(func(l domain.Loan) bool { return l.Active() }), nil
}

func (r *loanRepo) filter(keep func(domain.Loan) bool) []domain.Loan {
	var out []domain.Loan
	for _, l := range r.s.Loans {
		if keep(l) {
			out = append(out, l)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
