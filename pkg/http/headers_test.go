package http

import (
	"testing"
	"time"
)

func TestBuildRequest(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		uri         string
		version     *string
		shouldError bool
		expectedErr string
	}{
		{
			name:        "valid request",
			method:      "GET",
			uri:         "/test",
			version:     nil,
			shouldError: false,
		},
		{
			name:        "valid request with version",
			method:      "POST",
			uri:         "/api/v1/test",
			version:     stringPtr("HTTP/2.0"),
			shouldError: false,
		},
		{
			name:        "empty method",
			method:      "",
			uri:         "/test",
			version:     nil,
			shouldError: true,
			expectedErr: "method cannot be empty",
		},
		{
			name:        "empty URI",
			method:      "GET",
			uri:         "",
			version:     nil,
			shouldError: true,
			expectedErr: "URI cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := BuildRequest(tt.method, tt.uri, tt.version)
			
			if tt.shouldError {
				if err == nil {
					t.Errorf("BuildRequest() expected error but got none")
					return
				}
				if err.Message != tt.expectedErr {
					t.Errorf("BuildRequest() error = %v, expected %v", err.Message, tt.expectedErr)
				}
				return
			}

			if err != nil {
				t.Errorf("BuildRequest() unexpected error = %v", err)
				return
			}

			if req.Method != tt.method {
				t.Errorf("BuildRequest().Method = %v, expected %v", req.Method, tt.method)
			}
			if req.URI != tt.uri {
				t.Errorf("BuildRequest().URI = %v, expected %v", req.URI, tt.uri)
			}

			expectedVersion := "HTTP/1.1"
			if tt.version != nil {
				expectedVersion = *tt.version
			}
			if req.Version != expectedVersion {
				t.Errorf("BuildRequest().Version = %v, expected %v", req.Version, expectedVersion)
			}
		})
	}
}

func TestRequestHeader_HeaderOperations(t *testing.T) {
	req, err := BuildRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	// Test InsertHeader
	err = req.InsertHeader("Content-Type", "application/json")
	if err != nil {
		t.Errorf("InsertHeader() unexpected error = %v", err)
	}

	// Test GetHeader
	if got := req.GetHeader("Content-Type"); got != "application/json" {
		t.Errorf("GetHeader() = %v, expected 'application/json'", got)
	}

	// Test InsertHeader with empty name
	err = req.InsertHeader("", "value")
	if err == nil {
		t.Errorf("InsertHeader() with empty name should return error")
	}

	// Test SetHeader
	err = req.SetHeader("Content-Type", "text/plain")
	if err != nil {
		t.Errorf("SetHeader() unexpected error = %v", err)
	}

	if got := req.GetHeader("Content-Type"); got != "text/plain" {
		t.Errorf("GetHeader() after SetHeader() = %v, expected 'text/plain'", got)
	}

	// Test RemoveHeader
	req.RemoveHeader("Content-Type")
	if got := req.GetHeader("Content-Type"); got != "" {
		t.Errorf("GetHeader() after RemoveHeader() = %v, expected empty string", got)
	}
}

func TestResponseHeader_HeaderOperations(t *testing.T) {
	resp := &ResponseHeader{
		Version:    "HTTP/1.1",
		StatusCode: 200,
		Status:     "OK",
	}

	// Test InsertHeader
	err := resp.InsertHeader("Content-Type", "application/json")
	if err != nil {
		t.Errorf("InsertHeader() unexpected error = %v", err)
	}

	// Test GetHeader
	if got := resp.GetHeader("Content-Type"); got != "application/json" {
		t.Errorf("GetHeader() = %v, expected 'application/json'", got)
	}

	// Test InsertHeader with empty name
	err = resp.InsertHeader("", "value")
	if err == nil {
		t.Errorf("InsertHeader() with empty name should return error")
	}

	// Test SetHeader
	err = resp.SetHeader("Content-Type", "text/plain")
	if err != nil {
		t.Errorf("SetHeader() unexpected error = %v", err)
	}

	if got := resp.GetHeader("Content-Type"); got != "text/plain" {
		t.Errorf("GetHeader() after SetHeader() = %v, expected 'text/plain'", got)
	}

	// Test GetHeader with nil headers
	respNil := &ResponseHeader{}
	if got := respNil.GetHeader("Content-Type"); got != "" {
		t.Errorf("GetHeader() with nil headers = %v, expected empty string", got)
	}
}

func TestRequestHeader_String(t *testing.T) {
	req, err := BuildRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req.InsertHeader("Host", "example.com")
	req.InsertHeader("User-Agent", "Test-Agent")

	str := req.String()
	
	// Check if the string contains the request line
	if !contains(str, "GET /test HTTP/1.1") {
		t.Errorf("String() should contain request line, got: %v", str)
	}

	// Check if headers are present
	if !contains(str, "Host: example.com") {
		t.Errorf("String() should contain Host header, got: %v", str)
	}

	if !contains(str, "User-Agent: Test-Agent") {
		t.Errorf("String() should contain User-Agent header, got: %v", str)
	}
}

