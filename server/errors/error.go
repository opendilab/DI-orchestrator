package errors

import (
	"errors"
)

type NerveXError struct {
	Type    ErrorType `json:"type"`
	Message string    `json:"message"`
}

func (n *NerveXError) Error() string {
	return n.Message
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
)

func IsNotFound(err error) bool {
	return TypeForError(err) == ErrorNotFound
}

func IsAlreadyExists(err error) bool {
	return TypeForError(err) == ErrorAlreadyExists
}

func IsBadRequest(err error) bool {
	return TypeForError(err) == ErrorBadRequest
}

func TypeForError(err error) ErrorType {
	var nvxErr *NerveXError
	if errors.As(err, &nvxErr) {
		return nvxErr.Type
	}
	return ErrorUnknown
}
