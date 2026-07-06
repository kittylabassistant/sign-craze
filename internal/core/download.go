package core

import "github.com/kittylabassistant/sign-craze/internal/ghrelease"

// FromFetchResult конвертирует ghrelease.FetchResult (результат
// ghrelease.Downloader.Fetch) в DownloadResult ядра. Поля совпадают 1:1 —
// общий адаптер, чтобы каждый Download() ядра (sing-box/xray/mihomo) не
// дублировал одинаковый литерал сборки.
func FromFetchResult(res ghrelease.FetchResult) DownloadResult {
	return DownloadResult{
		Downloaded: res.Downloaded,
		Version:    res.Version,
		Path:       res.Path,
	}
}
