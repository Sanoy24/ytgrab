// Package phone lets phones on the same Wi-Fi use YTGrab. It is off until the user turns it
// on. Then a second server listens on the local network, and only phones paired by scanning
// a QR code on the computer get past it, to a subset of the API: adding and managing
// downloads, watches, and saving finished files to the phone. Settings, files on the
// computer, sign-in, backups, and updates stay on the computer.
package phone

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	enabledKey = "phone_access"
	devicesKey = "phone_devices"
	feedKeyKey = "feed_key"

	// CookieName holds a paired phone's key.
	CookieName = "ytgrab_phone"
	// MaxDevices is how many phones can be paired at once.
	MaxDevices = 10

	codeLifetime = 10 * time.Minute
	keyLifetime  = 400 * 24 * time.Hour // the longest browsers keep a cookie
	seenEvery    = time.Hour            // how often a phone's last visit is saved
)

// Store keeps the setting and the paired phones.
type Store interface {
	GetSetting(ctx context.Context, key string) (string, bool, error)
	PutSetting(ctx context.Context, key, value string) error
}

// Device is a paired phone. Only a hash of its key is kept.
type Device struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	KeyHash  string    `json:"key_hash"`
	PairedAt time.Time `json:"paired_at"`
	LastSeen time.Time `json:"last_seen"`
	// Address is this computer's address the phone paired at. A phone's key belongs to that
	// address (browsers keep cookies per address), so a new address means pairing again.
	Address string `json:"address,omitempty"`
}

// DeviceInfo is a paired phone as the settings page shows it.
type DeviceInfo struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	PairedAt time.Time `json:"paired_at"`
	LastSeen time.Time `json:"last_seen"`
	Moved    bool      `json:"moved,omitempty"` // paired at an address this computer no longer has
}

// Status is what the settings page shows.
type Status struct {
	Enabled bool         `json:"enabled"`
	URL     string       `json:"url,omitempty"`   // where paired phones open YTGrab
	Error   string       `json:"error,omitempty"` // why phones can't connect
	Devices []DeviceInfo `json:"devices"`
	// Moved is set when this computer's address changed since a phone paired, so that phone
	// can't reach YTGrab until it scans a new code.
	Moved bool `json:"moved,omitempty"`
}

// Pairing is a one-time code for a new phone, as a link and its QR code.
type Pairing struct {
	URL       string    `json:"url"`
	QR        string    `json:"qr"` // a data: URL of an SVG image
	ExpiresAt time.Time `json:"expires_at"`
}

var (
	ErrOff         = errors.New("phone access is off")
	ErrNoAddress   = errors.New("this computer is not connected to a local network")
	ErrTooMany     = errors.New("too many paired phones")
	ErrUnknownCode = errors.New("the pairing code is wrong or expired")
)

// Service runs the phone server and keeps the paired phones.
type Service struct {
	store Store
	port  int
	app   http.Handler

	mu       sync.Mutex
	enabled  bool
	devices  []Device
	code     string
	codeEnds time.Time
	server   *http.Server
	listen   string // why the server could not start

	// Replaced in tests.
	now      func() time.Time
	address  func() string
	listenOn func(network, address string) (net.Listener, error)
}

// New loads the setting and paired phones. Call Start once the app's handler exists.
func New(ctx context.Context, store Store, port int) (*Service, error) {
	service := &Service{store: store, port: port, now: time.Now, address: localAddress, listenOn: net.Listen}
	if value, ok, err := store.GetSetting(ctx, enabledKey); err != nil {
		return nil, fmt.Errorf("read phone setting: %w", err)
	} else if ok {
		service.enabled = value == "1"
	}
	if value, ok, err := store.GetSetting(ctx, devicesKey); err != nil {
		return nil, fmt.Errorf("read paired phones: %w", err)
	} else if ok && value != "" {
		if err := json.Unmarshal([]byte(value), &service.devices); err != nil {
			service.devices = nil // unreadable: phones pair again
		}
	}
	return service, nil
}

// Port is where the phone server listens.
func (s *Service) Port() int { return s.port }

// Start serves app to paired phones when phone access is on.
func (s *Service) Start(app http.Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.app = app
	if s.enabled {
		s.startLocked()
	}
}

// Close stops the phone server.
func (s *Service) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

