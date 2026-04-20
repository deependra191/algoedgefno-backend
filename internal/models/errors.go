package models

import "errors"

// ErrNotFound is returned by repository methods when the requested record does not exist.
// Services check for this to distinguish "not found" from unexpected DB errors.
var ErrNotFound = errors.New("not found")
