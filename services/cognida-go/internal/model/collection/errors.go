package collection

import "errors"

var (
	ErrInvalidTask       = errors.New("invalid collection task")
	ErrInvalidSource     = errors.New("invalid collection source")
	ErrInvalidTransition = errors.New("invalid collection lifecycle transition")
	ErrTaskNotFound      = errors.New("collection task not found")
	ErrRunNotFound       = errors.New("collection run not found")
	ErrItemNotFound      = errors.New("collection item not found")
	ErrActiveRunExists   = errors.New("collection task already has an active run")
	ErrRepository        = errors.New("collection repository error")
)
