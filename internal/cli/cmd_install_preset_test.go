package cli

import (
	"testing"
)

func TestParsePresetFlag(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantName string
		wantRest []string
	}{
		{
			name:     "no flag",
			args:     []string{"--proxy", "vless://..."},
			wantName: "",
			wantRest: []string{"--proxy", "vless://..."},
		},
		{
			name:     "space form",
			args:     []string{"--preset", "ru-direct", "--proxy", "X"},
			wantName: "ru-direct",
			wantRest: []string{"--proxy", "X"},
		},
		{
			name:     "equals form",
			args:     []string{"--preset=sign-craze-default"},
			wantName: "sign-craze-default",
			wantRest: []string{},
		},
		{
			name:     "trailing equals",
			args:     []string{"--proxy", "X", "--preset=block-ads"},
			wantName: "block-ads",
			wantRest: []string{"--proxy", "X"},
		},
		{
			name:     "dangling --preset (no value)",
			args:     []string{"--preset"},
			wantName: "",
			wantRest: []string{"--preset"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, rest := parseStringFlag(tc.args, "preset")
			if got != tc.wantName {
				t.Errorf("name: got %q, want %q", got, tc.wantName)
			}
			if len(rest) != len(tc.wantRest) {
				t.Errorf("rest len: got %v, want %v", rest, tc.wantRest)
				return
			}
			for i := range rest {
				if rest[i] != tc.wantRest[i] {
					t.Errorf("rest[%d]: got %q, want %q", i, rest[i], tc.wantRest[i])
				}
			}
		})
	}
}
