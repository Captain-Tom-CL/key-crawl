package analysis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/captain-tom-cl/key-crawl/src/internal/settings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"golang.org/x/time/rate"
)

const maxModelRetries = 3

// One instance is shared by every worker in a batch. Never mutate the client
// options after construction; each request gets its own SDK request state.
type modelRequester struct {
	client        openai.Client
	limiter       *rate.Limiter
	admission     chan struct{}
	mu            sync.Mutex
	cooldownUntil time.Time
}

func newModelRequester(config *settings.Config) *modelRequester {
	requester := &modelRequester{
		client: openai.NewClient(
			option.WithBaseURL(config.BaseURL),
			option.WithAPIKey(config.ApiKey),
			option.WithMaxRetries(0), // Every attempt must pass our shared limiter.
		),
		admission: make(chan struct{}, 1),
	}
	if config.RequestsPerMinute > 0 {
		requester.limiter = rate.NewLimiter(rate.Limit(float64(config.RequestsPerMinute)/60), 1)
	}
	return requester
}

func (requester *modelRequester) complete(ctx context.Context, fileName string, params openai.ChatCompletionNewParams) (*openai.ChatCompletion, error) {
	requestCtx, cancel := context.WithTimeout(ctx, modelRequestTimeout)
	defer cancel()
	for attempt := 0; ; attempt++ {
		if err := requester.waitForAdmission(requestCtx); err != nil {
			return nil, fmt.Errorf("等待模型请求额度: %w", err)
		}
		response, err := requester.client.Chat.Completions.New(requestCtx, params)
		if err == nil {
			return response, nil
		}
		if requestCtx.Err() != nil {
			return nil, requestCtx.Err()
		}
		if !isRetryableModelError(err) {
			return nil, err
		}

		delay := modelRetryDelay(attempt)
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Response != nil {
				delay = max(delay, modelRetryAfter(apiErr.Response.Header, time.Now()))
			}
			// Even an exhausted request must slow down the other workers.
			if apiErr.StatusCode == http.StatusTooManyRequests {
				requester.extendCooldown(delay)
				analyzerLogger.Printf("模型触发 429，共享冷却至少 %s；文件：%s", delay.Round(time.Millisecond), fileName)
			}
		}
		if attempt >= maxModelRetries {
			return nil, fmt.Errorf("模型请求尝试 %d 次后仍失败: %w", attempt+1, err)
		}
		analyzerLogger.Printf("模型请求暂时失败：%s；将在 %s 后重试（%d/%d）；%v", fileName, delay.Round(time.Millisecond), attempt+1, maxModelRetries, err)
		if err := waitForModelDelay(requestCtx, delay); err != nil {
			return nil, fmt.Errorf("等待模型重试: %w", err)
		}
	}
}

func (requester *modelRequester) waitForAdmission(ctx context.Context) error {
	// Serialize admission, not HTTP calls. Otherwise workers could reserve RPM
	// tokens during a cooldown and all send at once when the cooldown expires.
	select {
	case requester.admission <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-requester.admission }()
	for {
		if err := waitForModelDelay(ctx, requester.cooldownRemaining()); err != nil {
			return err
		}
		if requester.limiter != nil {
			if err := requester.limiter.Wait(ctx); err != nil {
				return err
			}
		}
		// A different request may have received 429 while we waited for a token.
		if requester.cooldownRemaining() <= 0 {
			return ctx.Err()
		}
	}
}

func (requester *modelRequester) cooldownRemaining() time.Duration {
	requester.mu.Lock()
	defer requester.mu.Unlock()
	return time.Until(requester.cooldownUntil)
}

func (requester *modelRequester) extendCooldown(delay time.Duration) {
	requester.mu.Lock()
	defer requester.mu.Unlock()
	if until := time.Now().Add(delay); until.After(requester.cooldownUntil) {
		requester.cooldownUntil = until
	}
}

func waitForModelDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

func modelRetryDelay(attempt int) time.Duration {
	// Equal jitter: exponential windows of 1s, 2s, 4s, ... capped at 30s.
	ceiling := min(time.Second<<min(attempt, 5), 30*time.Second)
	return ceiling/2 + time.Duration(rand.Int64N(int64(ceiling/2)))
}

func modelRetryAfter(header http.Header, now time.Time) time.Duration {
	var delay time.Duration
	value := strings.TrimSpace(header.Get("Retry-After"))
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds >= 0 {
		// Avoid duration overflow without shortening any practical Retry-After.
		const maxDuration = time.Duration(1<<63 - 1)
		if seconds > int64(maxDuration/time.Second) {
			delay = maxDuration
		} else {
			delay = time.Duration(seconds) * time.Second
		}
	} else if date, err := http.ParseTime(value); err == nil {
		delay = max(0, date.Sub(now))
	}
	// Some OpenAI-compatible services use a millisecond hint instead.
	if milliseconds, err := time.ParseDuration(strings.TrimSpace(header.Get("Retry-After-Ms")) + "ms"); err == nil {
		delay = max(delay, milliseconds)
	}
	return delay
}

func isRetryableModelError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		for _, value := range []string{apiErr.Code, apiErr.Type} {
			switch strings.ToLower(value) {
			case "insufficient_quota", "insufficient_balance", "billing_hard_limit_reached":
				return false
			}
		}
		switch apiErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsTemporary {
		return true
	}
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNABORTED) ||
		errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.EPIPE)
}
