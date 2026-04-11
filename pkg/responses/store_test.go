package responses

import (
	"testing"
	"time"
)

func TestStore_SaveAndGet(t *testing.T) {
	store := NewStore(1 * time.Hour)
	t.Cleanup(store.Close)

	resp := NewResponse("resp_test123", "gpt-4")
	resp.Status = StatusCompleted
	resp.OutputText = "Hello, world!"

	store.Save(resp)

	// Get should return the response
	got, ok := store.Get("resp_test123")
	if !ok {
		t.Fatal("expected to find response")
	}
	if got.ID != resp.ID {
		t.Errorf("got ID %s, want %s", got.ID, resp.ID)
	}
	if got.OutputText != resp.OutputText {
		t.Errorf("got OutputText %s, want %s", got.OutputText, resp.OutputText)
	}
}

func TestStore_GetNotFound(t *testing.T) {
	store := NewStore(1 * time.Hour)
	t.Cleanup(store.Close)

	_, ok := store.Get("nonexistent")
	if ok {
		t.Error("expected response not to be found")
	}
}

func TestStore_Delete(t *testing.T) {
	store := NewStore(1 * time.Hour)
	t.Cleanup(store.Close)

	resp := NewResponse("resp_test123", "gpt-4")
	store.Save(resp)

	// Delete should succeed
	if !store.Delete("resp_test123") {
		t.Error("expected delete to succeed")
	}

	// Get should now fail
	_, ok := store.Get("resp_test123")
	if ok {
		t.Error("expected response to be deleted")
	}

	// Delete again should return false
	if store.Delete("resp_test123") {
		t.Error("expected second delete to return false")
	}
}

func TestStore_Update(t *testing.T) {
	store := NewStore(1 * time.Hour)
	t.Cleanup(store.Close)

	resp := NewResponse("resp_test123", "gpt-4")
	resp.Status = StatusInProgress
	store.Save(resp)

	// Update should succeed
	ok := store.Update("resp_test123", func(r *Response) {
		r.Status = StatusCompleted
		r.OutputText = "Updated content"
	})
	if !ok {
		t.Error("expected update to succeed")
	}

	// Get should return updated response
	got, ok := store.Get("resp_test123")
	if !ok {
		t.Fatal("expected to find response")
	}
	if got.Status != StatusCompleted {
		t.Errorf("got Status %s, want %s", got.Status, StatusCompleted)
	}
	if got.OutputText != "Updated content" {
		t.Errorf("got OutputText %s, want Updated content", got.OutputText)
	}
}

func TestStore_UpdateNotFound(t *testing.T) {
	store := NewStore(1 * time.Hour)
	t.Cleanup(store.Close)

	ok := store.Update("nonexistent", func(r *Response) {
		r.Status = StatusCompleted
	})
	if ok {
		t.Error("expected update to fail for nonexistent response")
	}
}

func TestStore_Count(t *testing.T) {
	store := NewStore(1 * time.Hour)
	t.Cleanup(store.Close)

	if store.Count() != 0 {
		t.Errorf("expected count 0, got %d", store.Count())
	}

	store.Save(NewResponse("resp_1", "gpt-4"))
	store.Save(NewResponse("resp_2", "gpt-4"))

	if store.Count() != 2 {
		t.Errorf("expected count 2, got %d", store.Count())
	}
}

func TestStore_TTLExpiration(t *testing.T) {
	// Use a very short TTL for testing
	store := &Store{
		responses: make(map[string]*storedResponse),
		ttl:       1 * time.Millisecond,
	}

	resp := NewResponse("resp_test123", "gpt-4")
	store.Save(resp)

	// Should be found immediately
	_, ok := store.Get("resp_test123")
	if !ok {
		t.Fatal("expected to find response immediately after save")
	}

	// Wait for TTL to expire
	time.Sleep(10 * time.Millisecond)

	// Should not be found after expiration
	_, ok = store.Get("resp_test123")
	if ok {
		t.Error("expected response to be expired")
	}
}
