package concurrency

import (
	"context"
	"sync"
	"testing"

	onceo "github.com/HalxDocs/onceo-core"
	"github.com/HalxDocs/onceo-core/providers/paystack"
)

func TestConcurrentMemoryStore(t *testing.T) {
	store := onceo.NewMemoryStore()
	ctx := context.Background()

	var wg sync.WaitGroup
	const goroutines = 100

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			e := onceo.Event{
				Provider:        "test",
				ProviderEventID: fmt.Sprintf("evt_%d", id),
				Type:            "charge.success",
				Status:          onceo.StatusSuccess,
			}
			created, err := store.SaveIfNew(ctx, e)
			if err != nil {
				t.Errorf("SaveIfNew(%d): %v", id, err)
			}
			if !created {
				t.Errorf("SaveIfNew(%d): not created", id)
			}
		}(i)
	}
	wg.Wait()

	for i := range goroutines {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			e := onceo.Event{
				Provider:        "test",
				ProviderEventID: fmt.Sprintf("evt_%d", id),
				Type:            "charge.success",
				Status:          onceo.StatusSuccess,
			}
			created, err := store.SaveIfNew(ctx, e)
			if err != nil {
				t.Errorf("SaveIfNew(%d): %v", id, err)
			}
			if created {
				t.Errorf("SaveIfNew(%d): duplicate should not be created", id)
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrentProcess(t *testing.T) {
	store := onceo.NewMemoryStore()

	var wg sync.WaitGroup
	const n = 50

	for i := range n {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			secret := fmt.Sprintf("sk_test_%d", id)
			body := []byte(fmt.Sprintf(`{"event":"charge.success","data":{"id":%d,"status":"success","reference":"ref_%d","amount":50000,"currency":"NGN"}}`, id, id))
			headers := testSignedHeaders(secret, body)

			provider, err := paystack.New(secret)
			if err != nil {
				t.Errorf("paystack.New(%d): %v", id, err)
				return
			}

			ctx := context.Background()
			_, err = onceo.Process(ctx, provider, store, headers, body)
			if err != nil {
				t.Errorf("Process(%d): %v", id, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestConcurrentSameEventIdempotency(t *testing.T) {
	store := onceo.NewMemoryStore()
	secret := "sk_test_same"
	body := []byte(`{"event":"charge.success","data":{"id":1,"status":"success","reference":"ref_001","amount":50000,"currency":"NGN"}}`)
	headers := testSignedHeaders(secret, body)

	provider, err := paystack.New(secret)
	if err != nil {
		t.Fatalf("paystack.New: %v", err)
	}

	var wg sync.WaitGroup
	const n = 20
	successCount := 0
	var mu sync.Mutex

	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := onceo.Process(context.Background(), provider, store, headers, body)
			mu.Lock()
			if err == nil {
				successCount++
			} else if !errors.Is(err, onceo.ErrDuplicateEvent) {
				t.Errorf("unexpected error: %v", err)
			}
			mu.Unlock()
		}()
	}
	wg.Wait()

	if successCount != 1 {
		t.Errorf("expected exactly 1 success, got %d", successCount)
	}
}
