package retry

import (
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
)

type fakeNetError struct{}

func (fakeNetError) Error() string   { return "fake net error" }
func (fakeNetError) Timeout() bool   { return true }
func (fakeNetError) Temporary() bool { return true }

func TestIsRetriableNet(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"net.Error interface", fakeNetError{}, true},
		{"connection refused", syscall.ECONNREFUSED, true},
		{"wrapped connection refused", &net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}, true},
		{"io.EOF", io.EOF, true},
		// context.DeadlineExceeded реализует net.Error (Timeout()/Temporary()) -
		// таймаут клиента тоже стоит повторить, это не баг классификатора.
		{"context deadline exceeded - implements net.Error, retriable", context.DeadlineExceeded, true},
		{"generic error - not retriable", errors.New("boom"), false},
		{"http 500 - retriable", &HTTPStatusError{StatusCode: 500}, true},
		{"http 503 - retriable", &HTTPStatusError{StatusCode: 503}, true},
		{"http 599 - retriable (upper bound)", &HTTPStatusError{StatusCode: 599}, true},
		{"http 400 - not retriable", &HTTPStatusError{StatusCode: 400}, false},
		{"http 404 - not retriable", &HTTPStatusError{StatusCode: 404}, false},
		{"http 499 - not retriable (below 5xx)", &HTTPStatusError{StatusCode: 499}, false},
		{"http 600 - not retriable (above 5xx)", &HTTPStatusError{StatusCode: 600}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetriableNet(tt.err); got != tt.want {
				t.Errorf("IsRetriableNet(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// net.OpError с DNS-таймаутом реализует net.Error - должен считаться retriable.
func TestIsRetriableNet_DialTimeout(t *testing.T) {
	err := &net.OpError{
		Op:  "dial",
		Err: &net.DNSError{IsTimeout: true},
	}
	if !IsRetriableNet(err) {
		t.Error("IsRetriableNet() = false, want true for dial timeout")
	}
}
