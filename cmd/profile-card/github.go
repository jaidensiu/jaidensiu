package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// GitHubError is a failed GitHub GraphQL call. Status is 0 when no HTTP response was received.
type GitHubError struct {
	Msg      string
	Status   int
	Attempts int
	Err      error
}

func (e *GitHubError) Error() string { return e.Msg }
func (e *GitHubError) Unwrap() error { return e.Err }

type httpResult struct {
	status        int
	body          string
	retryAfter    time.Duration
	hasRetryAfter bool
}

// transport POSTs body to the GraphQL endpoint. It returns an error on network failure or timeout.
type transport func(body []byte) (httpResult, error)

func httpTransport(token string) transport {
	client := &http.Client{Timeout: 30 * time.Second}
	return func(body []byte) (httpResult, error) {
		req, err := http.NewRequest(http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(body))
		if err != nil {
			return httpResult{}, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "profile-card")
		resp, err := client.Do(req)
		if err != nil {
			return httpResult{}, err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return httpResult{}, err
		}
		r := httpResult{status: resp.StatusCode, body: string(b)}
		if s, err := strconv.ParseInt(resp.Header.Get("Retry-After"), 10, 64); err == nil {
			r.retryAfter, r.hasRetryAfter = time.Duration(s)*time.Second, true
		} else if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			// primary rate limit exhausted: GitHub reports it as 403/429 with remaining=0 and a reset epoch
			if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
				r.retryAfter, r.hasRetryAfter = max(time.Until(time.Unix(reset, 0)), time.Second), true
			}
		}
		return r, nil
	}
}

// gitHubClient is a read-only GraphQL client. Every query is idempotent, so transient failures
// (network errors, timeouts, 5xx, rate limiting) are retried with capped exponential backoff
// and full jitter. Anything else fails immediately.
type gitHubClient struct {
	post        transport
	maxAttempts int
	baseDelay   time.Duration
	maxDelay    time.Duration
	sleep       func(time.Duration)
	random      func() float64
	retries     int
}

func newGitHubClient(post transport) *gitHubClient {
	return &gitHubClient{
		post:        post,
		maxAttempts: 4,
		baseDelay:   time.Second,
		maxDelay:    time.Minute,
		sleep:       time.Sleep,
		random:      rand.Float64,
	}
}

// graphql runs query and decodes its data into out.
func (c *gitHubClient) graphql(query string, variables map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return err
	}
	res, err := c.send(body)
	if err != nil {
		return err
	}
	var resp struct {
		Data   json.RawMessage
		Errors json.RawMessage
	}
	if err := json.Unmarshal([]byte(res.body), &resp); err != nil {
		return &GitHubError{Msg: "GitHub GraphQL returned a non-JSON body", Status: res.status, Attempts: 1, Err: err}
	}
	if !isNull(resp.Errors) {
		return &GitHubError{Msg: "GitHub GraphQL errors: " + string(resp.Errors), Status: res.status, Attempts: 1}
	}
	if isNull(resp.Data) {
		return &GitHubError{Msg: "GitHub GraphQL response has no data", Status: res.status, Attempts: 1}
	}
	if err := json.Unmarshal(resp.Data, out); err != nil {
		return fmt.Errorf("decode GitHub GraphQL data: %w", err)
	}
	return nil
}

func isNull(m json.RawMessage) bool { return len(m) == 0 || string(m) == "null" }

func (c *gitHubClient) send(body []byte) (httpResult, error) {
	for attempt := 1; ; attempt++ {
		res, err := c.post(body)
		var failure string
		switch {
		case err != nil:
			failure = err.Error()
		case res.status >= 200 && res.status <= 299:
			return res, nil
		default:
			failure = fmt.Sprintf("HTTP %d: %s", res.status, strings.Join(strings.Fields(res.body[:min(len(res.body), 200)]), " "))
			retryable := res.status >= 500 || res.status == 429 || (res.status == 403 && res.hasRetryAfter)
			if !retryable {
				return res, &GitHubError{Msg: "GitHub GraphQL failed, not retrying: " + failure, Status: res.status, Attempts: attempt}
			}
		}

		if attempt >= c.maxAttempts {
			return res, &GitHubError{Msg: fmt.Sprintf("GitHub GraphQL failed after %d attempts: %s", attempt, failure), Status: res.status, Attempts: attempt, Err: err}
		}
		delay := c.backoff(attempt)
		if res.hasRetryAfter {
			if res.retryAfter > c.maxDelay {
				return res, &GitHubError{Msg: fmt.Sprintf("GitHub asked to wait %v (> %v), giving up: %s", res.retryAfter, c.maxDelay, failure), Status: res.status, Attempts: attempt}
			}
			delay = res.retryAfter
		}
		c.retries++
		fmt.Fprintf(os.Stderr, "warn: github graphql retry attempt=%d/%d delay=%v status=%d cause=%s\n", attempt, c.maxAttempts, delay, res.status, failure)
		c.sleep(delay)
	}
}

func (c *gitHubClient) backoff(attempt int) time.Duration {
	return time.Duration(c.random() * float64(min(c.maxDelay, c.baseDelay<<(attempt-1))))
}
