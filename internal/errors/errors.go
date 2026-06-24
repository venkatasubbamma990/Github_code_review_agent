package errors

import (
	"errors"
	"fmt"
	"net/http"
)

type AppError struct {
	Code       string
	Message    string
	HTTPStatus int
	Cause      error
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func (e *AppError) ClientMessage() string {
	return e.Message
}

func AsAppError(err error) (*AppError, bool) {
	var ae *AppError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}

var (
	ErrInvalidRequest = &AppError{Code: "INVALID_REQUEST", Message: "Invalid request", HTTPStatus: http.StatusBadRequest}
	ErrUnauthorized   = &AppError{Code: "UNAUTHORIZED", Message: "Unauthorized", HTTPStatus: http.StatusUnauthorized}
	ErrNotFound       = &AppError{Code: "NOT_FOUND", Message: "Resource not found", HTTPStatus: http.StatusNotFound}
	ErrInternal       = &AppError{Code: "INTERNAL", Message: "Internal server error", HTTPStatus: http.StatusInternalServerError}
	ErrReviewFailed   = &AppError{Code: "REVIEW_FAILED", Message: "Code review failed", HTTPStatus: http.StatusBadGateway}
)

func WithMessage(base *AppError, msg string) *AppError {
	return &AppError{
		Code:       base.Code,
		Message:    msg,
		HTTPStatus: base.HTTPStatus,
	}
}

func WithCause(base *AppError, cause error) *AppError {
	return &AppError{
		Code:       base.Code,
		Message:    base.Message,
		HTTPStatus: base.HTTPStatus,
		Cause:      cause,
	}
}
