package web

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	t1net "github.com/TheKigen/t1net-go"
	"github.com/TheKigen/t1ms-go/internal/config"
	"github.com/TheKigen/t1ms-go/internal/master"
)

func testWebSetup() (*Service, *master.Service) {
	cfg := &config.Config{}
	cfg.Master.Name = "Test Master"
	cfg.Master.MOTD = "Test MOTD"
	cfg.Master.RateLimit = 5
	cfg.Master.MasterQueryTimeSeconds = 120
	cfg.Master.GameQueryTimeSeconds = 60
	cfg.Master.BuildPacketsEverySecs = 1
	cfg.Web.ListenAddress = "127.0.0.1:0"
	cfg.Web.BuildPagesEverySecs = 1
	cfg.Web.AddServerRateLimit = 100

	logger := log.New(os.Stderr, "test: ", 0)
	masterSvc := master.NewService(logger, cfg)
	webSvc := NewService(logger, cfg, masterSvc)
	webSvc.LoadConfig()
	return webSvc, masterSvc
}

func TestNewWebService(t *testing.T) {
	svc, _ := testWebSetup()
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
}

// --- LoadConfig ---

func TestLoadConfig_SetsAddress(t *testing.T) {
	svc, _ := testWebSetup()
	svc.LoadConfig()

	svc.mu.RLock()
	defer svc.mu.RUnlock()
	if svc.listenAddress != "127.0.0.1:0" {
		t.Errorf("listenAddress = %q, want %q", svc.listenAddress, "127.0.0.1:0")
	}
}

func TestLoadConfig_MinBuildPages(t *testing.T) {
	svc, _ := testWebSetup()
	svc.config.Web.BuildPagesEverySecs = 0
	svc.LoadConfig()

	if got := svc.buildPagesSecs.Load(); got != 1 {
		t.Errorf("buildPagesSecs = %d, want 1 (minimum)", got)
	}
}

func TestLoadConfig_CustomBuildPages(t *testing.T) {
	svc, _ := testWebSetup()
	svc.config.Web.BuildPagesEverySecs = 5
	svc.LoadConfig()

	if got := svc.buildPagesSecs.Load(); got != 5 {
		t.Errorf("buildPagesSecs = %d, want 5", got)
	}
}

// --- buildPages ---

func TestBuildPages_EmptyState(t *testing.T) {
	svc, _ := testWebSetup()
	svc.LoadConfig()
	svc.buildPages()

	svc.mu.RLock()
	defer svc.mu.RUnlock()

	if len(svc.masterPageXML) == 0 {
		t.Error("masterPageXML is empty")
	}
	if len(svc.serverPageXML) == 0 {
		t.Error("serverPageXML is empty")
	}
	if len(svc.statsPageXML) == 0 {
		t.Error("statsPageXML is empty")
	}
	if len(svc.masterPageJSON) == 0 {
		t.Error("masterPageJSON is empty")
	}
	if len(svc.serverPageJSON) == 0 {
		t.Error("serverPageJSON is empty")
	}
	if len(svc.statsPageJSON) == 0 {
		t.Error("statsPageJSON is empty")
	}
}

// --- handleXML ---

func TestHandleXML_Masters(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageXML = []byte("<masters/>")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/masters.xml", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing X-Content-Type-Options header")
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS header")
	}
	if w.Body.String() != "<masters/>" {
		t.Errorf("body = %q, want %q", w.Body.String(), "<masters/>")
	}
}

