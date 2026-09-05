package domain_test

import (
	"errors"
	"testing"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func TestNewOwnerID(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{name: "valid", in: "default", wantErr: false},
		{name: "empty", in: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := domain.NewOwnerID(tc.in)
			if tc.wantErr {
				if !errors.Is(err, domain.ErrEmptyID) {
					t.Fatalf("want ErrEmptyID, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.String() != tc.in {
				t.Fatalf("String() = %q, want %q", got.String(), tc.in)
			}
		})
	}
}
