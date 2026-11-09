package main

import (
	"time"

	"github.com/google/uuid"
)

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}

type uuidGenerator struct{}

func (uuidGenerator) NewID() string {
	return uuid.NewString()
}
