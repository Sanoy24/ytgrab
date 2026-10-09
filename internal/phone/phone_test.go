package phone

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryStore) GetSetting(_ context.Context, key string) (string, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	value, ok := m.values[key]
	return value, ok, nil
}

func (m *memoryStore) PutSetting(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
	return nil
}

// newService is phone access, turned on, in front of an app that answers "app".
func newService(t *testing.T, store *memoryStore) *Service {
	t.Helper()
	service, err := New(context.Background(), store, 0)
	if err != nil {
		t.Fatal(err)
	}
	service.address = func() string { return "192.168.1.5" }
	// Loopback only, so tests don't ask the firewall for network access.
	service.listenOn = func(network, address string) (net.Listener, error) { return net.Listen(network, "127.0.0.1"+address) }
	service.Start(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("app")) }))
	t.Cleanup(service.Close)
	if err := service.SetEnabled(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	return service
}

// visit makes a request as a phone on the Wi-Fi.
func visit(service *Service, method, target, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://192.168.1.5:8788"+target, nil)
	request.RemoteAddr = "192.168.1.20:51000"
	if key != "" {
		request.AddCookie(&http.Cookie{Name: CookieName, Value: key})
	}
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	return response
}

func pairPhone(t *testing.T, service *Service) string {
	t.Helper()
	pairing, err := service.NewPairing()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pairing.QR, "data:image/svg+xml;base64,") || !strings.HasPrefix(pairing.URL, "http://192.168.1.5:") {
		t.Fatalf("pairing = %+v", pairing)
	}
	link, _ := url.Parse(pairing.URL)
	response := visit(service, http.MethodGet, "/pair?"+link.RawQuery, "")
	if response.Code != http.StatusSeeOther {
		t.Fatalf("pair = %d %s", response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == CookieName && cookie.HttpOnly && cookie.Value != "" {
			return cookie.Value
		}
	}
	t.Fatalf("no key cookie: %v", response.Header())
	return ""
}

