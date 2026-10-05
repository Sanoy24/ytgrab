package settings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Sanoy24/ytgrab/internal/domain"
)

const (
	downloadsKey     = "downloads_dir"
	cookiesKey       = "cookies_browser"
	maxDownloadsKey  = "max_downloads"
	defaultPresetKey = "default_preset"
	subtitlesKey     = "subtitles_mode"
	subtitleLangKey  = "subtitles_lang"
	speedLimitKey    = "speed_limit_kbps"
	autoUpdateKey    = "auto_update_ytdlp"
	fileNamesKey     = "file_names"
	sponsorBlockKey  = "sponsorblock"
	windowKey        = "download_window"
	normalizeKey     = "normalize_audio"
	ytdlpChannelKey  = "ytdlp_channel"
	metadataKey      = "save_metadata"
)

// SponsorBlockModes: leave sponsor segments alone, mark them as chapters, or cut them out.
var SponsorBlockModes = []string{"off", "mark", "remove"}

// FileNameStyles are the offered ways to name files. Every style keeps the video ID, which
// keeps names unique and marks the files YTGrab may delete.
var FileNameStyles = []string{"title", "channel-title", "date-title", "channel-folder"}

// SpeedLimits are the offered per-download limits in kB/s; 0 means no limit.
var SpeedLimits = []int{0, 500, 1000, 2000, 5000, 10000}

// Subtitle modes: off, embedded in the video file, or saved next to it as .srt.
const (
	SubtitlesOff   = "off"
	SubtitlesEmbed = "embed"
	SubtitlesFile  = "file"
)

// SubtitleLanguages are the languages offered for subtitles. Only these codes are
// accepted, so the setting can never become an arbitrary yt-dlp option.
var SubtitleLanguages = []string{"en", "am", "ar", "de", "es", "fr", "hi", "id", "it", "ja", "ko", "nl", "pl", "pt", "ru", "sw", "tr", "uk", "vi", "zh"}

// Parallel downloads are bounded: more rarely helps against YouTube's limits.
const (
	MinDownloads     = 1
	MaxDownloadsCap  = 4
	defaultDownloads = 2
)

var ErrInvalidPreference = errors.New("That setting value isn't allowed.")

// Browsers yt-dlp can read YouTube sign-in cookies from. Only these names are accepted,
// so the setting can never become an arbitrary yt-dlp option.
var CookieBrowsers = []string{"firefox", "chrome", "edge", "brave", "chromium", "opera", "vivaldi", "safari"}

var ErrInvalidBrowser = errors.New("Choose one of the listed browsers.")

var ErrInvalidDirectory = errors.New("Choose an existing, writable absolute folder.")

type Store interface {
	GetSetting(context.Context, string) (string, bool, error)
	PutSetting(context.Context, string, string) error
}

type Manager struct {
	mu           sync.RWMutex
	store        Store
	downloadsDir string
	cookies      string
	cookieFile   string // where an imported cookies.txt is kept
	maxDownloads int
	preset       domain.Preset
	subtitles    string
	subtitleLang string
	speedLimit   int
	autoUpdate   bool
	fileNames    string
	sponsorBlock string
	window       domain.Window
	onWindow     func() // called after the download window changes
	normalize    bool
	ytdlpChannel string // "stable" or "nightly"
	saveMetadata bool   // write info.json, description, and thumbnail next to each file
	defaultDir   string
	configured   bool
}

