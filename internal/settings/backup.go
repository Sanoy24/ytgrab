package settings

import (
	"context"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

// Snapshot is the user's preferences as saved in a backup. Starting at sign-in is left
// out: it belongs to the computer, not the person.
type Snapshot struct {
	DownloadsDir    string        `json:"downloads_dir,omitempty"`
	MaxDownloads    int           `json:"max_downloads"`
	DefaultPreset   domain.Preset `json:"default_preset"`
	SpeedLimitKBps  int           `json:"speed_limit_kbps"`
	SubtitlesMode   string        `json:"subtitles_mode"`
	SubtitlesLang   string        `json:"subtitles_lang"`
	CookiesBrowser  string        `json:"cookies_browser"`
	AutoUpdateYtdlp bool          `json:"auto_update_ytdlp"`
	FileNames       string        `json:"file_names"`
	SponsorBlock    string        `json:"sponsorblock"`
	NormalizeAudio  bool          `json:"normalize_audio"`
	DownloadWindow  string        `json:"download_window"`
	YtdlpChannel    string        `json:"ytdlp_channel,omitempty"`
	SaveMetadata    bool          `json:"save_metadata,omitempty"`
}

// Snapshot returns the current preferences.
func (manager *Manager) Snapshot() Snapshot {
	mode, lang := manager.Subtitles()
	snapshot := Snapshot{
		MaxDownloads:    manager.MaxDownloads(),
		DefaultPreset:   manager.DefaultPreset(),
		SpeedLimitKBps:  manager.SpeedLimit(),
		SubtitlesMode:   mode,
		SubtitlesLang:   lang,
		CookiesBrowser:  manager.CookiesBrowser(),
		AutoUpdateYtdlp: manager.AutoUpdateYtdlp(),
		FileNames:       manager.FileNames(),
		SponsorBlock:    manager.SponsorBlock(),
		NormalizeAudio:  manager.NormalizeAudio(),
		DownloadWindow:  manager.DownloadWindow().String(),
		YtdlpChannel:    manager.YtdlpChannel(),
		SaveMetadata:    manager.SaveMetadata(),
	}
	if manager.Configured() {
		snapshot.DownloadsDir = manager.DownloadsDir()
	}
	return snapshot
}

// Restore applies a backup's preferences through the same checks as the settings page.
// A value that doesn't apply here, like a download folder this computer doesn't have, is
// skipped and named in the result, and the current value stays.
func (manager *Manager) Restore(ctx context.Context, snapshot Snapshot) (skipped []string) {
	apply := func(name string, err error) {
		if err != nil {
			skipped = append(skipped, name)
		}
	}
	if snapshot.DownloadsDir != "" {
		apply("download folder", manager.SetDownloadsDir(ctx, snapshot.DownloadsDir))
	}
	apply("parallel downloads", manager.SetMaxDownloads(ctx, snapshot.MaxDownloads))
	apply("default format", manager.SetDefaultPreset(ctx, snapshot.DefaultPreset))
	apply("speed limit", manager.SetSpeedLimit(ctx, snapshot.SpeedLimitKBps))
	apply("subtitles", manager.SetSubtitles(ctx, snapshot.SubtitlesMode, snapshot.SubtitlesLang))
	apply("browser sign-in", manager.SetCookiesBrowser(ctx, snapshot.CookiesBrowser))
	apply("yt-dlp updates", manager.SetAutoUpdateYtdlp(ctx, snapshot.AutoUpdateYtdlp))
	apply("file names", manager.SetFileNames(ctx, snapshot.FileNames))
	apply("SponsorBlock", manager.SetSponsorBlock(ctx, snapshot.SponsorBlock))
	apply("loudness", manager.SetNormalizeAudio(ctx, snapshot.NormalizeAudio))
	apply("download window", manager.SetDownloadWindow(ctx, snapshot.DownloadWindow))
	apply("metadata files", manager.SetSaveMetadata(ctx, snapshot.SaveMetadata))
	if snapshot.YtdlpChannel != "" {
		apply("yt-dlp builds", manager.SetYtdlpChannel(ctx, snapshot.YtdlpChannel))
	}
	return skipped
}
