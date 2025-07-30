package errors

import (
	"fmt"
	"net/http"
)

type ErrorType string

const (
	InvalidHTTPHeader    ErrorType = "InvalidHTTPHeader"
	InternalError        ErrorType = "InternalError"
	InvalidHTML          ErrorType = "InvalidHTML"
	ConnectionError      ErrorType = "ConnectionError"
	TLSError            ErrorType = "TLSError"
	TimeoutError        ErrorType = "TimeoutError"
	ProxyError          ErrorType = "ProxyError"
	LoadBalancingError  ErrorType = "LoadBalancingError"
	CacheError          ErrorType = "CacheError"
	PoolError           ErrorType = "PoolError"
)

type Error struct {
	Type        ErrorType
	Message     string
	Cause       error
	StatusCode  int
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s (caused by: %v)", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *Error) Unwrap() error {
	return e.Cause
}

func (e *Error) HTTPStatusCode() int {
	if e.StatusCode != 0 {
		return e.StatusCode
	}
	switch e.Type {
	case InvalidHTTPHeader:
		return http.StatusBadRequest
	case TimeoutError:
		return http.StatusGatewayTimeout
	case ProxyError:
		return http.StatusBadGateway
	case TLSError:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func New(errorType ErrorType, message string) *Error {
	return &Error{
		Type:    errorType,
		Message: message,
	}
}

func NewWithCause(errorType ErrorType, message string, cause error) *Error {
	return &Error{
		Type:    errorType,
		Message: message,
		Cause:   cause,
	}
}

func NewWithStatus(errorType ErrorType, message string, statusCode int) *Error {
	return &Error{
		Type:       errorType,
		Message:    message,
		StatusCode: statusCode,
	}
}

func Explain(errorType ErrorType, explanation string) *Error {
	return New(errorType, explanation)
}

func OrErr(err error, errorType ErrorType, message string) *Error {
	if err != nil {
		return NewWithCause(errorType, message, err)
	}
	return nil
}

type Result[T any] struct {
	Value T
	Error *Error
}

func Ok[T any](value T) Result[T] {
	return Result[T]{Value: value}
}

func Err[T any](err *Error) Result[T] {
	return Result[T]{Error: err}
}

func (r Result[T]) IsOk() bool {
	return r.Error == nil
}

func (r Result[T]) IsErr() bool {
	return r.Error != nil
}

func (r Result[T]) Unwrap() (T, error) {
	if r.Error == nil {
		return r.Value, nil
	}
	return r.Value, r.Error
}