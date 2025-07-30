package errors

import (
	"net/http"
	"testing"
)

func TestError_Error(t *testing.T) {
	tests := []struct {
		name     string
		error    *Error
		expected string
	}{
		{
			name: "error without cause",
			error: &Error{
				Type:    InvalidHTTPHeader,
				Message: "test error",
			},
			expected: "InvalidHTTPHeader: test error",
		},
		{
			name: "error with cause",
			error: &Error{
				Type:    InternalError,
				Message: "test error",
				Cause:   New(InvalidHTTPHeader, "root cause"),
			},
			expected: "InternalError: test error (caused by: InvalidHTTPHeader: root cause)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.error.Error(); got != tt.expected {
				t.Errorf("Error.Error() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestError_HTTPStatusCode(t *testing.T) {
	tests := []struct {
		name     string
		error    *Error
		expected int
	}{
		{
			name: "invalid http header",
			error: &Error{
				Type:    InvalidHTTPHeader,
				Message: "test error",
			},
			expected: http.StatusBadRequest,
		},
		{
			name: "timeout error",
			error: &Error{
				Type:    TimeoutError,
				Message: "test error",
			},
			expected: http.StatusGatewayTimeout,
		},
		{
			name: "proxy error",
			error: &Error{
				Type:    ProxyError,
				Message: "test error",
			},
			expected: http.StatusBadGateway,
		},
		{
			name: "custom status code",
			error: &Error{
				Type:       InternalError,
				Message:    "test error",
				StatusCode: http.StatusTeapot,
			},
			expected: http.StatusTeapot,
		},
		{
			name: "default status code",
			error: &Error{
				Type:    ErrorType("UnknownError"),
				Message: "test error",
			},
			expected: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.error.HTTPStatusCode(); got != tt.expected {
				t.Errorf("Error.HTTPStatusCode() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		message   string
	}{
		{
			name:      "create new error",
			errorType: InvalidHTTPHeader,
			message:   "test message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(tt.errorType, tt.message)
			if err.Type != tt.errorType {
				t.Errorf("Error.Type = %v, expected %v", err.Type, tt.errorType)
			}
			if err.Message != tt.message {
				t.Errorf("Error.Message = %v, expected %v", err.Message, tt.message)
			}
			if err.Cause != nil {
				t.Errorf("Error.Cause = %v, expected nil", err.Cause)
			}
		})
	}
}

func TestNewWithCause(t *testing.T) {
	tests := []struct {
		name      string
		errorType ErrorType
		message   string
		cause     error
	}{
		{
			name:      "create error with cause",
			errorType: InternalError,
			message:   "wrapper error",
			cause:     New(InvalidHTTPHeader, "root cause"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewWithCause(tt.errorType, tt.message, tt.cause)
			if err.Type != tt.errorType {
				t.Errorf("Error.Type = %v, expected %v", err.Type, tt.errorType)
			}
			if err.Message != tt.message {
				t.Errorf("Error.Message = %v, expected %v", err.Message, tt.message)
			}
			if err.Cause != tt.cause {
				t.Errorf("Error.Cause = %v, expected %v", err.Cause, tt.cause)
			}
		})
	}
}

func TestNewWithStatus(t *testing.T) {
	tests := []struct {
		name       string
		errorType  ErrorType
		message    string
		statusCode int
	}{
		{
			name:       "create error with status",
			errorType:  ProxyError,
			message:    "custom error",
			statusCode: http.StatusTeapot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewWithStatus(tt.errorType, tt.message, tt.statusCode)
			if err.Type != tt.errorType {
				t.Errorf("Error.Type = %v, expected %v", err.Type, tt.errorType)
			}
			if err.Message != tt.message {
				t.Errorf("Error.Message = %v, expected %v", err.Message, tt.message)
			}
			if err.StatusCode != tt.statusCode {
				t.Errorf("Error.StatusCode = %v, expected %v", err.StatusCode, tt.statusCode)
			}
		})
	}
}

func TestResult(t *testing.T) {
	t.Run("Ok result", func(t *testing.T) {
		result := Ok("test value")
		if !result.IsOk() {
			t.Errorf("Result.IsOk() = false, expected true")
		}
		if result.IsErr() {
			t.Errorf("Result.IsErr() = true, expected false")
		}
		if result.Value != "test value" {
			t.Errorf("Result.Value = %v, expected 'test value'", result.Value)
		}
		if result.Error != nil {
			t.Errorf("Result.Error = %v, expected nil", result.Error)
		}
	})

	t.Run("Err result", func(t *testing.T) {
		testErr := New(InternalError, "test error")
		result := Err[string](testErr)
		if result.IsOk() {
			t.Errorf("Result.IsOk() = true, expected false")
		}
		if !result.IsErr() {
			t.Errorf("Result.IsErr() = false, expected true")
		}
		if result.Value != "" {
			t.Errorf("Result.Value = %v, expected empty string", result.Value)
		}
		if result.Error != testErr {
			t.Errorf("Result.Error = %v, expected %v", result.Error, testErr)
		}
	})

	t.Run("Unwrap", func(t *testing.T) {
		okResult := Ok(42)
		value, err := okResult.Unwrap()
		if value != 42 {
			t.Errorf("Unwrap value = %v, expected 42", value)
		}
		if err != nil {
			t.Errorf("Unwrap error = %v, expected nil", err)
		}

		testErr := New(InternalError, "test error")
		errResult := Err[int](testErr)
		value, err = errResult.Unwrap()
		if value != 0 {
			t.Errorf("Unwrap value = %v, expected 0", value)
		}
		if err != testErr {
			t.Errorf("Unwrap error = %v, expected %v", err, testErr)
		}
	})
}

func TestOrErr(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		errorType ErrorType
		message   string
		expected  *Error
	}{
		{
			name:      "nil error",
			err:       nil,
			errorType: InternalError,
			message:   "test message",
			expected:  nil,
		},
		{
			name:      "non-nil error",
			err:       New(InvalidHTTPHeader, "original error"),
			errorType: InternalError,
			message:   "wrapper message",
			expected: &Error{
				Type:    InternalError,
				Message: "wrapper message",
				Cause:   New(InvalidHTTPHeader, "original error"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OrErr(tt.err, tt.errorType, tt.message)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("OrErr() = %v, expected nil", result)
				}
				return
			}
			
			if result == nil {
				t.Errorf("OrErr() = nil, expected error")
				return
			}
			
			if result.Type != tt.expected.Type {
				t.Errorf("OrErr().Type = %v, expected %v", result.Type, tt.expected.Type)
			}
			if result.Message != tt.expected.Message {
				t.Errorf("OrErr().Message = %v, expected %v", result.Message, tt.expected.Message)
			}
			if result.Cause == nil && tt.expected.Cause != nil {
				t.Errorf("OrErr().Cause = nil, expected %v", tt.expected.Cause)
			}
		})
	}
}