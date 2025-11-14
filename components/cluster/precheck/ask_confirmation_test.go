package precheck

import (
	"bytes"
	"strings"
	"testing"
)

func TestAskForUserConfirmation(t *testing.T) {
	cases := []struct {
		name   string
		input  string
		expect bool
	}{
		{"yes-lower", "y\n", true},
		{"yes-upper", "YES\n", true},
		{"no-default", "\n", false},
		{"no-explicit", "n\n", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			restore := OverrideIO(buf, strings.NewReader(tc.input))
			defer restore()

			got, err := AskForUserConfirmation()
			if err != nil {
				t.Fatalf("AskForUserConfirmation returned error: %v", err)
			}
			if got != tc.expect {
				t.Fatalf("expected %v, got %v", tc.expect, got)
			}
		})
	}
}