func TestResponseHeader_String(t *testing.T) {
	resp := &ResponseHeader{
		Version:    "HTTP/1.1",
		StatusCode: 200,
		Status:     "OK",
	}

	resp.InsertHeader("Content-Type", "application/json")
	resp.InsertHeader("Content-Length", "100")

	str := resp.String()

	// Check if the string contains the status line
	if !contains(str, "HTTP/1.1 200 OK") {
		t.Errorf("String() should contain status line, got: %v", str)
	}

	// Check if headers are present
	if !contains(str, "Content-Type: application/json") {
		t.Errorf("String() should contain Content-Type header, got: %v", str)
	}

	if !contains(str, "Content-Length: 100") {
		t.Errorf("String() should contain Content-Length header, got: %v", str)
	}
}

func TestParseHTTPVersion(t *testing.T) {
	tests := []struct {
		name        string
		version     string
		expected    HTTPVersion
		shouldError bool
	}{
		{
			name:     "HTTP/1.0",
			version:  "HTTP/1.0",
			expected: HTTPVersion{1, 0},
		},
		{
			name:     "HTTP/1.1",
			version:  "HTTP/1.1",
			expected: HTTPVersion{1, 1},
		},
		{
			name:     "HTTP/2.0",
			version:  "HTTP/2.0",
			expected: HTTPVersion{2, 0},
		},
		{
			name:        "invalid format",
			version:     "INVALID",
			shouldError: true,
		},
		{
			name:        "missing version",
			version:     "HTTP/",
			shouldError: true,
		},
		{
			name:        "invalid major version",
			version:     "HTTP/X.1",
			shouldError: true,
		},
		{
			name:        "invalid minor version",
			version:     "HTTP/1.X",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := ParseHTTPVersion(tt.version)

			if tt.shouldError {
				if err == nil {
					t.Errorf("ParseHTTPVersion() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseHTTPVersion() unexpected error = %v", err)
				return
			}

			if version != tt.expected {
				t.Errorf("ParseHTTPVersion() = %v, expected %v", version, tt.expected)
			}
		})
	}
}

func TestHTTPVersion_String(t *testing.T) {
	tests := []struct {
		name     string
		version  HTTPVersion
		expected string
	}{
		{
			name:     "HTTP/1.0",
			version:  HTTP10,
			expected: "HTTP/1.0",
		},
		{
			name:     "HTTP/1.1",
			version:  HTTP11,
			expected: "HTTP/1.1",
		},
		{
			name:     "HTTP/2.0",
			version:  HTTP2,
			expected: "HTTP/2.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.version.String(); got != tt.expected {
				t.Errorf("HTTPVersion.String() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestParseCacheControl(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected *CacheControl
	}{
		{
			name:  "no-cache",
			value: "no-cache",
			expected: &CacheControl{
				NoCache: true,
			},
		},
		{
			name:  "max-age",
			value: "max-age=300",
			expected: &CacheControl{
				MaxAge: durationPtr(300 * time.Second),
			},
		},
		{
			name:  "multiple directives",
			value: "public, max-age=3600, must-revalidate",
			expected: &CacheControl{
				Public:         true,
				MaxAge:         durationPtr(3600 * time.Second),
				MustRevalidate: true,
			},
		},
		{
			name:  "no-store and private",
			value: "no-store, private",
			expected: &CacheControl{
				NoStore: true,
				Private: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := ParseCacheControl(tt.value)

			if cc.NoCache != tt.expected.NoCache {
				t.Errorf("CacheControl.NoCache = %v, expected %v", cc.NoCache, tt.expected.NoCache)
			}
			if cc.NoStore != tt.expected.NoStore {
				t.Errorf("CacheControl.NoStore = %v, expected %v", cc.NoStore, tt.expected.NoStore)
			}
			if cc.Public != tt.expected.Public {
				t.Errorf("CacheControl.Public = %v, expected %v", cc.Public, tt.expected.Public)
			}
			if cc.Private != tt.expected.Private {
				t.Errorf("CacheControl.Private = %v, expected %v", cc.Private, tt.expected.Private)
			}
			if cc.MustRevalidate != tt.expected.MustRevalidate {
				t.Errorf("CacheControl.MustRevalidate = %v, expected %v", cc.MustRevalidate, tt.expected.MustRevalidate)
			}

			if tt.expected.MaxAge != nil {
				if cc.MaxAge == nil {
					t.Errorf("CacheControl.MaxAge = nil, expected %v", *tt.expected.MaxAge)
				} else if *cc.MaxAge != *tt.expected.MaxAge {
					t.Errorf("CacheControl.MaxAge = %v, expected %v", *cc.MaxAge, *tt.expected.MaxAge)
				}
			} else if cc.MaxAge != nil {
				t.Errorf("CacheControl.MaxAge = %v, expected nil", *cc.MaxAge)
			}
		})
	}
}

func TestCacheControl_String(t *testing.T) {
	tests := []struct {
		name     string
		cc       *CacheControl
		expected string
	}{
		{
			name: "no-cache",
			cc: &CacheControl{
				NoCache: true,
			},
			expected: "no-cache",
		},
		{
			name: "max-age",
			cc: &CacheControl{
				MaxAge: durationPtr(300 * time.Second),
			},
			expected: "max-age=300",
		},
		{
			name: "multiple directives",
			cc: &CacheControl{
				Public:         true,
				MaxAge:         durationPtr(3600 * time.Second),
				MustRevalidate: true,
			},
			expected: "public, must-revalidate, max-age=3600",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cc.String()
			if got != tt.expected {
				t.Errorf("CacheControl.String() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}