func TestHandleXML_Servers(t *testing.T) {
	svc, _ := testWebSetup()
	svc.serverPageXML = []byte("<servers/>")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers.xml", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "<servers/>" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestHandleXML_Stats(t *testing.T) {
	svc, _ := testWebSetup()
	svc.statsPageXML = []byte("<stats/>")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats.xml", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "<stats/>" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestHandleXML_HEAD(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageXML = []byte("<masters/>")

	req := httptest.NewRequest(http.MethodHead, "/api/v1/masters.xml", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleXML_MethodNotAllowed(t *testing.T) {
	svc, _ := testWebSetup()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		req := httptest.NewRequest(method, "/api/v1/masters.xml", nil)
		w := httptest.NewRecorder()
		svc.handleAPI(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// --- handleJSON ---

func TestHandleJSON_Masters(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageJSON = []byte(`{"masters":[]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/masters.json", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Body.String() != `{"masters":[]}` {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestHandleJSON_Servers(t *testing.T) {
	svc, _ := testWebSetup()
	svc.serverPageJSON = []byte(`{"servers":[]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers.json", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleJSON_Stats(t *testing.T) {
	svc, _ := testWebSetup()
	svc.statsPageJSON = []byte(`{"stats":{}}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats.json", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleJSON_HEAD(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageJSON = []byte(`{}`)

	req := httptest.NewRequest(http.MethodHead, "/api/v1/masters.json", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleJSON_MethodNotAllowed(t *testing.T) {
	svc, _ := testWebSetup()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/masters.json", nil)
		w := httptest.NewRecorder()
		svc.handleAPI(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

// --- handleAddServer ---

func TestHandleAddServer_Valid(t *testing.T) {
	svc, masterSvc := testWebSetup()
	masterSvc.LoadConfig()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.String() != "OK" {
		t.Errorf("body = %q, want %q", w.Body.String(), "OK")
	}
}

func TestHandleAddServer_MethodNotAllowed(t *testing.T) {
	svc, _ := testWebSetup()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		req := httptest.NewRequest(method, "/api/v1/addserver?address=1.2.3.4:28000", nil)
		w := httptest.NewRecorder()
		svc.handleAddServer(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: status = %d, want %d", method, w.Code, http.StatusMethodNotAllowed)
		}
	}
}

func TestHandleAddServer_MissingAddress(t *testing.T) {
	svc, _ := testWebSetup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddServer_InvalidFormat(t *testing.T) {
	svc, _ := testWebSetup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=not-host-port", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddServer_InvalidPort(t *testing.T) {
	svc, _ := testWebSetup()

	tests := []struct {
		name    string
		address string
	}{
		{"not a number", "1.2.3.4:abc"},
		{"port 0", "1.2.3.4:0"},
		{"port too high", "1.2.3.4:70000"},
		{"negative port", "1.2.3.4:-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address="+tt.address, nil)
			w := httptest.NewRecorder()
			svc.handleAddServer(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleAddServer_InvalidIP(t *testing.T) {
	svc, _ := testWebSetup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=notanip:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddServer_LoopbackIP(t *testing.T) {
	svc, _ := testWebSetup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address="+net.JoinHostPort("127.0.0.1", "28000"), nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddServer_MulticastIP(t *testing.T) {
	svc, _ := testWebSetup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=224.0.0.1:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddServer_UnspecifiedIP(t *testing.T) {
	svc, _ := testWebSetup()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=0.0.0.0:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddServer_PrivateIPs(t *testing.T) {
	svc, _ := testWebSetup()

	tests := []struct {
		name    string
		address string
	}{
		{"10.x", "10.0.0.1:28000"},
		{"172.16.x", "172.16.0.1:28000"},
		{"172.31.x", "172.31.255.1:28000"},
		{"192.168.x", "192.168.1.1:28000"},
		{"link-local", "169.254.1.1:28000"},
		{"CGNAT", "100.64.0.1:28000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address="+tt.address, nil)
			w := httptest.NewRecorder()
			svc.handleAddServer(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestHandleAddServer_APIKey_Required(t *testing.T) {
	svc, masterSvc := testWebSetup()
	masterSvc.LoadConfig()
	svc.addServerAPIKey.Store("secret-key")

	// No key provided
	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("no key: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Wrong key
	req = httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w = httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("wrong key: status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Correct key
	req = httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
	req.Header.Set("X-API-Key", "secret-key")
	w = httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("correct key: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleAddServer_APIKey_NotConfigured(t *testing.T) {
	svc, masterSvc := testWebSetup()
	masterSvc.LoadConfig()
	// addServerAPIKey is empty string (default)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (no key required when unconfigured)", w.Code, http.StatusOK)
	}
}

func TestHandleAddServer_RateLimit(t *testing.T) {
	svc, masterSvc := testWebSetup()
	masterSvc.LoadConfig()
	svc.addServerRateMax.Store(3)

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
		w := httptest.NewRecorder()
		svc.handleAddServer(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}

	// 4th request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/api/v1/addserver?address=8.8.8.8:28000", nil)
	w := httptest.NewRecorder()
	svc.handleAddServer(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("rate limited request: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

// --- handleXML/handleJSON default path ---

func TestHandleAPI_UnknownXMLPath(t *testing.T) {
	svc, _ := testWebSetup()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown.xml", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleAPI_UnknownJSONPath(t *testing.T) {
	svc, _ := testWebSetup()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/unknown.json", nil)
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// --- buildPages with data ---

func addValidatedGameServer(masterSvc *master.Service, addr string) {
	masterSvc.AddGameServer(addr)
	// Mark it as validated and populate Data with mock results
	masterSvc.RangeGameServers(func(key string, gs *master.GameServer) bool {
		if key == addr {
			gs.Mutex.Lock()
			gs.Validated = master.Validated
			gs.Data = &t1net.GameResult{
				Name:       "Test Server",
				Game:       "Tribes",
				Version:    "1.0",
				NumPlayers: 5,
				MaxPlayers: 32,
				Ping:       50 * time.Millisecond,
			}
			gs.Mutex.Unlock()
		}
		return true
	})
}

func addMockMasterServer(masterSvc *master.Service, name, addr string) {
	masterSvc.RangeMasterServers(func(key string, ms *master.MasterServer) bool {
		if key == addr {
			ms.Mutex.Lock()
			ms.Data = &t1net.MasterResult{
				Name:        name,
				MOTD:        "Mock MOTD",
				ServerCount: 5,
				Servers:     []string{"1.2.3.4:28000"},
				Ping:        25 * time.Millisecond,
			}
			ms.Mutex.Unlock()
		}
		return true
	})
}

func TestBuildPages_WithValidatedServers(t *testing.T) {
	cfg := &config.Config{}
	cfg.Master.Name = "Test Master"
	cfg.Master.MOTD = "Test MOTD"
	cfg.Master.RateLimit = 5
	cfg.Master.MasterQueryTimeSeconds = 120
	cfg.Master.GameQueryTimeSeconds = 60
	cfg.Master.BuildPacketsEverySecs = 1
	cfg.Web.ListenAddress = "127.0.0.1:0"
	cfg.Web.BuildPagesEverySecs = 1

	logger := log.New(os.Stderr, "test: ", 0)
	masterSvc := master.NewService(logger, cfg)
	masterSvc.LoadConfig()
	webSvc := NewService(logger, cfg, masterSvc)

	addValidatedGameServer(masterSvc, "5.6.7.8:28001")
	addValidatedGameServer(masterSvc, "9.10.11.12:28002")

	// Add a client with queries
	c := &master.Client{}
	c.Queries.Store(3)
	masterSvc.RangeClients(func(_ string, _ *master.Client) bool { return true }) // no-op, just to verify range works

	// Store client directly for testing
	masterSvc.AddGameServer("100.0.0.1:28000") // triggers client list population indirectly? No.
	// We need direct access. Since we're in a different package, use a workaround.

	webSvc.buildPages()

	webSvc.mu.RLock()
	defer webSvc.mu.RUnlock()

	// Verify servers XML contains server data
	var serversResp struct {
		Name         string `xml:"name"`
		MOTD         string `xml:"message-of-the-day"`
		TotalServers int    `xml:"total-servers"`
	}
	if err := xml.Unmarshal(webSvc.serverPageXML, &serversResp); err != nil {
		t.Fatalf("Failed to unmarshal servers XML: %v", err)
	}
	if serversResp.TotalServers != 2 {
		t.Errorf("TotalServers = %d, want 2", serversResp.TotalServers)
	}
	if serversResp.Name != "Test Master" {
		t.Errorf("Name = %q, want %q", serversResp.Name, "Test Master")
	}
	if serversResp.MOTD != "Test MOTD" {
		t.Errorf("MOTD = %q, want %q", serversResp.MOTD, "Test MOTD")
	}

	// Verify JSON also has servers
	var jsonResp struct {
		TotalServers int      `json:"total-servers"`
		Servers      []Server `json:"servers"`
	}
	if err := json.Unmarshal(webSvc.serverPageJSON, &jsonResp); err != nil {
		t.Fatalf("Failed to unmarshal servers JSON: %v", err)
	}
	if jsonResp.TotalServers != 2 {
		t.Errorf("JSON TotalServers = %d, want 2", jsonResp.TotalServers)
	}
	if len(jsonResp.Servers) != 2 {
		t.Errorf("JSON Servers count = %d, want 2", len(jsonResp.Servers))
	}

	// Verify local master in masters response includes server addresses
	var msResp struct {
		Masters []Master `json:"masters"`
	}
	if err := json.Unmarshal(webSvc.masterPageJSON, &msResp); err != nil {
		t.Fatalf("Failed to unmarshal masters JSON: %v", err)
	}
	if len(msResp.Masters) == 0 {
		t.Fatal("No masters in response")
	}
	local := msResp.Masters[0]
	if !local.Local {
		t.Error("First master should be local")
	}
	if local.ServerCount != 2 {
		t.Errorf("Local master ServerCount = %d, want 2", local.ServerCount)
	}
	if len(local.Server) != 2 {
		t.Errorf("Local master Server list = %d, want 2", len(local.Server))
	}

	// Verify stats
	var stats Stats
	if err := xml.Unmarshal(webSvc.statsPageXML, &stats); err != nil {
		t.Fatalf("Failed to unmarshal stats XML: %v", err)
	}
	if stats.TotalServers != 2 {
		t.Errorf("Stats TotalServers = %d, want 2", stats.TotalServers)
	}
}

func TestBuildPages_WithMasterServers(t *testing.T) {
	cfg := &config.Config{}
	cfg.Master.Name = "Test"
	cfg.Master.MOTD = "MOTD"
	cfg.Master.RateLimit = 5
	cfg.Master.MasterQueryTimeSeconds = 120
	cfg.Master.GameQueryTimeSeconds = 60
	cfg.Master.BuildPacketsEverySecs = 1
	cfg.Master.Masters.Masters = []config.MasterEntry{
		{Name: "M1", Address: "m1.test.com:28000"},
	}
	cfg.Web.ListenAddress = "127.0.0.1:0"
	cfg.Web.BuildPagesEverySecs = 1

	logger := log.New(os.Stderr, "test: ", 0)
	masterSvc := master.NewService(logger, cfg)
	masterSvc.LoadConfig()
	webSvc := NewService(logger, cfg, masterSvc)

	// Add mock data for the remote master server
	addMockMasterServer(masterSvc, "M1", "m1.test.com:28000")

	webSvc.buildPages()

	webSvc.mu.RLock()
	defer webSvc.mu.RUnlock()

	var msResp struct {
		Masters []Master `xml:"master"`
	}
	if err := xml.Unmarshal(webSvc.masterPageXML, &msResp); err != nil {
		t.Fatalf("Failed to unmarshal masters XML: %v", err)
	}
	if len(msResp.Masters) != 2 {
		t.Errorf("Masters count = %d, want 2", len(msResp.Masters))
	}
	// First entry is the local server
	if len(msResp.Masters) > 0 && msResp.Masters[0].Name != "Test" {
		t.Errorf("Local master name = %q, want %q", msResp.Masters[0].Name, "Test")
	}
	if len(msResp.Masters) > 0 && !msResp.Masters[0].Local {
		t.Error("Local master should have Local=true")
	}
	if len(msResp.Masters) > 0 && msResp.Masters[0].ServerCount != 0 {
		t.Errorf("Local master ServerCount = %d, want 0", msResp.Masters[0].ServerCount)
	}
	if len(msResp.Masters) > 0 && len(msResp.Masters[0].Server) != 0 {
		t.Errorf("Local master Server list = %d, want 0", len(msResp.Masters[0].Server))
	}
	// Second entry is the remote master
	if len(msResp.Masters) > 1 && msResp.Masters[1].Name != "M1" {
		t.Errorf("Remote master name = %q, want %q", msResp.Masters[1].Name, "M1")
	}
	if len(msResp.Masters) > 1 && msResp.Masters[1].Local {
		t.Error("Remote master should have Local=false")
	}
}

// --- Host header replacement ---

func TestHandleAPI_MastersJSON_ReplacesHostPlaceholder(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageJSON = []byte(`{"masters":[{"address":"` + localAddressPlaceholder + `:28000"}]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/masters.json", nil)
	req.Host = "myhost.example.com:8080"
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	want := `{"masters":[{"address":"myhost.example.com:28000"}]}`
	if w.Body.String() != want {
		t.Errorf("body = %q, want %q", w.Body.String(), want)
	}
}

func TestHandleAPI_MastersXML_ReplacesHostPlaceholder(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageXML = []byte(`<masters><master><address>` + localAddressPlaceholder + `:28000</address></master></masters>`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/masters.xml", nil)
	req.Host = "myhost.example.com:9090"
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	want := `<masters><master><address>myhost.example.com:28000</address></master></masters>`
	if w.Body.String() != want {
		t.Errorf("body = %q, want %q", w.Body.String(), want)
	}
}

func TestHandleAPI_MastersJSON_HostWithoutPort(t *testing.T) {
	svc, _ := testWebSetup()
	svc.masterPageJSON = []byte(`{"masters":[{"address":"` + localAddressPlaceholder + `:28000"}]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/masters.json", nil)
	req.Host = "myhost.example.com"
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	want := `{"masters":[{"address":"myhost.example.com:28000"}]}`
	if w.Body.String() != want {
		t.Errorf("body = %q, want %q", w.Body.String(), want)
	}
}

func TestHandleAPI_ServersJSON_NoReplacement(t *testing.T) {
	svc, _ := testWebSetup()
	svc.serverPageJSON = []byte(`{"servers":[]}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers.json", nil)
	req.Host = "myhost.example.com:8080"
	w := httptest.NewRecorder()
	svc.handleAPI(w, req)

	if w.Body.String() != `{"servers":[]}` {
		t.Errorf("body = %q, should not be modified", w.Body.String())
	}
}

// --- Run with empty address ---

func TestRun_NoAddress(t *testing.T) {
	cfg := &config.Config{}
	// Web.ListenAddress is empty
	logger := log.New(os.Stderr, "test: ", 0)
	masterSvc := master.NewService(logger, cfg)
	webSvc := NewService(logger, cfg, masterSvc)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := webSvc.Run(ctx)
	if err != nil {
		t.Errorf("Run with empty address should return nil, got: %v", err)
	}
}
