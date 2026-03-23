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

func TestRetireAsrResult_AliyunQwen3RetryableError(t *testing.T) {
	a := &Asr{
		AsrType:          "aliyun_qwen3",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{
		Error: errors.New("read message failed: read tcp 198.18.0.1:1822->198.18.0.97:443: wsarecv: An existing connection was forcibly closed by the remote host."),
	}

	result, isRetry, err := a.RetireAsrResult(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !isRetry {
		t.Fatalf("expected isRetry to be true")
	}
	if result.RetryReason != asr_types.RetryReasonAliyunQwen3ConnectionClosed {
		t.Fatalf("expected retry reason %q, got %q", asr_types.RetryReasonAliyunQwen3ConnectionClosed, result.RetryReason)
	}
	if result.Error == nil {
		t.Fatalf("expected original error to be preserved")
	}
}

func TestRetireAsrResult_AliyunQwen3NonRetryableError(t *testing.T) {
	a := &Asr{
		AsrType:          "aliyun_qwen3",
		AsrResultChannel: make(chan asr_types.StreamingResult, 1),
	}
	a.AsrResultChannel <- asr_types.StreamingResult{
		Error: errors.New("aliyun qwen3 error: invalid parameter"),
	}

	_, isRetry, err := a.RetireAsrResult(context.Background())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if isRetry {
		t.Fatalf("expected isRetry to be false")
	}
}
