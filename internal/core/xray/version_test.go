package xray

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		fail bool
	}{
		{
			name: "modern Xray output",
			in:   "Xray 1.8.4 (Xray, Penetrates Everything.) Custom (go1.21.5 linux/amd64)\nA unified platform for anti-censorship.",
			want: "1.8.4",
		},
		{
			name: "with v-prefix",
			in:   "Xray v1.8.4 (Xray, ...)",
			want: "1.8.4",
		},
		{
			name: "version keyword",
			in:   "xray version 1.0.0",
			want: "1.0.0",
		},
		{
			name: "empty",
			in:   "",
			fail: true,
		},
		{
			name: "no digits",
			in:   "Xray (broken build)",
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
