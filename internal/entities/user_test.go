package entities

import (
	"reflect"
	"testing"
	"time"
)

// TestUserEntity_TypeShape asserts that entities.User has the expected fields
// matching the post-017 schema: FirebaseUID is a non-nullable string; Name and
// PasswordHash are *string (nullable); DisplayName and PhotoURL are *string;
// LastLoginAt is *time.Time; UpdatedAt is time.Time (non-nullable).
func TestUserEntity_TypeShape(t *testing.T) {
	u := User{}
	typ := reflect.TypeOf(u)

	cases := []struct {
		field    string
		wantKind reflect.Kind
		wantType reflect.Type
	}{
		{"ID", reflect.Array, nil}, // uuid.UUID is [16]byte
		{"Email", reflect.String, nil},
		{"Name", reflect.Ptr, reflect.TypeOf((*string)(nil))},
		{"PasswordHash", reflect.Ptr, reflect.TypeOf((*string)(nil))},
		{"FirebaseUID", reflect.String, nil},
		{"DisplayName", reflect.Ptr, reflect.TypeOf((*string)(nil))},
		{"PhotoURL", reflect.Ptr, reflect.TypeOf((*string)(nil))},
		{"LastLoginAt", reflect.Ptr, reflect.TypeOf((*time.Time)(nil))},
		{"CreatedAt", reflect.Struct, nil},
		{"UpdatedAt", reflect.Struct, nil},
	}

	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			f, ok := typ.FieldByName(tc.field)
			if !ok {
				t.Fatalf("entities.User missing field %q", tc.field)
			}
			if f.Type.Kind() != tc.wantKind {
				t.Errorf("entities.User.%s kind = %v, want %v", tc.field, f.Type.Kind(), tc.wantKind)
			}
			if tc.wantType != nil && f.Type != tc.wantType {
				t.Errorf("entities.User.%s type = %v, want %v", tc.field, f.Type, tc.wantType)
			}
		})
	}
}

// TestUserEntity_NoJSONTags asserts that entities.User carries no json: tags
// (rule 20 — entities must not be serialized to HTTP responses).
func TestUserEntity_NoJSONTags(t *testing.T) {
	typ := reflect.TypeOf(User{})
	for i := range typ.NumField() {
		f := typ.Field(i)
		if tag := f.Tag.Get("json"); tag != "" {
			t.Errorf("entities.User.%s has json tag %q — entities must not have json tags", f.Name, tag)
		}
	}
}
