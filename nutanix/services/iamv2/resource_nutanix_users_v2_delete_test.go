package iamv2

import (
	"errors"
	"testing"
)

func TestIsUserNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "entity not found",
			err:  errors.New("ENTITY_NOT_FOUND: user does not exist"),
			want: true,
		},
		{
			name: "generic not found",
			err:  errors.New("404 not found"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isUserNotFoundError(tt.err)
			if got != tt.want {
				t.Fatalf("isUserNotFoundError() = %v, want %v", got, tt.want)
			}
		})
	}
}
