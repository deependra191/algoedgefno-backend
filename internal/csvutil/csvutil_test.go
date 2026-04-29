package csvutil

import "testing"

func TestParseFloat(t *testing.T) {
	tests := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"100.50", 100.50, false},
		{"0", 0, false},
		{"", 0, true},
		{"-", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseFloat(tt.in)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFloat(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseFloat(%q) = %f, want %f", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseInt64(t *testing.T) {
	got, err := ParseInt64("12345")
	if err != nil || got != 12345 {
		t.Errorf("ParseInt64(\"12345\") = %d, %v", got, err)
	}
	got, _ = ParseInt64("")
	if got != 0 {
		t.Errorf("ParseInt64(\"\") = %d, want 0", got)
	}
	got, _ = ParseInt64("-")
	if got != 0 {
		t.Errorf("ParseInt64(\"-\") = %d, want 0", got)
	}
}
