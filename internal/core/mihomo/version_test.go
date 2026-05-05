package mihomo

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		fail bool
	}{
		{
			name: "modern Mihomo Meta",
			in:   "Mihomo Meta v1.19.24 linux/amd64 with go1.22.5 Mon Aug 12 12:00:00 UTC 2024",
			want: "1.19.24",
		},
		{
			name: "legacy Clash Meta",
			in:   "Clash Meta v1.18.0 linux/arm64 with go1.21.0",
			want: "1.18.0",
		},
		{
			name: "no v-prefix",
			in:   "Mihomo Meta 1.19.24 ...",
			want: "1.19.24",
		},
		{
			name: "empty",
			in:   "",
			fail: true,
		},
		{
			name: "no version (just text)",
			in:   "Mihomo Meta unknown build",
			fail: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseVersion(c.in)
			if c.fail {
				if err == nil {
					t.Errorf("ожидалась ошибка, получено %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ошибка: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