// New loads the saved output folder. Until the user chooses one (or sets
// YTGRAB_DOWNLOAD_DIR, reported as explicit), the page asks for it before downloading.
func New(ctx context.Context, store Store, defaultDirectory string, explicit bool) (*Manager, error) {
	directory, found, err := store.GetSetting(ctx, downloadsKey)
	if err != nil {
		return nil, fmt.Errorf("load output folder: %w", err)
	}
	if !found {
		directory = defaultDirectory
	}
	cookies, _, err := store.GetSetting(ctx, cookiesKey)
	if err != nil {
		return nil, fmt.Errorf("load sign-in setting: %w", err)
	}
	if !slices.Contains(CookieBrowsers, cookies) && cookies != CookieFileSetting {
		cookies = ""
	}
	maxDownloads := defaultDownloads
	if saved, ok, err := store.GetSetting(ctx, maxDownloadsKey); err == nil && ok {
		if n, err := strconv.Atoi(saved); err == nil && n >= MinDownloads && n <= MaxDownloadsCap {
			maxDownloads = n
		}
	}
	preset := domain.VideoBest
	if saved, ok, err := store.GetSetting(ctx, defaultPresetKey); err == nil && ok && domain.Preset(saved).Valid() {
		preset = domain.Preset(saved)
	}
	subtitles, subtitleLang := SubtitlesOff, "en"
	if saved, ok, err := store.GetSetting(ctx, subtitlesKey); err == nil && ok && validSubtitleMode(saved) {
		subtitles = saved
	}
	if saved, ok, err := store.GetSetting(ctx, subtitleLangKey); err == nil && ok && slices.Contains(SubtitleLanguages, saved) {
		subtitleLang = saved
	}
	speedLimit := 0
	if saved, ok, err := store.GetSetting(ctx, speedLimitKey); err == nil && ok {
		if n, err := strconv.Atoi(saved); err == nil && slices.Contains(SpeedLimits, n) {
			speedLimit = n
		}
	}
	// On unless turned off: an old yt-dlp is the usual reason YouTube downloads fail.
	autoUpdate := true
	if saved, ok, err := store.GetSetting(ctx, autoUpdateKey); err == nil && ok {
		autoUpdate = saved != "0"
	}
	fileNames := FileNameStyles[0]
	if saved, ok, err := store.GetSetting(ctx, fileNamesKey); err == nil && ok && slices.Contains(FileNameStyles, saved) {
		fileNames = saved
	}
	sponsorBlock := SponsorBlockModes[0]
	if saved, ok, err := store.GetSetting(ctx, sponsorBlockKey); err == nil && ok && slices.Contains(SponsorBlockModes, saved) {
		sponsorBlock = saved
	}
	var window domain.Window
	if saved, ok, err := store.GetSetting(ctx, windowKey); err == nil && ok {
		window, _ = domain.ParseWindow(saved)
	}
	saveMetadata := false
	if saved, ok, err := store.GetSetting(ctx, metadataKey); err == nil && ok {
		saveMetadata = saved == "1"
	}
	ytdlpChannel := "stable"
	if saved, ok, err := store.GetSetting(ctx, ytdlpChannelKey); err == nil && ok && saved == "nightly" {
		ytdlpChannel = "nightly"
	}
	normalize := false
	if saved, ok, err := store.GetSetting(ctx, normalizeKey); err == nil && ok {
		normalize = saved == "1"
	}
	return &Manager{store: store, downloadsDir: directory, cookies: cookies, maxDownloads: maxDownloads, preset: preset, subtitles: subtitles, subtitleLang: subtitleLang, speedLimit: speedLimit, autoUpdate: autoUpdate, fileNames: fileNames, sponsorBlock: sponsorBlock, window: window, normalize: normalize, ytdlpChannel: ytdlpChannel, saveMetadata: saveMetadata, defaultDir: defaultDirectory, configured: found || explicit}, nil
}

// SaveMetadata reports whether downloads also keep their metadata as files: yt-dlp's
// info.json, the description, and the thumbnail, for archives and media servers.
func (manager *Manager) SaveMetadata() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.saveMetadata
}

func (manager *Manager) SetSaveMetadata(ctx context.Context, on bool) error {
	value := "0"
	if on {
		value = "1"
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, metadataKey, value); err != nil {
		return fmt.Errorf("save metadata setting: %w", err)
	}
	manager.saveMetadata = on
	return nil
}

// YtdlpChannel is where yt-dlp updates come from: "stable" (the default) or "nightly",
// whose fixes for YouTube changes arrive a day or two sooner.
func (manager *Manager) YtdlpChannel() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	if manager.ytdlpChannel == "nightly" {
		return "nightly"
	}
	return "stable"
}

