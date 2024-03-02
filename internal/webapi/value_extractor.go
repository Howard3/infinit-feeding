package webapi

import (
	"fmt"
	"net/http"
	"strconv"

	"errors"

	"github.com/go-chi/chi/v5"
)

// ParamParser is a helper for parsing and validating request parameters
type ParamParser[T any] struct {
	Result  T
	Errors  []error
	Request *http.Request
}

// NewParamParser creates a new ParamParser
func NewParamParser[T any](req *http.Request, result T) *ParamParser[T] {
	return &ParamParser[T]{Result: result, Request: req}
}

// AddError adds an error to the parser
func (p *ParamParser[T]) AddError(err error) {
	p.Errors = append(p.Errors, err)
}

// Error returns an error if there are any errors in the parser
func (p *ParamParser[T]) Error() error {
	if len(p.Errors) == 0 {
		return nil
	}

	var errMsgs error
	for _, err := range p.Errors {
		errMsgs = errors.Join(errMsgs, err)
	}

	return errMsgs
}

// WithValueAsUint64 parses a uint64 value from a request and updates the result
func (p *ParamParser[T]) WithValueAsUint64(key string, updateFunc func(*T, uint64), extractor valueExtractor) *ParamParser[T] {
	value, err := extractor(p.Request, key)
	if err != nil {
		p.AddError(err)
		return p
	}

	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		p.AddError(fmt.Errorf("invalid uint value for %s: %v", key, err))
		return p
	}

	updateFunc(&p.Result, parsed)
	return p
}

// WithValue parses a string value from a request and updates the result
func (p *ParamParser[T]) WithValue(key string, updateFunc func(*T, string), extractor valueExtractor) *ParamParser[T] {
	value, err := extractor(p.Request, key)
	if err != nil {
		p.AddError(err)
		return p
	}

	updateFunc(&p.Result, value)
	return p
}

type valueExtractor func(*http.Request, string) (string, error)

// queryExtractor extracts a query parameter from a request
func queryExtractor(req *http.Request, key string) (string, error) {
	if req == nil {
		panic("request is nil")
	} else if req.URL == nil {
		panic("request URL is nil")
	}

	value := req.URL.Query().Get(key)
	if value == "" {
		return "", fmt.Errorf("missing query parameter: %s", key)
	}
	return value, nil
}

// formExtractor extracts a form parameter from a request
func formExtractor(req *http.Request, key string) (string, error) {
	if err := req.ParseForm(); err != nil {
		return "", fmt.Errorf("error parsing form: %v", err)
	}

	return req.FormValue(key), nil
}

// ChiURLParamExtractor extracts a URL parameter from a chi router
func chiURLParamExtractor(req *http.Request, key string) (string, error) {
	if req == nil {
		panic("request is nil")
	}

	value := chi.URLParam(req, key)
	if value == "" {
		return "", fmt.Errorf("missing URL parameter: %s", key)
	}
	return value, nil
}
