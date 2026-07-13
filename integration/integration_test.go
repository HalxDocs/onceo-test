package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	onceo "github.com/HalxDocs/onceo-core"
	"github.com/HalxDocs/onceo-core/providers/flutterwave"
	"github.com/HalxDocs/onceo-core/providers/mpesa"
	"github.com/HalxDocs/onceo-core/providers/opay"
	"github.com/HalxDocs/onceo-core/providers/paystack"
	redisstore "github.com/HalxDocs/onceo-core/store/redis"
)

// Fixture helpers — tests use inline payloads so they don't depend on external files.

func paystackChargeSuccessBody() []byte {
	return []byte(`{"event":"charge.success","data":{"id":12345,"status":"success","reference":"ref_001","amount":50000,"currency":"NGN","paid_at":"2024-01-01T00:00:00Z","created_at":"2024-01-01T00:00:00Z"}}`)
}

func paystackHeaders(secret string, body []byte) http.Header {
	h := http.Header{}
	h.Set("X-Paystack-Signature", testSignHMACSHA512([]byte(secret), body))
	return h
}

func flutterwaveChargeBody() []byte {
	return []byte(`{"event":"charge.completed","data":{"id":98765,"tx_ref":"tx_ref_001","flw_ref":"flw_ref_001","status":"successful","amount":15000,"charged_amount":15000,"currency":"NGN","created_at":"2024-01-01T00:00:00Z"}}`)
}

func flutterwaveHeaders(secret string, _ []byte) http.Header {
	h := http.Header{}
	// Verif-Hash = SHA256(secret) — static, body-independent
	h.Set("Verif-Hash", testSHA256Hex([]byte(secret)))
	return h
}

func opayPaymentBody() []byte {
	return []byte(`{"eventType":"PAYMENT_SUCCESSFUL","data":{"reference":"opay_ref_001","amount":"450.00","currency":"NGN","orderNo":"ORDER_12345","completeTime":"2024-01-01T00:00:00Z"}}`)
}

func opayHeaders(secret string, body []byte) http.Header {
	h := http.Header{}
	h.Set("Authorization", testSignHMACSHA512([]byte(secret), body))
	return h
}

func mpesaSTKBody() []byte {
	return []byte(`{"Body":{"stkCallback":{"MerchantRequestID":"mreq_001","CheckoutRequestID":"ws_CO_07072026123000000","ResultCode":0,"ResultDesc":"Success","CallbackMetadata":{"Item":[{"Name":"Amount","Value":1500.0},{"Name":"MpesaReceiptNumber","Value":"MPR123ABC"},{"Name":"PhoneNumber","Value":"254700000000"}]}}}}`)
}

func mpesaHeaders(token string, _ []byte) http.Header {
	h := http.Header{}
	h.Set("X-Onceo-Mpesa-Callback-Token", token)
	return h
}

func TestPaystackFullFlow(t *testing.T) {
	secret := "sk_test_live_abc123"
	body := paystackChargeSuccessBody()
	headers := paystackHeaders(secret, body)

	provider, err := paystack.New(secret)
	if err != nil {
		t.Fatalf("paystack.New: %v", err)
	}
	store := onceo.NewMemoryStore()

	event, err := onceo.Process(context.Background(), provider, store, headers, body)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if event.Provider != "paystack" {
		t.Errorf("Provider = %q, want paystack", event.Provider)
	}
	if event.ProviderEventID != "12345" {
		t.Errorf("ProviderEventID = %q, want 12345", event.ProviderEventID)
	}
	if event.Type != "charge.success" {
		t.Errorf("Type = %q", event.Type)
	}
	if event.Status != onceo.StatusSuccess {
		t.Errorf("Status = %v", event.Status)
	}
	if event.AmountMinor != 50000 {
		t.Errorf("AmountMinor = %d, want 50000", event.AmountMinor)
	}
	if event.Currency != "NGN" {
		t.Errorf("Currency = %q", event.Currency)
	}
	if event.ID == "" {
		t.Error("ID not set")
	}
	if event.ProcessedAt.IsZero() {
		t.Error("ProcessedAt not set")
	}
	if len(event.RawPayload) == 0 {
		t.Error("RawPayload not set")
	}
}

func TestPaystackDuplicate(t *testing.T) {
	secret := "sk_test_dup"
	body := paystackChargeSuccessBody()
	headers := paystackHeaders(secret, body)

	provider, err := paystack.New(secret)
	if err != nil {
		t.Fatalf("paystack.New: %v", err)
	}
	store := onceo.NewMemoryStore()

	_, err = onceo.Process(context.Background(), provider, store, headers, body)
	if err != nil {
		t.Fatalf("first Process: %v", err)
	}

	_, err = onceo.Process(context.Background(), provider, store, headers, body)
	if !errors.Is(err, onceo.ErrDuplicateEvent) {
		t.Fatalf("expected ErrDuplicateEvent, got %v", err)
	}
}

