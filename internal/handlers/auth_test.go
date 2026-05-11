package handlers

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
)

// TestUserResponseJSONShape asserts that the wire shape of userResponse is
// exactly {id, email, name, created_at, updated_at}. Any drift (e.g. a
// forgotten password_hash field) is caught here before it reaches Android.
func TestUserResponseJSONShape(t *testing.T) {
	now := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	u := &models.User{
		ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email:     "a@b.c",
		Name:      "tester",
		CreatedAt: now,
		UpdatedAt: now,
	}

	resp := toUserResponse(u)
	keys := jsonKeys(t, resp)

	want := []string{"created_at", "email", "id", "name", "updated_at"}
	assertKeysEqual(t, keys, want)

	// Absence check: no credential field of any name should be present.
	forbidden := []string{"password", "password_hash", "passwordHash", "hash"}
	for _, f := range forbidden {
		for _, k := range keys {
			if k == f {
				t.Fatalf("forbidden key %q appeared in userResponse JSON", f)
			}
		}
	}
}

// TestAuthResponseJSONShape asserts the wire shape of authResponse.
func TestAuthResponseJSONShape(t *testing.T) {
	now := time.Date(2026, 4, 16, 10, 0, 0, 0, time.UTC)
	resp := authResponse{
		Token: "tok",
		User: toUserResponse(&models.User{
			ID:        uuid.MustParse("11111111-2222-3333-4444-555555555555"),
			Email:     "a@b.c",
			Name:      "tester",
			CreatedAt: now,
			UpdatedAt: now,
		}),
	}

	keys := jsonKeys(t, resp)
	want := []string{"token", "user"}
	assertKeysEqual(t, keys, want)

	// Walk the nested user object too and ensure no credential field slipped in.
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var nested struct {
		User map[string]json.RawMessage `json:"user"`
	}
	if err := json.Unmarshal(b, &nested); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for k := range nested.User {
		switch k {
		case "password", "password_hash", "passwordHash", "hash":
			t.Fatalf("forbidden key %q in nested user object", k)
		}
	}
}

// jsonKeys marshals v and returns its top-level JSON keys, sorted.
func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func assertKeysEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("key count mismatch: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("key mismatch at %d: got %q want %q (full got=%v)", i, got[i], want[i], got)
		}
	}
}
