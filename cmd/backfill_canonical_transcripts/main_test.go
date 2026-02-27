package main

import (
	"errors"
	"testing"
)

// TestIsSQLiteCGOStubError handles the corresponding operation.
func TestIsSQLiteCGOStubError(t *testing.T) {
	testCases := []struct {
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
			name: "unrelated error",
			err:  errors.New("dial tcp timeout"),
			want: false,
		},
		{
			name: "cgo enabled marker",
			err:  errors.New("Binary was compiled with CGO_ENABLED=0"),
			want: true,
		},
		{
			name: "requires cgo marker",
			err:  errors.New("driver requires cgo support"),
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSQLiteCGOStubError(tc.err); got != tc.want {
				t.Fatalf("expected %v, got %v for error %v", tc.want, got, tc.err)
			}
		})
	}
}
