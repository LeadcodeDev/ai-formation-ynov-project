package domain

import "fmt"

type Role string

const (
	RoleStudent Role = "student"
	RoleStaff   Role = "staff"
	RoleAdmin   Role = "admin"
)

func ParseRole(s string) (Role, bool) {
	switch r := Role(s); r {
	case RoleStudent, RoleStaff, RoleAdmin:
		return r, true
	}
	return "", false
}

type Identity struct {
	Sub   string
	Roles []Role
}

func NewIdentity(sub string, roles []Role) (Identity, error) {
	if sub == "" {
		return Identity{}, fmt.Errorf("%w: empty subject", ErrInvalidInput)
	}
	return Identity{Sub: sub, Roles: roles}, nil
}

func (i Identity) Has(role Role) bool {
	for _, r := range i.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func (i Identity) IsStaff() bool {
	return i.Has(RoleStaff) || i.Has(RoleAdmin)
}