func (manager *Manager) SetYtdlpChannel(ctx context.Context, channel string) error {
	if channel != "stable" && channel != "nightly" {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, ytdlpChannelKey, channel); err != nil {
		return fmt.Errorf("save yt-dlp channel: %w", err)
	}
	manager.ytdlpChannel = channel
	return nil
}

// NormalizeAudio reports whether converted audio (MP3, FLAC, WAV) is evened out in loudness.
func (manager *Manager) NormalizeAudio() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.normalize
}

func (manager *Manager) SetNormalizeAudio(ctx context.Context, on bool) error {
	value := "0"
	if on {
		value = "1"
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, normalizeKey, value); err != nil {
		return fmt.Errorf("save loudness setting: %w", err)
	}
	manager.normalize = on
	return nil
}

// DownloadWindow is when downloads may start; its zero value means any time.
func (manager *Manager) DownloadWindow() domain.Window {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.window
}

// SetDownloadWindow takes "" (any time) or "start-end" in whole hours.
func (manager *Manager) SetDownloadWindow(ctx context.Context, value string) error {
	window, ok := domain.ParseWindow(value)
	if !ok {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, windowKey, window.String()); err != nil {
		return fmt.Errorf("save download window: %w", err)
	}
	manager.window = window
	if manager.onWindow != nil {
		manager.onWindow()
	}
	return nil
}

// OnWindowChange registers a function to call after the download window changes, so
// waiting downloads can start straight away.
func (manager *Manager) OnWindowChange(changed func()) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	manager.onWindow = changed
}

// SponsorBlock is what to do with sponsor segments (one of SponsorBlockModes).
func (manager *Manager) SponsorBlock() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.sponsorBlock
}

func (manager *Manager) SetSponsorBlock(ctx context.Context, mode string) error {
	if !slices.Contains(SponsorBlockModes, mode) {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, sponsorBlockKey, mode); err != nil {
		return fmt.Errorf("save SponsorBlock setting: %w", err)
	}
	manager.sponsorBlock = mode
	return nil
}

// FileNames is how new downloads are named (one of FileNameStyles).
func (manager *Manager) FileNames() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.fileNames
}

func (manager *Manager) SetFileNames(ctx context.Context, style string) error {
	if !slices.Contains(FileNameStyles, style) {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, fileNamesKey, style); err != nil {
		return fmt.Errorf("save file names: %w", err)
	}
	manager.fileNames = style
	return nil
}

// AutoUpdateYtdlp reports whether YTGrab keeps yt-dlp up to date by itself.
func (manager *Manager) AutoUpdateYtdlp() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.autoUpdate
}

func (manager *Manager) SetAutoUpdateYtdlp(ctx context.Context, on bool) error {
	value := "0"
	if on {
		value = "1"
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, autoUpdateKey, value); err != nil {
		return fmt.Errorf("save automatic updates: %w", err)
	}
	manager.autoUpdate = on
	return nil
}

// SpeedLimit is the most each download may use, in kB/s; 0 means no limit.
func (manager *Manager) SpeedLimit() int {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.speedLimit
}

// SetSpeedLimit sets the per-download limit to one of SpeedLimits; it applies to
// downloads that start afterwards.
func (manager *Manager) SetSpeedLimit(ctx context.Context, kbps int) error {
	if !slices.Contains(SpeedLimits, kbps) {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, speedLimitKey, strconv.Itoa(kbps)); err != nil {
		return fmt.Errorf("save speed limit: %w", err)
	}
	manager.speedLimit = kbps
	return nil
}

func validSubtitleMode(mode string) bool {
	return mode == SubtitlesOff || mode == SubtitlesEmbed || mode == SubtitlesFile
}

// Subtitles returns the subtitle mode and language for video downloads.
func (manager *Manager) Subtitles() (mode, lang string) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.subtitles, manager.subtitleLang
}