func (s *Service) startLocked() {
	if s.server != nil || s.app == nil {
		return
	}
	listener, err := s.listenOn("tcp", ":"+strconv.Itoa(s.port))
	if err != nil {
		s.listen = fmt.Sprintf("Port %d is in use by another program, so phones can't connect.", s.port)
		return
	}
	s.listen = ""
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func(server *http.Server) { _ = server.Serve(listener) }(s.server)
}

func (s *Service) stopLocked() {
	if s.server != nil {
		_ = s.server.Close() // ends open progress streams too
		s.server = nil
	}
}

// Status reports the setting, the address, and the paired phones.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := Status{Enabled: s.enabled, Devices: []DeviceInfo{}}
	if !s.enabled {
		return status
	}
	address := s.address()
	for _, device := range s.devices {
		info := DeviceInfo{ID: device.ID, Name: device.Name, PairedAt: device.PairedAt, LastSeen: device.LastSeen}
		info.Moved = address != "" && device.Address != "" && device.Address != address
		status.Moved = status.Moved || info.Moved
		status.Devices = append(status.Devices, info)
	}
	status.Error = s.listen
	if address != "" {
		status.URL = s.base(address)
	} else if status.Error == "" {
		status.Error = "This computer isn't connected to a Wi-Fi or local network."
	}
	return status
}

func (s *Service) base(address string) string {
	return "http://" + net.JoinHostPort(address, strconv.Itoa(s.port))
}

// SetEnabled turns phone access on or off. Turning it off also forgets every paired phone,
// so turning it on again always starts from scratch.
func (s *Service) SetEnabled(ctx context.Context, on bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := "0"
	if on {
		value = "1"
	}
	if err := s.store.PutSetting(ctx, enabledKey, value); err != nil {
		return fmt.Errorf("save phone setting: %w", err)
	}
	s.enabled = on
	if on {
		s.startLocked()
		return nil
	}
	s.stopLocked()
	s.code, s.listen = "", ""
	s.devices = nil
	return s.saveLocked(ctx)
}

// NewPairing makes a one-time code, replacing any earlier one.
func (s *Service) NewPairing() (Pairing, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return Pairing{}, ErrOff
	}
	if len(s.devices) >= MaxDevices {
		return Pairing{}, ErrTooMany
	}
	address := s.address()
	if address == "" {
		return Pairing{}, ErrNoAddress
	}
	s.code = randomText(16)
	s.codeEnds = s.now().Add(codeLifetime)
	link := s.base(address) + "/pair?code=" + s.code
	qr, err := qrDataURL(link)
	if err != nil {
		return Pairing{}, err
	}
	return Pairing{URL: link, QR: qr, ExpiresAt: s.codeEnds}, nil
}

// FeedKey is the secret in podcast feed addresses. Podcast apps can't pair like a browser,
// so each feed address carries it; one per install, made on first use.
func (s *Service) FeedKey(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.feedKeyLocked(ctx)
}

func (s *Service) feedKeyLocked(ctx context.Context) (string, error) {
	key, ok, err := s.store.GetSetting(ctx, feedKeyKey)
	if err != nil {
		return "", fmt.Errorf("read feed key: %w", err)
	}
	if ok && key != "" {
		return key, nil
	}
	key = randomText(24)
	if err := s.store.PutSetting(ctx, feedKeyKey, key); err != nil {
		return "", fmt.Errorf("save feed key: %w", err)
	}
	return key, nil
}

// ValidFeedKey reports whether key is the feed key.
func (s *Service) ValidFeedKey(ctx context.Context, key string) bool {
	if key == "" {
		return false
	}
	current, ok, err := s.store.GetSetting(ctx, feedKeyKey)
	return err == nil && ok && subtle.ConstantTimeCompare([]byte(key), []byte(current)) == 1
}

// ResetFeedKey replaces the feed key, so every earlier feed address stops working.
func (s *Service) ResetFeedKey(ctx context.Context) error {
	return s.store.PutSetting(ctx, feedKeyKey, randomText(24))
}

// Forget unpairs a phone; its key stops working at once.
func (s *Service) Forget(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.devices[:0:0]
	for _, device := range s.devices {
		if device.ID != id {
			kept = append(kept, device)
		}
	}
	s.devices = kept
	return s.saveLocked(ctx)
}