func TestPaystackBadSignature(t *testing.T) {
	secret := "sk_test_real"
	body := paystackChargeSuccessBody()
	headers := paystackHeaders("sk_test_wrong", body)

	provider, err := paystack.New(secret)
	if err != nil {
		t.Fatalf("paystack.New: %v", err)
	}

	_, err = onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, body)
	if !errors.Is(err, onceo.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestPaystackTamperedBody(t *testing.T) {
	secret := "sk_test_abc"
	body := paystackChargeSuccessBody()
	headers := paystackHeaders(secret, body)

	tampered := []byte(`{"event":"charge.success","data":{"id":12345,"status":"success","reference":"ref_001","amount":99999999,"currency":"NGN"}}`)

	provider, err := paystack.New(secret)
	if err != nil {
		t.Fatalf("paystack.New: %v", err)
	}

	_, err = onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, tampered)
	if !errors.Is(err, onceo.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for tampered body, got %v", err)
	}
}

func TestFlutterwaveFullFlow(t *testing.T) {
	secret := "my_flw_secret_hash"
	body := flutterwaveChargeBody()
	headers := flutterwaveHeaders(secret, body)

	provider, err := flutterwave.New(secret)
	if err != nil {
		t.Fatalf("flutterwave.New: %v", err)
	}
	store := onceo.NewMemoryStore()

	event, err := onceo.Process(context.Background(), provider, store, headers, body)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if event.Provider != "flutterwave" {
		t.Errorf("Provider = %q", event.Provider)
	}
	if event.Type != "charge.completed" {
		t.Errorf("Type = %q", event.Type)
	}
	if event.Status != onceo.StatusSuccess {
		t.Errorf("Status = %v", event.Status)
	}
	if event.AmountMinor != 15000 {
		t.Errorf("AmountMinor = %d", event.AmountMinor)
	}
}

func TestFlutterwaveNoBodyIntegrity(t *testing.T) {
	secret := "flw_secret"
	body := flutterwaveChargeBody()
	headers := flutterwaveHeaders(secret, body)

	provider, err := flutterwave.New(secret)
	if err != nil {
		t.Fatalf("flutterwave.New: %v", err)
	}

	if provider.BodyBound() {
		t.Fatal("Flutterwave should NOT be body-bound")
	}

	// Tamper the body — signature still passes because Verif-Hash is static
	tampered := []byte(`{"event":"charge.completed","data":{"id":99999,"tx_ref":"hijacked","status":"successful","amount":99999999,"currency":"NGN"}}`)
	_, err = onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, tampered)
	if err != nil {
		t.Fatalf("Flutterwave should accept tampered body (no body binding), got: %v", err)
	}
}

func TestOPayFullFlow(t *testing.T) {
	secret := "opay_secret_key_001"
	body := opayPaymentBody()
	headers := opayHeaders(secret, body)

	provider, err := opay.New(secret)
	if err != nil {
		t.Fatalf("opay.New: %v", err)
	}
	store := onceo.NewMemoryStore()

	event, err := onceo.Process(context.Background(), provider, store, headers, body)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if event.Provider != "opay" {
		t.Errorf("Provider = %q", event.Provider)
	}
	if event.ProviderEventID != "ORDER_12345" {
		t.Errorf("ProviderEventID = %q", event.ProviderEventID)
	}
	if event.Type != "PAYMENT_SUCCESSFUL" {
		t.Errorf("Type = %q", event.Type)
	}
	if event.Status != onceo.StatusSuccess {
		t.Errorf("Status = %v", event.Status)
	}
	if event.AmountMinor != 45000 {
		t.Errorf("AmountMinor = %d, want 45000", event.AmountMinor)
	}
	if event.Currency != "NGN" {
		t.Errorf("Currency = %q", event.Currency)
	}
}

func TestOPayOpaySignatureFallback(t *testing.T) {
	secret := "opay_secret"
	body := opayPaymentBody()

	provider, err := opay.New(secret)
	if err != nil {
		t.Fatalf("opay.New: %v", err)
	}

	headers := http.Header{}
	headers.Set("Opay-Signature", testSignHMACSHA512([]byte(secret), body))

	_, err = onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, body)
	if err != nil {
		t.Fatalf("Opay-Signature fallback failed: %v", err)
	}
}

func TestOPayDuplicateAuthorizationFails(t *testing.T) {
	secret := "opay_secret"
	body := opayPaymentBody()

	provider, err := opay.New(secret)
	if err != nil {
		t.Fatalf("opay.New: %v", err)
	}

	headers := http.Header{}
	headers.Add("Authorization", testSignHMACSHA512([]byte(secret), body))
	headers.Add("Authorization", testSignHMACSHA512([]byte(secret), body))

	_, err = onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, body)
	if !errors.Is(err, onceo.ErrInvalidSignature) && !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate header error, got %v", err)
	}
}

