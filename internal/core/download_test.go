package core

import (
	"testing"

	"github.com/kittylabassistant/sign-craze/internal/ghrelease"
)

// TestFromFetchResult — поля ghrelease.FetchResult переносятся в
// DownloadResult 1:1 (Downloaded/Version/Path), включая нулевые значения
// (файл не перезаписан, т.к. ETag совпал).
func TestFromFetchResult(t *testing.T) {
	tests := []struct {
		name string
		in   ghrelease.FetchResult
		want DownloadResult
	}{
		{
			name: "скачан новый файл",
			in:   ghrelease.FetchResult{Downloaded: true, Version: "v1.10.0", Path: "/tmp/sing-box.tar.gz"},
			want: DownloadResult{Downloaded: true, Version: "v1.10.0", Path: "/tmp/sing-box.tar.gz"},
		},
		{
			name: "ETag совпал — файл не перезаписан",
			in:   ghrelease.FetchResult{Downloaded: false, Version: "v1.8.4", Path: "/tmp/xray.zip"},
			want: DownloadResult{Downloaded: false, Version: "v1.8.4", Path: "/tmp/xray.zip"},
		},
		{
			name: "нулевое значение",
			in:   ghrelease.FetchResult{},
			want: DownloadResult{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromFetchResult(tt.in)
			if got != tt.want {
				t.Errorf("FromFetchResult(%+v) = %+v, ожидалось %+v", tt.in, got, tt.want)
			}
		})
	}
}