// pair trades a one-time code for a phone's key. address is how the phone reached this
// computer. A phone of the same kind left behind by an address change is replaced, since it
// is most likely this phone pairing again.
func (s *Service) pair(ctx context.Context, code, userAgent, address string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.code == "" || s.now().After(s.codeEnds) || subtle.ConstantTimeCompare([]byte(code), []byte(s.code)) != 1 {
		return "", ErrUnknownCode
	}
	if len(s.devices) >= MaxDevices {
		return "", ErrTooMany
	}
	s.code = "" // one phone per code
	key := randomText(32)
	now := s.now()
	name := deviceName(userAgent)
	before := s.devices
	devices := make([]Device, 0, len(s.devices)+1)
	replaced := false
	for _, device := range s.devices {
		if !replaced && device.Name == name && device.Address != "" && device.Address != address {
			replaced = true
			continue
		}
		devices = append(devices, device)
	}
	s.devices = append(devices, Device{ID: randomText(8), Name: name, KeyHash: hashKey(key), PairedAt: now, LastSeen: now, Address: address})
	if err := s.saveLocked(ctx); err != nil {
		s.devices = before
		return "", err
	}
	return key, nil
}

// paired reports whether key belongs to a paired phone, noting the visit.
func (s *Service) paired(key string) bool {
	if key == "" {
		return false
	}
	hash := hashKey(key)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.devices {
		if subtle.ConstantTimeCompare([]byte(hash), []byte(s.devices[i].KeyHash)) == 1 {
			if now := s.now(); now.Sub(s.devices[i].LastSeen) > seenEvery {
				s.devices[i].LastSeen = now
				_ = s.saveLocked(context.Background())
			}
			return true
		}
	}
	return false
}

func (s *Service) saveLocked(ctx context.Context) error {
	data, err := json.Marshal(s.devices)
	if err != nil {
		return err
	}
	if s.devices == nil {
		data = []byte("[]")
	}
	if err := s.store.PutSetting(ctx, devicesKey, string(data)); err != nil {
		return fmt.Errorf("save paired phones: %w", err)
	}
	return nil
}

func randomText(bytes int) string {
	data := make([]byte, bytes)
	_, _ = rand.Read(data) // never fails
	return base64.RawURLEncoding.EncodeToString(data)
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// deviceName names a phone by its browser's user agent: "iPhone · Safari".
func deviceName(userAgent string) string {
	device := "Phone"
	switch {
	case strings.Contains(userAgent, "iPhone"):
		device = "iPhone"
	case strings.Contains(userAgent, "iPad"):
		device = "iPad"
	case strings.Contains(userAgent, "Android") && strings.Contains(userAgent, "Mobile"):
		device = "Android phone"
	case strings.Contains(userAgent, "Android"):
		device = "Android tablet"
	case strings.Contains(userAgent, "Windows"), strings.Contains(userAgent, "Macintosh"), strings.Contains(userAgent, "Linux"):
		device = "Computer"
	}
	browser := ""
	switch {
	case strings.Contains(userAgent, "SamsungBrowser"):
		browser = "Samsung Internet"
	case strings.Contains(userAgent, "Firefox"), strings.Contains(userAgent, "FxiOS"):
		browser = "Firefox"
	case strings.Contains(userAgent, "Edg"):
		browser = "Edge"
	case strings.Contains(userAgent, "Chrome"), strings.Contains(userAgent, "CriOS"):
		browser = "Chrome"
	case strings.Contains(userAgent, "Safari"):
		browser = "Safari"
	}
	if browser == "" {
		return device
	}
	return device + " · " + browser
}

// localAddress is this computer's address on its local network: the one its traffic leaves
// from, or else the first private address of a network adapter.
func localAddress() string {
	// Connecting a UDP socket only picks the route; nothing is sent.
	if conn, err := net.Dial("udp4", "192.0.2.1:9"); err == nil {
		ip := conn.LocalAddr().(*net.UDPAddr).IP
		conn.Close()
		if ip.IsPrivate() {
			return ip.String()
		}
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, adapter := range interfaces {
		if adapter.Flags&net.FlagUp == 0 || adapter.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, _ := adapter.Addrs()
		for _, address := range addresses {
			if network, ok := address.(*net.IPNet); ok && network.IP.To4() != nil && network.IP.IsPrivate() {
				return network.IP.String()
			}
		}
	}
	return ""
}
