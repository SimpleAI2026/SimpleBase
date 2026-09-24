package auth

import "github.com/google/uuid"

func newID() string { return uuid.NewString() }