func TestMpesaSTKFullFlow(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890abcdef12"
	body := mpesaSTKBody()
	headers := mpesaHeaders(token, body)

	provider, err := mpesa.New(token)
	if err != nil {
		t.Fatalf("mpesa.New: %v", err)
	}
	store := onceo.NewMemoryStore()

	event, err := onceo.Process(context.Background(), provider, store, headers, body)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if event.Provider != "mpesa" {
		t.Errorf("Provider = %q", event.Provider)
	}
	if event.Type != "stk.push" {
		t.Errorf("Type = %q", event.Type)
	}
	if event.Status != onceo.StatusSuccess {
		t.Errorf("Status = %v", event.Status)
	}
	if event.AmountMinor != 150000 {
		t.Errorf("AmountMinor = %d, want 150000", event.AmountMinor)
	}
	if event.Currency != "KES" {
		t.Errorf("Currency = %q", event.Currency)
	}
}

func TestMpesaWrongToken(t *testing.T) {
	token := "abcdef1234567890abcdef1234567890abcdef12"
	body := mpesaSTKBody()
	headers := mpesaHeaders("wrong_token_value_thats_long_enough", body)

	provider, err := mpesa.New(token)
	if err != nil {
		t.Fatalf("mpesa.New: %v", err)
	}

	_, err = onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), headers, body)
	if !errors.Is(err, onceo.ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature for wrong token, got %v", err)
	}
}

func TestProcessCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	provider, _ := paystack.New("sk_test")
	_, err := onceo.Process(ctx, provider, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestProcessBodyTooLarge(t *testing.T) {
	provider, _ := paystack.New("sk_test")
	big := make([]byte, onceo.MaxBodySize+1)
	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), http.Header{}, big)
	if err == nil {
		t.Fatal("expected error for body exceeding MaxBodySize")
	}
}

