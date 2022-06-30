package types

import (
	"errors"
)

type DIError struct {
	Type    ErrorType `json:"type"`
	Message error     `json:"message"`
}

func (n *DIError) Error() string {
	return n.Message.Error()
}

type ErrorType string

const (
	// StatusCode = 500
	ErrorUnknown ErrorType = "Unknown"

	// StatusCode = 404
	ErrorNotFound ErrorType = "NotFound"

	// StatusCode = 409
	ErrorAlreadyExists ErrorType = "AlreadyExists"

	// StatusCode = 400
	ErrorBadRequest ErrorType = "BadRequest"

	// StatusCode = 501
	ErrorNotImplemented ErrorType = "NotImplemented"
)

func NewNotFoundError(msg error) *DIError {
	return &DIError{
		Type:    ErrorNotFound,
		Message: msg,
	}
}

func NewAlreadyExistsError(msg error) *DIError {
	return &DIError{
		Type:    ErrorAlreadyExists,
		Message: msg,
	}
}

func NewBadRequestError(msg error) *DIError {
	return &DIError{
		Type:    ErrorBadRequest,
		Message: msg,
	}
}

func NewNotImplementedError(msg error) *DIError {
	return &DIError{
		Type:    ErrorNotImplemented,
		Message: msg,
	}
}

func IsNotFound(err error) bool {
	return TypeForError(err) == ErrorNotFound
}

func IsAlreadyExists(err error) bool {
	return TypeForError(err) == ErrorAlreadyExists
}

func IsBadRequest(err error) bool {
	return TypeForError(err) == ErrorBadRequest
}

func IsNotImplemented(err error) bool {
	return TypeForError(err) == ErrorNotImplemented
}

func TypeForError(err error) ErrorType {
	var diErr *DIError
	if errors.As(err, &diErr) {
		return diErr.Type
	}
	return ErrorUnknown
}