// SetSubtitles saves the subtitle mode (off, embed, file) and language.
func (manager *Manager) SetSubtitles(ctx context.Context, mode, lang string) error {
	if !validSubtitleMode(mode) || !slices.Contains(SubtitleLanguages, lang) {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, subtitlesKey, mode); err != nil {
		return fmt.Errorf("save subtitles setting: %w", err)
	}
	if err := manager.store.PutSetting(ctx, subtitleLangKey, lang); err != nil {
		return fmt.Errorf("save subtitles setting: %w", err)
	}
	manager.subtitles, manager.subtitleLang = mode, lang
	return nil
}

// CookiesBrowser returns the browser whose YouTube sign-in yt-dlp should use, or "" (off).
func (manager *Manager) CookiesBrowser() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.cookies
}

// SetCookiesBrowser turns browser sign-in on for one of CookieBrowsers or the imported
// cookies.txt (CookieFileSetting), or off with "". Leaving the file deletes it.
func (manager *Manager) SetCookiesBrowser(ctx context.Context, browser string) error {
	if browser != "" && browser != CookieFileSetting && !slices.Contains(CookieBrowsers, browser) {
		return ErrInvalidBrowser
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if browser == CookieFileSetting {
		if info, err := os.Stat(manager.cookieFile); manager.cookieFile == "" || err != nil || !info.Mode().IsRegular() {
			return ErrNoCookieFile
		}
	}
	if err := manager.store.PutSetting(ctx, cookiesKey, browser); err != nil {
		return fmt.Errorf("save sign-in setting: %w", err)
	}
	if browser != CookieFileSetting {
		manager.removeCookieFile()
	}
	manager.cookies = browser
	return nil
}

// MaxDownloads is how many downloads may run at once.
func (manager *Manager) MaxDownloads() int {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.maxDownloads
}

// SetMaxDownloads changes how many downloads run at once (1–4); it applies immediately.
func (manager *Manager) SetMaxDownloads(ctx context.Context, n int) error {
	if n < MinDownloads || n > MaxDownloadsCap {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, maxDownloadsKey, strconv.Itoa(n)); err != nil {
		return fmt.Errorf("save parallel downloads: %w", err)
	}
	manager.maxDownloads = n
	return nil
}

// DefaultPreset is the format the page selects for new links.
func (manager *Manager) DefaultPreset() domain.Preset {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.preset
}

func (manager *Manager) SetDefaultPreset(ctx context.Context, preset domain.Preset) error {
	if !preset.Valid() {
		return ErrInvalidPreference
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, defaultPresetKey, string(preset)); err != nil {
		return fmt.Errorf("save default format: %w", err)
	}
	manager.preset = preset
	return nil
}

// Configured reports whether an output folder has been chosen.
func (manager *Manager) Configured() bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.configured
}

// DefaultDir is the suggested folder offered on first run.
func (manager *Manager) DefaultDir() string {
	return manager.defaultDir
}

// UseDefault creates the suggested folder if needed and saves it as the choice.
func (manager *Manager) UseDefault(ctx context.Context) error {
	if err := os.MkdirAll(manager.defaultDir, 0o755); err != nil {
		return ErrInvalidDirectory
	}
	return manager.SetDownloadsDir(ctx, manager.defaultDir)
}

func (manager *Manager) DownloadsDir() string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()
	return manager.downloadsDir
}

func (manager *Manager) SetDownloadsDir(ctx context.Context, directory string) error {
	if directory == "" || strings.TrimSpace(directory) != directory || !filepath.IsAbs(directory) {
		return ErrInvalidDirectory
	}
	directory = filepath.Clean(directory)
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return ErrInvalidDirectory
	}
	probe, err := os.CreateTemp(directory, ".ytgrab-write-check-*")
	if err != nil {
		return ErrInvalidDirectory
	}
	if err := probe.Close(); err != nil {
		_ = os.Remove(probe.Name())
		return ErrInvalidDirectory
	}
	if err := os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("remove write-check file: %w", err)
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if err := manager.store.PutSetting(ctx, downloadsKey, directory); err != nil {
		return fmt.Errorf("save output folder: %w", err)
	}
	manager.downloadsDir = directory
	manager.configured = true
	return nil
}
