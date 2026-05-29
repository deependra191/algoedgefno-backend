package entities

import (
	"reflect"
	"testing"
	"time"
)

// TestRefreshTokenEntity_TypeShape asserts that entities.RefreshToken has the
// expected field types matching the migration 018 schema.
func TestRefreshTokenEntity_TypeShape(t *testing.T) {
	u := RefreshToken{}
	typ := reflect.TypeOf(u)

	cases := []struct {
		field    string
		wantKind reflect.Kind
		wantType reflect.Type
	}{
		{"ID", reflect.Array, nil},     // uuid.UUID
		{"UserID", reflect.Array, nil}, // uuid.UUID
		{"TokenHash", reflect.String, nil},
		{"ExpiresAt", reflect.Struct, nil}, // time.Time
		{"RevokedAt", reflect.Ptr, reflect.TypeOf((*time.Time)(nil))},
		{"CreatedAt", reflect.Struct, nil}, // time.Time
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := typ.FieldByName(tc.field)
			if !ok {
				t.Fatalf("entities.RefreshToken missing field %q", tc.field)
			}
			if f.Type.Kind() != tc.wantKind {
				t.Errorf("entities.RefreshToken.%s kind = %v, want %v", tc.field, f.Type.Kind(), tc.wantKind)
			}
			if tc.wantType != nil && f.Type != tc.wantType {
				t.Errorf("entities.RefreshToken.%s type = %v, want %v", tc.field, f.Type, tc.wantType)
			}
		})
	}
}

// TestRefreshTokenEntity_NoJSONTags asserts no json tags on entities.RefreshToken.
func TestRefreshTokenEntity_NoJSONTags(t *testing.T) {
	typ := reflect.TypeOf(RefreshToken{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		if tag := f.Tag.Get("json"); tag != "" {
			t.Errorf("entities.RefreshToken.%s has json tag %q — entities must not have json tags", f.Name, tag)
		}
	}
}
