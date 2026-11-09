package domain

import (
	"time"
	_ "time/tzdata"
)

var parisLocation = mustLoadParis()

func mustLoadParis() *time.Location {
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		panic(err)
	}
	return loc
}

func DueAt(borrowedAt time.Time, maxLoanDays int) time.Time {
	y, m, d := borrowedAt.In(parisLocation).Date()
	return time.Date(y, m, d+maxLoanDays+1, 0, 0, 0, 0, parisLocation)
}

func ExtendedDueAt(dueAt time.Time, maxLoanDays int) time.Time {
	y, m, d := dueAt.In(parisLocation).Date()
	return time.Date(y, m, d+maxLoanDays, 0, 0, 0, 0, parisLocation)
}

func IsOverdue(l Loan, now time.Time) bool {
	return now.After(l.DueAt)
}
