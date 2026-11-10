package discord

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Failure carries safe retry advice without retaining response bodies or tokens.
type Failure struct {
	Cause          *Error
	RetryAfter     time.Duration
	Global         bool
	Permanent      bool
	Uncertain      bool
	Blocked        bool
	UnknownMessage bool
}

func (e *Failure) Error() string { return e.Cause.Error() }
func (e *Failure) Unwrap() error { return e.Cause }

type rateLimits struct {
	mu      sync.Mutex
	until   map[string]time.Time
	buckets map[string]string
}

func rateKeys(token, method, path string) (string, string, string) {
	digest := sha256.Sum256([]byte(token))
	owner := hex.EncodeToString(digest[:])
	plain, _, _ := strings.Cut(path, "?")
	parts := strings.Split(strings.Trim(plain, "/"), "/")

	major := ""
	if len(parts) > 1 && (parts[0] == "channels" || parts[0] == "guilds") {
		major = parts[0] + "/" + parts[1]
	}

	return owner, owner + "/" + method + plain, major
}

func (r *rateLimits) check(owner, route string, now time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	global := r.until[owner]

	until := maxTime(r.until[route], r.until[r.buckets[route]])
	if global.After(until) {
		until = global
	}

	if until.After(now) {
		return &Failure{Cause: &Error{"Discord's rate limit was reached. Delivery will resume after the requested delay."}, RetryAfter: until.Sub(now), Global: global.After(now)}
	}

	return nil
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}

	return b
}

func delaySeconds(raw string) time.Duration {
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > float64(math.MaxInt64)/float64(time.Second) {
		return 0
	}

	return time.Duration(math.Ceil(seconds*1000)) * time.Millisecond
}

func (r *rateLimits) record(owner, route, major string, response *http.Response, delay time.Duration, global bool, now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.until == nil {
		r.until, r.buckets = map[string]time.Time{}, map[string]string{}
	}

	for key, until := range r.until {
		if !until.After(now) {
			delete(r.until, key)
		}
	}

	key := route
	if bucket := response.Header.Get("X-RateLimit-Bucket"); bucket != "" {
		key = owner + "/bucket/" + bucket + "/" + major
		r.buckets[route] = key
	}

	if response.Header.Get("X-RateLimit-Remaining") == "0" {
		delay = max(delay, delaySeconds(response.Header.Get("X-RateLimit-Reset-After")))
	}

	if delay > 0 {
		r.until[key] = maxTime(r.until[key], now.Add(delay))

		r.until[route] = maxTime(r.until[route], now.Add(delay))
		if global {
			r.until[owner] = maxTime(r.until[owner], now.Add(delay))
		}
	}
}

func (c *Client) do(ctx context.Context, token, method, path string, payload, output any) error {
	if len(token) == 0 || len(token) > 512 || strings.ContainsAny(token, " \t\r\n") {
		return &Failure{Cause: &Error{"Enter a valid Discord bot token."}, Permanent: true}
	}

	owner, route, major := rateKeys(token, method, path)
	if err := c.rates.check(owner, route, time.Now()); err != nil {
		return err
	}

	var body io.Reader

	contentType := "application/json"

	if encoded, ok := payload.(multipartBody); ok {
		body, contentType = bytes.NewReader(encoded.data), encoded.contentType
	} else if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return &Failure{Cause: &Error{"Unable to prepare the Discord request."}, Permanent: true}
		}

		body = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return &Failure{Cause: &Error{"Unable to prepare the Discord request."}, Permanent: true}
	}

	request.Header.Set("Authorization", "Bot "+token)
	request.Header.Set("User-Agent", "Campfire Discord integration")
	request.Header.Set("Content-Type", contentType)

	mutating := method == http.MethodPost

	response, err := c.http.Do(request)
	if err != nil {
		return &Failure{Cause: &Error{"Discord could not be reached. Check the panel's network connection and try again."}, Uncertain: mutating}
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusNotFound {
		c.invalidateAccess()
	}

	delay := delaySeconds(response.Header.Get("Retry-After"))
	if at, err := http.ParseTime(response.Header.Get("Retry-After")); err == nil {
		delay = max(delay, time.Until(at))
	}

	global := response.Header.Get("X-RateLimit-Global") == "true" || response.Header.Get("X-RateLimit-Scope") == "global"
	if response.StatusCode == http.StatusTooManyRequests {
		var limited struct {
			RetryAfter float64 `json:"retry_after"`
			Global     bool    `json:"global"`
		}
		if json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&limited) == nil {
			delay = max(delay, delaySeconds(strconv.FormatFloat(limited.RetryAfter, 'f', -1, 64)))
			global = global || limited.Global
		}

		delay = max(delay, time.Second)
	}

	c.rates.record(owner, route, major, response, delay, global, time.Now())

	switch response.StatusCode {
	case http.StatusUnauthorized:
		return &Failure{Cause: &Error{"Discord rejected the bot token. Replace it with a valid bot token."}, Permanent: true, Blocked: true, Global: true}
	case http.StatusForbidden, http.StatusNotFound:
		var problem struct {
			Code int `json:"code"`
		}
		if response.StatusCode == http.StatusNotFound {
			_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&problem)
		}

		return &Failure{Cause: &Error{"Discord access is unavailable. Check the bot's server membership and channel permissions."}, Permanent: true, Blocked: true, UnknownMessage: response.StatusCode == http.StatusNotFound && problem.Code == 10008}
	case http.StatusTooManyRequests:
		return &Failure{Cause: &Error{"Discord's rate limit was reached. Delivery will resume after the requested delay."}, RetryAfter: delay, Global: global}
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &Failure{Cause: &Error{"Discord could not complete the request. Check the integration and try again."}, Permanent: response.StatusCode < 500, Uncertain: mutating && response.StatusCode >= 500}
	}

	if output != nil && json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(output) != nil {
		return &Failure{Cause: &Error{"Discord returned an invalid response."}, Uncertain: mutating}
	}

	return nil
}
