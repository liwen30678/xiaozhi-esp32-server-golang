package client

import (
	"context"
	"errors"
	"testing"
	asr_types "xiaozhi-esp32-server-golang/internal/domain/asr/types"
)

func TestRetireAsrResult_DoubaoRetryableError(t *testing.T) {
	a := &Asr{
		AsrType:          "doubao",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{Error: errors.New("asr response code: 45000081")}

	result, isRetry, err := a.RetireAsrResult(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.Text != "" {
		t.Fatalf("expected empty text, got %q", result.Text)
	}
	if !isRetry {
		t.Fatalf("expected isRetry to be true")
	}
}

func TestRetireAsrResult_DoubaoWaitingNextPacketTimeoutRetryable(t *testing.T) {
	a := &Asr{
		AsrType:          "doubao",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{
		Error: errors.New("[Timeout waiting next packet] Handle response: pre-handle: schedule request: allocate session: waiting next packet timeout: 5.000000 seconds, session has ended"),
	}

	result, isRetry, err := a.RetireAsrResult(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !isRetry {
		t.Fatalf("expected isRetry to be true")
	}
	if result.RetryReason != asr_types.RetryReasonDoubaoWaitingNextPacketTimeout {
		t.Fatalf("expected retry reason %q, got %q", asr_types.RetryReasonDoubaoWaitingNextPacketTimeout, result.RetryReason)
	}
	if result.Error == nil {
		t.Fatalf("expected original error to be preserved")
	}
}

func TestRetireAsrResult_DoubaoNonRetryableError(t *testing.T) {
	a := &Asr{
		AsrType:          "doubao",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{Error: errors.New("asr response code: 123")}

	_, isRetry, err := a.RetireAsrResult(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if isRetry {
		t.Fatalf("expected isRetry to be false")
	}
}

func TestRetireAsrResult_XunfeiRetryableError(t *testing.T) {
	a := &Asr{
		AsrType:          "xunfei",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{Error: errors.New("xunfei asr error code=10008 message=service instance invalid sid=iat123")}

	result, isRetry, err := a.RetireAsrResult(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !isRetry {
		t.Fatalf("expected isRetry to be true")
	}
	if result.RetryReason != asr_types.RetryReasonXunfeiServiceInstanceInvalid {
		t.Fatalf("expected retry reason %q, got %q", asr_types.RetryReasonXunfeiServiceInstanceInvalid, result.RetryReason)
	}
	if result.Error == nil {
		t.Fatalf("expected original error to be preserved")
	}
}

func TestRetireAsrResult_XunfeiNonRetryableError(t *testing.T) {
	a := &Asr{
		AsrType:          "xunfei",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{Error: errors.New("xunfei asr error code=10163 message=invalid parameter sid=iat123")}

	_, isRetry, err := a.RetireAsrResult(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if isRetry {
		t.Fatalf("expected isRetry to be false")
	}
}
