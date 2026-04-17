package persistence

import "errors"

// ErrNotFound is returned when a requested entity does not exist in the store.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when a write operation violates a uniqueness or
// referential-integrity constraint (e.g. duplicate name within a cluster).
var ErrConflict = errors.New("conflict")

// ErrValidation is returned when the caller supplies invalid or incomplete
// data that cannot be stored (e.g. an empty required field).
var ErrValidation = errors.New("validation")
