package edgecases

import (
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"testing"

	onceo "github.com/HalxDocs/onceo-core"
	"github.com/HalxDocs/onceo-core/providers/paystack"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestEmptyBody(t *testing.T) {
	provider, _ := paystack.New("sk_test")
	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), http.Header{}, []byte{})
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestMaxBodySizeBoundary(t *testing.T) {
	provider, _ := paystack.New("sk_test")
	body := make([]byte, onceo.MaxBodySize)
	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), http.Header{}, body)
	if err == nil {
		t.Fatal("expected error for body at MaxBodySize (no valid signature)")
	}
}

func TestDuplicateHeaders(t *testing.T) {
	provider, _ := paystack.New("sk_test")
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"r","amount":1000,"currency":"NGN"}}`)

	headers := http.Header{}
	headers.Add("X-Paystack-Signature", sign("sk_test", body))
	headers.Add("X-Paystack-Signature", sign("sk_test", body))

	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, body)
	if !errors.Is(err, onceo.ErrDuplicateHeader) {
		t.Fatalf("expected ErrDuplicateHeader, got %v", err)
	}
}

func TestProviderNameWithColon(t *testing.T) {
	p := &namedProvider{name: "pay:stack"}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for provider name with colon")
	}
}

func TestProviderNameWithBackslash(t *testing.T) {
	p := &namedProvider{name: "pay\\stack"}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for provider name with backslash")
	}
}

func TestNegativeAmount(t *testing.T) {
	p := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: "evt_001",
			Type:            "charge.success",
			AmountMinor:     -1,
		},
	}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestInvalidCurrencyLength(t *testing.T) {
	p := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: "evt_001",
			Type:            "charge.success",
			AmountMinor:     1000,
			Currency:        "NGNx",
		},
	}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for invalid currency")
	}
}

func TestEventTypeEmpty(t *testing.T) {
	p := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: "evt_001",
			Type:            "",
		},
	}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for empty event type")
	}
}

func TestProviderEventIDWithReservedChars(t *testing.T) {
	p := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: "evt:001",
			Type:            "charge.success",
			Status:          onceo.StatusSuccess,
		},
	}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for ProviderEventID with colon")
	}

	p2 := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: "evt\\001",
			Type:            "charge.success",
			Status:          onceo.StatusSuccess,
		},
	}
	_, err = onceo.Process(context.Background(), p2, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for ProviderEventID with backslash")
	}
}

func TestContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	provider, _ := paystack.New("sk_test")
	_, err := onceo.Process(ctx, provider, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error from timed-out context")
	}
}

func TestProviderEventIDMaxLengthBoundary(t *testing.T) {
	p := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: strings.Repeat("a", 256),
			Type:            "charge.success",
			Status:          onceo.StatusSuccess,
		},
	}
	_, err := onceo.Process(context.Background(), p, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err != nil {
		t.Fatalf("256-byte ProviderEventID should be accepted, got: %v", err)
	}

	p2 := &eventProvider{
		evt: onceo.Event{
			Provider:        "test",
			ProviderEventID: strings.Repeat("a", 257),
			Type:            "charge.success",
			Status:          onceo.StatusSuccess,
		},
	}
	_, err = onceo.Process(context.Background(), p2, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for 257-byte ProviderEventID")
	}
}

type namedProvider struct {
	name string
}

func (p *namedProvider) Name() string { return p.name }
func (p *namedProvider) BodyBound() bool { return true }
func (p *namedProvider) VerifySignature(_ http.Header, _ []byte) error { return nil }
func (p *namedProvider) Parse(_ []byte) (struct{}, error) { return struct{}{}, nil }
func (p *namedProvider) Normalize(_ struct{}) (onceo.Event, error) {
	return onceo.Event{}, nil
}

type eventProvider struct {
	evt onceo.Event
}

func (p *eventProvider) Name() string { return "test" }
func (p *eventProvider) BodyBound() bool { return true }
func (p *eventProvider) VerifySignature(_ http.Header, _ []byte) error { return nil }
func (p *eventProvider) Parse(_ []byte) (struct{}, error) { return struct{}{}, nil }
func (p *eventProvider) Normalize(_ struct{}) (onceo.Event, error) { return p.evt, nil }
