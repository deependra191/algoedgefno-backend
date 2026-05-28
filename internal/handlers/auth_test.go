package handlers

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/google/uuid"

	"github.com/deependra191/algoedgefno-backend/internal/models"
	"github.com/deependra191/algoedgefno-backend/internal/services"
)

// TestUserResponseJSONShape asserts that the wire shape of userResponse is
// exactly {id, email, displayName} (photoUrl is omitempty so it may or may
// not appear). Verifies that no credential or internal fields leak.
func TestUserResponseJSONShape(t *testing.T) {
	u := &models.User{
		ID:          uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email:       "a@b.c",
		DisplayName: "Tester",
		PhotoURL:    "https://example.com/photo.jpg",
	}

	r := userResponse{
		ID:          u.ID,
		Email:       u.Email,
		DisplayName: u.DisplayName,
		PhotoURL:    u.PhotoURL,
	}
	keys := jsonKeys(t, r)

	want := []string{"displayName", "email", "id", "photoUrl"}
	assertKeysEqual(t, keys, want)

	// Absence check: no credential field of any name should be present.
	forbidden := []string{"password", "password_hash", "passwordHash", "hash",
		"name", "updatedAt", "updated_at", "lastLoginAt", "last_login_at", "firebaseUID"}
	for _, f := range forbidden {
		for _, k := range keys {
			if k == f {
				t.Fatalf("forbidden key %q appeared in userResponse JSON", f)
			}
		}
	}
}

// TestUserResponseJSONShape_NoPhotoURL asserts that photoUrl is omitted when empty.
func TestUserResponseJSONShape_NoPhotoURL(t *testing.T) {
	r := userResponse{
		ID:          uuid.MustParse("11111111-2222-3333-4444-555555555555"),
		Email:       "a@b.c",
		DisplayName: "Tester",
		// PhotoURL intentionally zero-value (empty string)
	}
	keys := jsonKeys(t, r)
	// photoUrl must be absent when zero (omitempty).
	want := []string{"displayName", "email", "id"}
	assertKeysEqual(t, keys, want)
}

// TestAuthResponseJSONShape asserts the wire shape of authResponse.
func TestAuthResponseJSONShape(t *testing.T) {
	result := &services.SessionResult{
		TokenPair: services.TokenPair{
			AccessToken:  "access-tok",
			RefreshToken: "refresh-tok",
		},
		User: &models.User{
			ID:          uuid.MustParse("11111111-2222-3333-4444-555555555555"),
			Email:       "a@b.c",
			DisplayName: "Tester",
		},
	}

	resp := toAuthResponse(result)
	keys := jsonKeys(t, resp)
	want := []string{"accessToken", "refreshToken", "user"}
	assertKeysEqual(t, keys, want)

	// Walk the nested user object and ensure no credential field slipped in.
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
		case "password", "password_hash", "passwordHash", "hash", "firebaseUID",
			"updatedAt", "updated_at", "lastLoginAt", "last_login_at":
			t.Fatalf("forbidden key %q in nested user object", k)
		}
	}
}

// TestRefreshResponseJSONShape asserts that refreshResponse has exactly
// {accessToken, refreshToken} — no user field.
func TestRefreshResponseJSONShape(t *testing.T) {
	resp := refreshResponse{
		AccessToken:  "acc",
		RefreshToken: "ref",
	}
	keys := jsonKeys(t, resp)
	want := []string{"accessToken", "refreshToken"}
	assertKeysEqual(t, keys, want)
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