func TestBodyBoundEnforcement(t *testing.T) {
	tests := []struct {
		name string
		bind func() bool
		want bool
	}{
		{"paystack", func() bool { p, _ := paystack.New("sk"); return p.BodyBound() }, true},
		{"flutterwave", func() bool { p, _ := flutterwave.New("sk"); return p.BodyBound() }, false},
		{"opay", func() bool { p, _ := opay.New("sk"); return p.BodyBound() }, true},
		{"mpesa", func() bool { p, _ := mpesa.New("a1234567890123456789012345678901"); return p.BodyBound() }, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.bind()
			if got != tc.want {
				t.Errorf("BodyBound() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMemoryStoreEviction(t *testing.T) {
	store := onceo.NewMemoryStoreWithMax(3)
	ctx := context.Background()

	e1 := onceo.Event{Provider: "p", ProviderEventID: "1", Type: "t", Status: onceo.StatusSuccess}
	e2 := onceo.Event{Provider: "p", ProviderEventID: "2", Type: "t", Status: onceo.StatusSuccess}
	e3 := onceo.Event{Provider: "p", ProviderEventID: "3", Type: "t", Status: onceo.StatusSuccess}
	e4 := onceo.Event{Provider: "p", ProviderEventID: "4", Type: "t", Status: onceo.StatusSuccess}

	mustCreate(t, store, ctx, e1)
	mustCreate(t, store, ctx, e2)
	mustCreate(t, store, ctx, e3)

	// e4 should evict e1
	mustCreate(t, store, ctx, e4)

	// e1 is evicted, so it can be re-created
	created, err := store.SaveIfNew(ctx, e1)
	if err != nil {
		t.Fatalf("SaveIfNew after eviction: %v", err)
	}
	if !created {
		t.Error("expected e1 to be re-creatable after eviction")
	}
}

func mustCreate(t *testing.T, s onceo.Store, ctx context.Context, e onceo.Event) {
	t.Helper()
	created, err := s.SaveIfNew(ctx, e)
	if err != nil {
		t.Fatalf("SaveIfNew: %v", err)
	}
	if !created {
		t.Fatalf("expected event %s/%s to be created", e.Provider, e.ProviderEventID)
	}
}

func TestMemoryStoreMaxItemsZero(t *testing.T) {
	store := onceo.NewMemoryStoreWithMax(0)
	if store == nil {
		t.Fatal("NewMemoryStoreWithMax(0) returned nil")
	}
	ctx := context.Background()
	for i := 0; i < 200000; i++ {
		e := onceo.Event{
			Provider:        "p",
			ProviderEventID: fmt.Sprintf("evt_%d", i),
			Type:            "t",
			Status:          onceo.StatusSuccess,
		}
		created, err := store.SaveIfNew(ctx, e)
		if err != nil {
			t.Fatalf("SaveIfNew at %d: %v", i, err)
		}
		if !created {
			t.Fatalf("expected creation at %d", i)
		}
	}
}

func TestProviderEventIDTooLong(t *testing.T) {
	provider := &testProvider{
		name: "test",
		normalizeResult: onceo.Event{
			Provider:        "test",
			ProviderEventID: strings.Repeat("a", 257),
			Type:            "charge.success",
			Status:          onceo.StatusSuccess,
		},
	}

	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for ProviderEventID exceeding 256 bytes")
	}
	if !strings.Contains(err.Error(), "256") {
		t.Errorf("expected error mentioning 256, got: %v", err)
	}
}

type testProvider struct {
	name            string
	verifyErr       error
	normalizeResult onceo.Event
}

func (p *testProvider) Name() string { return p.name }
func (p *testProvider) BodyBound() bool { return true }
func (p *testProvider) VerifySignature(_ http.Header, _ []byte) error { return p.verifyErr }
func (p *testProvider) Parse(_ []byte) (testProviderData, error) { return testProviderData{}, nil }
func (p *testProvider) Normalize(_ testProviderData) (onceo.Event, error) { return p.normalizeResult, nil }

type testProviderData struct {
	ID string `json:"id"`
}

func TestProcessMissingHeaders(t *testing.T) {
	provider, _ := paystack.New("sk_test")
	body := paystackChargeSuccessBody()

	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), http.Header{}, body)
	if !errors.Is(err, onceo.ErrInvalidSignature) && !errors.Is(err, onceo.ErrMissingHeader) {
		t.Fatalf("expected signature/header error, got %v", err)
	}
}

func TestProcessEmptyProviderName(t *testing.T) {
	provider := &testProvider{name: ""}
	_, err := onceo.Process(context.Background(), provider, onceo.NewMemoryStore(), http.Header{}, []byte(`{}`))
	if err == nil {
		t.Fatal("expected error for empty provider name")
	}
}

func TestEventJSONRoundTrip(t *testing.T) {
	orig := onceo.Event{
		ID:              "evt_001",
		Provider:        "paystack",
		ProviderEventID: "123",
		Type:            "charge.success",
		Status:          onceo.StatusSuccess,
		AmountMinor:     50000,
		Currency:        "NGN",
		Reference:       "ref_001",
		RawPayload:      []byte(`{"event":"charge.success"}`),
		ReceivedAt:      time.Now().UTC(),
		ProcessedAt:     time.Now().UTC(),
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded onceo.Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.ID != orig.ID {
		t.Errorf("ID: got %q, want %q", decoded.ID, orig.ID)
	}
	if decoded.Status != orig.Status {
		t.Errorf("Status: got %v, want %v", decoded.Status, orig.Status)
	}
	if decoded.AmountMinor != orig.AmountMinor {
		t.Errorf("AmountMinor: got %d, want %d", decoded.AmountMinor, orig.AmountMinor)
	}
}

func TestProcessNilContext(t *testing.T) {
	provider, _ := paystack.New("sk_test")
	body := paystackChargeSuccessBody()
	headers := paystackHeaders("sk_test", body)

	// nil context is not allowed per Go's context contract; Process should
	// behave deterministically (panic or error, not silently succeed).
	defer func() {
		if r := recover(); r == nil {
			// If it doesn't panic, at minimum verify it errors predictably
		}
	}()
	_, err := onceo.Process(nil, provider, onceo.NewMemoryStore(), headers, body)
	if err == nil {
		t.Error("expected Process(nil) to fail")
	}
}

func TestRedisStoreReservedChars(t *testing.T) {
	store := &redisstore.Store{}

	_, err := store.SaveIfNew(context.Background(), onceo.Event{
		Provider: "pay:stack", ProviderEventID: "evt_001",
	})
	if err == nil {
		t.Error("expected error for provider name with colon")
	}

	_, err = store.SaveIfNew(context.Background(), onceo.Event{
		Provider: "paystack", ProviderEventID: "evt\\001",
	})
	if err == nil {
		t.Error("expected error for event ID with backslash")
	}

	_, err = store.SaveIfNew(context.Background(), onceo.Event{
		Provider: "pay\\stack", ProviderEventID: "evt:001",
	})
	if err == nil {
		t.Error("expected error for both fields with reserved chars")
	}
}

func TestEventRawPayloadNilRoundTrip(t *testing.T) {
	orig := onceo.Event{
		ID:              "evt_nil",
		Provider:        "paystack",
		ProviderEventID: "456",
		Type:            "charge.success",
		Status:          onceo.StatusSuccess,
		AmountMinor:     50000,
		Currency:        "NGN",
		// RawPayload intentionally nil
	}

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded onceo.Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.RawPayload != nil {
		t.Error("expected RawPayload to remain nil after round-trip")
	}
}
