package app

import "errors"

// Sentinel errors shared between the application layer, the persistence
// adapter and the HTTP adapter. Adapters wrap them with context via
// fmt.Errorf("...: %w", Err...) and the HTTP layer maps them to status codes.
var (
	// ErrNotFound: the resource does not exist within the tenant.
	ErrNotFound = errors.New("app: not found")
	// ErrConflict: a uniqueness constraint was violated.
	ErrConflict = errors.New("app: conflict")
	// ErrInvalidState: the operation is not allowed in the current lifecycle
	// state (e.g. posting a reversed transaction, capturing a released hold).
	ErrInvalidState = errors.New("app: invalid state")
	// ErrInsufficientFunds: the available balance cannot cover a hold.
	ErrInsufficientFunds = errors.New("app: insufficient funds")
	// ErrValidation: the request payload is semantically invalid.
	ErrValidation = errors.New("app: validation failed")
)