func TestPairing(t *testing.T) {
	store := &memoryStore{values: map[string]string{}}
	service := newService(t, store)

	if got := visit(service, http.MethodGet, "/api/jobs", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("unpaired API = %d", got.Code)
	}
	if got := visit(service, http.MethodGet, "/", ""); got.Code != http.StatusUnauthorized || !strings.Contains(got.Body.String(), "Settings → Phone") {
		t.Fatalf("unpaired page = %d %s", got.Code, got.Body.String())
	}
	if got := visit(service, http.MethodGet, "/pair?code=guess", ""); got.Code != http.StatusForbidden {
		t.Fatalf("wrong code = %d", got.Code)
	}

	pairing, _ := service.NewPairing()
	key := pairPhone(t, service) // replaces the code above
	link, _ := url.Parse(pairing.URL)
	if got := visit(service, http.MethodGet, "/pair?"+link.RawQuery, ""); got.Code != http.StatusForbidden {
		t.Errorf("a replaced code still works: %d", got.Code)
	}
	if got := visit(service, http.MethodGet, "/api/jobs", key); got.Code != http.StatusOK || got.Body.String() != "app" {
		t.Fatalf("paired API = %d %s", got.Code, got.Body.String())
	}

	// Paired phones survive a restart; only the hash of the key is stored.
	if strings.Contains(store.values[devicesKey], key) {
		t.Fatal("the key itself was stored")
	}
	again := newService(t, store)
	if got := visit(again, http.MethodGet, "/api/jobs", key); got.Code != http.StatusOK {
		t.Fatalf("after restart = %d", got.Code)
	}
	status := again.Status()
	if len(status.Devices) != 1 || !status.Enabled || status.URL == "" {
		t.Fatalf("status = %+v", status)
	}

	// Removing the phone stops its key at once.
	if err := again.Forget(context.Background(), status.Devices[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := visit(again, http.MethodGet, "/api/jobs", key); got.Code != http.StatusUnauthorized {
		t.Errorf("forgotten phone = %d", got.Code)
	}
}

func TestCodesExpireAndTurningOffForgetsPhones(t *testing.T) {
	service := newService(t, &memoryStore{values: map[string]string{}})
	now := time.Now()
	service.now = func() time.Time { return now }
	pairing, _ := service.NewPairing()
	now = now.Add(11 * time.Minute)
	link, _ := url.Parse(pairing.URL)
	if got := visit(service, http.MethodGet, "/pair?"+link.RawQuery, ""); got.Code != http.StatusForbidden {
		t.Errorf("expired code = %d", got.Code)
	}

	key := pairPhone(t, service)
	if err := service.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewPairing(); err != ErrOff {
		t.Errorf("pairing while off = %v", err)
	}
	_ = service.SetEnabled(context.Background(), true)
	if got := visit(service, http.MethodGet, "/api/jobs", key); got.Code != http.StatusUnauthorized {
		t.Errorf("phone kept after turning off = %d", got.Code)
	}
}

func TestPhonesStayInTheirLane(t *testing.T) {
	service := newService(t, &memoryStore{values: map[string]string{}})
	key := pairPhone(t, service)
	for _, c := range []struct {
		method, target string
		want           int
	}{
		{"GET", "/", 200},
		{"GET", "/app.js", 200},
		{"POST", "/api/jobs", 200},
		{"GET", "/api/jobs/abc/file", 200},
		{"GET", "/api/jobs/abc/events", 200},
		{"DELETE", "/api/jobs/abc", 200},
		{"PUT", "/api/watches/abc", 200},
		{"DELETE", "/api/jobs/abc?delete_file=true", 403},
		{"POST", "/api/history/clear?delete_files=true", 403},
		{"PUT", "/api/settings/preferences", 403},
		{"POST", "/api/settings/pick-folder", 403},
		{"PUT", "/api/settings/cookies", 403},
		{"POST", "/api/jobs/abc/open", 403},
		{"POST", "/api/jobs/abc/reveal", 403},
		{"GET", "/api/backup", 403},
		{"POST", "/api/backup/restore", 403},
		{"POST", "/api/system/update-ytgrab", 403},
		{"GET", "/api/phone", 403},
		{"POST", "/api/phone/pair", 403},
		{"POST", "/", 403},
		{"GET", "/api/added-later", 403},
	} {
		if got := visit(service, c.method, c.target, key); got.Code != c.want {
			t.Errorf("%s %s = %d, want %d", c.method, c.target, got.Code, c.want)
		}
	}
}

func TestFeedsNeedNoPairingButTheirOwnKey(t *testing.T) {
	store := &memoryStore{values: map[string]string{}}
	service := newService(t, store)
	if got := visit(service, http.MethodGet, "/feeds/w1?key=whatever", ""); got.Code != http.StatusOK {
		t.Errorf("feed from an unpaired podcast app = %d (the feed itself checks the key)", got.Code)
	}
	if got := visit(service, http.MethodPost, "/feeds/w1", ""); got.Code != http.StatusUnauthorized {
		t.Errorf("POST to a feed = %d", got.Code)
	}
	ctx := context.Background()
	key, err := service.FeedKey(ctx)
	if again, _ := service.FeedKey(ctx); err != nil || key == "" || again != key {
		t.Fatalf("FeedKey = %q, %q, %v", key, again, err)
	}
	if !service.ValidFeedKey(ctx, key) || service.ValidFeedKey(ctx, "") || service.ValidFeedKey(ctx, key+"x") {
		t.Error("ValidFeedKey")
	}
	_ = service.ResetFeedKey(ctx)
	if service.ValidFeedKey(ctx, key) {
		t.Error("the old key still works after a reset")
	}
}

func TestOnlyTheLocalNetwork(t *testing.T) {
	service := newService(t, &memoryStore{values: map[string]string{}})
	key := pairPhone(t, service)
	for _, c := range []struct{ remote, host string }{
		{"8.8.8.8:5000", "192.168.1.5:8788"},       // from the internet through a forwarded port
		{"[2001:db8::1]:5000", "192.168.1.5:8788"}, // a public IPv6 address
		{"192.168.1.20:5000", "evil.example:8788"}, // DNS rebinding
	} {
		request := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
		request.RemoteAddr, request.Host = c.remote, c.host
		request.AddCookie(&http.Cookie{Name: CookieName, Value: key})
		response := httptest.NewRecorder()
		service.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Errorf("%s via %s = %d", c.remote, c.host, response.Code)
		}
	}
}

func TestPhoneServerListens(t *testing.T) {
	service := newService(t, &memoryStore{values: map[string]string{}})
	service.mu.Lock()
	running := service.server != nil
	service.mu.Unlock()
	if !running {
		t.Fatalf("not listening: %+v", service.Status())
	}
	_ = service.SetEnabled(context.Background(), false)
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.server != nil {
		t.Fatal("still listening after turning off")
	}
}

func TestDeviceNames(t *testing.T) {
	for agent, want := range map[string]string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1": "iPhone · Safari",
		"Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Mobile Safari/537.36":                         "Android phone · Chrome",
		"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/27.0 Chrome/125.0 Mobile Safari/537.36":  "Android phone · Samsung Internet",
		"": "Phone",
	} {
		if got := deviceName(agent); got != want {
			t.Errorf("deviceName(%q) = %q, want %q", agent, got, want)
		}
	}
}

func TestAnAddressChangeIsFlaggedAndRepairingReplacesThePhone(t *testing.T) {
	service := newService(t, &memoryStore{values: map[string]string{}})
	pairPhone(t, service) // at 192.168.1.5
	if service.Status().Moved {
		t.Fatal("moved before the address changed")
	}

	// The router gives this computer a new address: the phone's key belonged to the old one.
	service.address = func() string { return "192.168.1.6" }
	status := service.Status()
	if !status.Moved || !status.Devices[0].Moved || status.URL != service.base("192.168.1.6") {
		t.Fatalf("status = %+v", status)
	}

	pairing, _ := service.NewPairing()
	link, _ := url.Parse(pairing.URL)
	request := httptest.NewRequest(http.MethodGet, pairing.URL, nil)
	request.RemoteAddr = "192.168.1.20:51000"
	if request.Host != link.Host {
		t.Fatalf("host = %s", request.Host)
	}
	response := httptest.NewRecorder()
	service.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("pair again = %d", response.Code)
	}
	status = service.Status()
	if status.Moved || len(status.Devices) != 1 {
		t.Fatalf("after pairing again = %+v", status)
	}
}
