// Package cloudflare is a minimal client for the handful of Cloudflare API v4
// endpoints this app needs: tunnels, their ingress configuration, and DNS
// records.
//
// It is deliberately not a general-purpose SDK. Everything here is stdlib.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.cloudflare.com/client/v4"
	defaultTimeout = 30 * time.Second

	// Cloudflare's global API limit is 1200 requests per five minutes. A
	// desktop app will never approach that, but a 429 is still worth one
	// polite retry rather than surfacing as a failed route.
	maxRetries = 2
)

// Client talks to the Cloudflare API as a single account.
type Client struct {
	token     string
	accountID string

	baseURL string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the HTTP client, mainly for tests.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithBaseURL overrides the API root, mainly for tests.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(u, "/") }
}

// New returns a client authenticated with an API token. The account id may be
// empty when the caller intends to discover it with Accounts.
func New(token, accountID string, opts ...Option) *Client {
	c := &Client{
		token:     token,
		accountID: accountID,
		baseURL:   defaultBaseURL,
		http:      &http.Client{Timeout: defaultTimeout},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// AccountID reports the account this client acts on.
func (c *Client) AccountID() string { return c.accountID }

// SetAccountID sets the account after discovery.
func (c *Client) SetAccountID(id string) { c.accountID = id }

// envelope is the shape every v4 response shares.
type envelope struct {
	Success  bool            `json:"success"`
	Errors   []APIErrorItem  `json:"errors"`
	Messages []APIErrorItem  `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

// APIErrorItem is one entry from the errors array.
type APIErrorItem struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// APIError is returned when Cloudflare reports failure. Code carries the first
// error's code so callers can branch on well-known ones - see IsAlreadyExists.
type APIError struct {
	Status int
	Code   int
	Errors []APIErrorItem
}

func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("cloudflare: http %d", e.Status)
	}
	parts := make([]string, 0, len(e.Errors))
	for _, it := range e.Errors {
		parts = append(parts, fmt.Sprintf("%d: %s", it.Code, it.Message))
	}
	return "cloudflare: " + strings.Join(parts, "; ")
}

// Well-known Cloudflare error codes we act on rather than just report.
const (
	errCodeRecordAlreadyExists = 81053
	errCodeRecordConflict      = 81057
	errCodeDuplicateApp        = 12130
)

// IsAlreadyExists reports whether err means "the thing you asked me to create
// is already there", which for setup steps is success, not failure.
func IsAlreadyExists(err error) bool {
	var ae *APIError
	if !asAPIError(err, &ae) {
		return false
	}
	for _, it := range ae.Errors {
		switch it.Code {
		case errCodeRecordAlreadyExists, errCodeRecordConflict, errCodeDuplicateApp:
			return true
		}
	}
	return false
}

// IsUnauthorized reports whether the token was rejected.
func IsUnauthorized(err error) bool {
	var ae *APIError
	if !asAPIError(err, &ae) {
		return false
	}
	return ae.Status == http.StatusUnauthorized || ae.Status == http.StatusForbidden
}

func asAPIError(err error, target **APIError) bool {
	for err != nil {
		if ae, ok := err.(*APIError); ok {
			*target = ae
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// do performs a request and unmarshals the envelope's result into out.
// out may be nil when the caller does not care about the body.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}

		var rdr io.Reader
		if payload != nil {
			rdr = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Accept", "application/json")
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s %s: %w", method, path, err)
			continue
		}

		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("read response: %w", readErr)
			continue
		}

		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = &APIError{Status: resp.StatusCode}
			if d := retryAfter(resp); d > 0 && attempt < maxRetries {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(d):
				}
			}
			continue
		}

		var env envelope
		// Some error responses are not enveloped; fall back to status only.
		if err := json.Unmarshal(raw, &env); err != nil {
			if resp.StatusCode >= 300 {
				return &APIError{Status: resp.StatusCode}
			}
			return fmt.Errorf("decode response: %w", err)
		}

		if !env.Success || resp.StatusCode >= 300 {
			ae := &APIError{Status: resp.StatusCode, Errors: env.Errors}
			if len(env.Errors) > 0 {
				ae.Code = env.Errors[0].Code
			}
			return ae
		}

		if out == nil || len(env.Result) == 0 || string(env.Result) == "null" {
			return nil
		}
		if err := json.Unmarshal(env.Result, out); err != nil {
			return fmt.Errorf("decode result: %w", err)
		}
		return nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("%s %s: exhausted retries", method, path)
	}
	return lastErr
}

func backoff(attempt int) time.Duration {
	return time.Duration(attempt) * 500 * time.Millisecond
}

func retryAfter(resp *http.Response) time.Duration {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return time.Second
	}
	secs, err := strconv.Atoi(v)
	if err != nil || secs < 0 {
		return time.Second
	}
	if secs > 30 {
		secs = 30
	}
	return time.Duration(secs) * time.Second
}
