package main

import (
	"testing"
)

func TestEnvOr(t *testing.T) {
	// Not parallel: t.Setenv modifies process environment.
	tests := []struct {
		name     string
		key      string
		envValue *string // nil means unset, pointer to "" means set to empty.
		fallback string
		want     string
	}{
		{
			name:     "returns env value when set",
			key:      "TEST_ENVOR_SET",
			envValue: strPtr("custom-value"),
			fallback: "default",
			want:     "custom-value",
		},
		{
			name:     "returns fallback when env is empty",
			key:      "TEST_ENVOR_EMPTY",
			envValue: strPtr(""),
			fallback: "default",
			want:     "default",
		},
		{
			name:     "returns fallback when env is unset",
			key:      "TEST_ENVOR_UNSET",
			envValue: nil,
			fallback: "default",
			want:     "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != nil {
				t.Setenv(tt.key, *tt.envValue)
			}

			got := envOr(tt.key, tt.fallback)
			if got != tt.want {
				t.Fatalf("envOr(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string {
	return &s
}
