package http

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SoulPancake/pinGo/pkg/errors"
)

type RequestHeader struct {
	Method  string
	URI     string
	Version string
	Headers http.Header
}

func BuildRequest(method, uri string, version *string) (*RequestHeader, *errors.Error) {
	if method == "" {
		return nil, errors.New(errors.InvalidHTTPHeader, "method cannot be empty")
	}
	if uri == "" {
		return nil, errors.New(errors.InvalidHTTPHeader, "URI cannot be empty")
	}
	
	ver := "HTTP/1.1"
	if version != nil {
		ver = *version
	}
	
	return &RequestHeader{
		Method:  method,
		URI:     uri,
		Version: ver,
		Headers: make(http.Header),
	}, nil
}

func (r *RequestHeader) InsertHeader(name, value string) *errors.Error {
	if name == "" {
		return errors.New(errors.InvalidHTTPHeader, "header name cannot be empty")
	}
	r.Headers.Add(name, value)
	return nil
}

func (r *RequestHeader) SetHeader(name, value string) *errors.Error {
	if name == "" {
		return errors.New(errors.InvalidHTTPHeader, "header name cannot be empty")
	}
	r.Headers.Set(name, value)
	return nil
}

func (r *RequestHeader) GetHeader(name string) string {
	return r.Headers.Get(name)
}

func (r *RequestHeader) RemoveHeader(name string) {
	r.Headers.Del(name)
}

func (r *RequestHeader) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s %s %s\r\n", r.Method, r.URI, r.Version))
	
	keys := make([]string, 0, len(r.Headers))
	for k := range r.Headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	
	for _, k := range keys {
		for _, v := range r.Headers[k] {
			buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
		}
	}
	buf.WriteString("\r\n")
	return buf.String()
}

type ResponseHeader struct {
	Version    string
	StatusCode int
	Status     string
	Headers    http.Header
}

func (r *ResponseHeader) InsertHeader(name, value string) *errors.Error {
	if name == "" {
		return errors.New(errors.InvalidHTTPHeader, "header name cannot be empty")
	}
	if r.Headers == nil {
		r.Headers = make(http.Header)
	}
	r.Headers.Add(name, value)
	return nil
}

func (r *ResponseHeader) SetHeader(name, value string) *errors.Error {
	if name == "" {
		return errors.New(errors.InvalidHTTPHeader, "header name cannot be empty")
	}
	if r.Headers == nil {
		r.Headers = make(http.Header)
	}
	r.Headers.Set(name, value)
	return nil
}

func (r *ResponseHeader) GetHeader(name string) string {
	if r.Headers == nil {
		return ""
	}
	return r.Headers.Get(name)
}

func (r *ResponseHeader) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s %d %s\r\n", r.Version, r.StatusCode, r.Status))
	
	if r.Headers != nil {
		keys := make([]string, 0, len(r.Headers))
		for k := range r.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		
		for _, k := range keys {
			for _, v := range r.Headers[k] {
				buf.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
			}
		}
	}
	buf.WriteString("\r\n")
	return buf.String()
}

type HTTPVersion struct {
	Major int
	Minor int
}

func (v HTTPVersion) String() string {
	return fmt.Sprintf("HTTP/%d.%d", v.Major, v.Minor)
}

var (
	HTTP10 = HTTPVersion{1, 0}
	HTTP11 = HTTPVersion{1, 1}
	HTTP2  = HTTPVersion{2, 0}
)

func ParseHTTPVersion(version string) (HTTPVersion, *errors.Error) {
	if !strings.HasPrefix(version, "HTTP/") {
		return HTTPVersion{}, errors.New(errors.InvalidHTTPHeader, "invalid HTTP version format")
	}
	
	versionPart := strings.TrimPrefix(version, "HTTP/")
	parts := strings.Split(versionPart, ".")
	if len(parts) != 2 {
		return HTTPVersion{}, errors.New(errors.InvalidHTTPHeader, "invalid HTTP version format")
	}
	
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return HTTPVersion{}, errors.NewWithCause(errors.InvalidHTTPHeader, "invalid major version", err)
	}
	
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return HTTPVersion{}, errors.NewWithCause(errors.InvalidHTTPHeader, "invalid minor version", err)
	}
	
	return HTTPVersion{Major: major, Minor: minor}, nil
}

type CacheControl struct {
	MaxAge       *time.Duration
	NoCache      bool
	NoStore      bool
	Public       bool
	Private      bool
	MustRevalidate bool
}

func ParseCacheControl(value string) *CacheControl {
	cc := &CacheControl{}
	directives := strings.Split(value, ",")
	
	for _, directive := range directives {
		directive = strings.TrimSpace(directive)
		parts := strings.SplitN(directive, "=", 2)
		name := strings.ToLower(parts[0])
		
		switch name {
		case "no-cache":
			cc.NoCache = true
		case "no-store":
			cc.NoStore = true
		case "public":
			cc.Public = true
		case "private":
			cc.Private = true
		case "must-revalidate":
			cc.MustRevalidate = true
		case "max-age":
			if len(parts) == 2 {
				if seconds, err := strconv.Atoi(parts[1]); err == nil {
					duration := time.Duration(seconds) * time.Second
					cc.MaxAge = &duration
				}
			}
		}
	}
	
	return cc
}

func (cc *CacheControl) String() string {
	var parts []string
	
	if cc.NoCache {
		parts = append(parts, "no-cache")
	}
	if cc.NoStore {
		parts = append(parts, "no-store")
	}
	if cc.Public {
		parts = append(parts, "public")
	}
	if cc.Private {
		parts = append(parts, "private")
	}
	if cc.MustRevalidate {
		parts = append(parts, "must-revalidate")
	}
	if cc.MaxAge != nil {
		parts = append(parts, fmt.Sprintf("max-age=%d", int(cc.MaxAge.Seconds())))
	}
	
	return strings.Join(parts, ", ")
}