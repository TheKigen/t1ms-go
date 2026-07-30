package master

import (
	"log"
	"net"
	"os"
	"testing"
	"time"

	"github.com/TheKigen/t1ms-go/internal/config"
)

func testConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Master.ListenAddress = "127.0.0.1:0"
	cfg.Master.Name = "Test"
	cfg.Master.MOTD = "MOTD"
	cfg.Master.RateLimit = 5
	cfg.Master.MasterQueryTimeSeconds = 120
	cfg.Master.GameQueryTimeSeconds = 60
	cfg.Master.BuildPacketsEverySecs = 1
	return cfg
}

func testService() *Service {
	return NewService(log.New(os.Stderr, "test: ", 0), testConfig())
}

func TestNewService(t *testing.T) {
	cfg := testConfig()
	logger := log.Default()
	svc := NewService(logger, cfg)
	if svc == nil {
		t.Fatal("NewService returned nil")
	}
	if svc.config != cfg {
		t.Error("config not set")
	}
	if svc.logger != logger {
		t.Error("logger not set")
	}
}


// --- LoadConfig ---

func TestLoadConfig_DefaultRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Master.RateLimit = 0
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.rateLimit.Load(); got != 2 {
		t.Errorf("rateLimit = %d, want 2 (default)", got)
	}
}

func TestLoadConfig_CustomRateLimit(t *testing.T) {
	cfg := testConfig()
	cfg.Master.RateLimit = 10
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.rateLimit.Load(); got != 10 {
		t.Errorf("rateLimit = %d, want 10", got)
	}
}

func TestLoadConfig_DefaultMasterQueryTime(t *testing.T) {
	cfg := testConfig()
	cfg.Master.MasterQueryTimeSeconds = 5 // below minimum 60
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.masterQueryTimeSecs.Load(); got != 120 {
		t.Errorf("masterQueryTimeSecs = %d, want 120 (default)", got)
	}
}

func TestLoadConfig_DefaultMasterQueryTimeAboveMax(t *testing.T) {
	cfg := testConfig()
	cfg.Master.MasterQueryTimeSeconds = 700 // above maximum 600
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.masterQueryTimeSecs.Load(); got != 120 {
		t.Errorf("masterQueryTimeSecs = %d, want 120 (default)", got)
	}
}

func TestLoadConfig_CustomMasterQueryTime(t *testing.T) {
	cfg := testConfig()
	cfg.Master.MasterQueryTimeSeconds = 200
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.masterQueryTimeSecs.Load(); got != 200 {
		t.Errorf("masterQueryTimeSecs = %d, want 200", got)
	}
}

func TestLoadConfig_DefaultGameQueryTime(t *testing.T) {
	cfg := testConfig()
	cfg.Master.GameQueryTimeSeconds = 3 // below minimum 10
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.gameQueryTimeSecs.Load(); got != 30 {
		t.Errorf("gameQueryTimeSecs = %d, want 30 (default)", got)
	}
}

func TestLoadConfig_CustomGameQueryTime(t *testing.T) {
	cfg := testConfig()
	cfg.Master.GameQueryTimeSeconds = 45
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.gameQueryTimeSecs.Load(); got != 45 {
		t.Errorf("gameQueryTimeSecs = %d, want 45", got)
	}
}

func TestLoadConfig_DefaultBuildPackets(t *testing.T) {
	cfg := testConfig()
	cfg.Master.BuildPacketsEverySecs = 0
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.buildPktSecs.Load(); got != 1 {
		t.Errorf("buildPktSecs = %d, want 1 (minimum)", got)
	}
}

func TestLoadConfig_DefaultMinPacketSize(t *testing.T) {
	cfg := testConfig()
	cfg.Master.MinPacketSize = 0
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.minPacketSize.Load(); got != 0 {
		t.Errorf("minPacketSize = %d, want 0 (default)", got)
	}
}

func TestLoadConfig_CustomMinPacketSize(t *testing.T) {
	cfg := testConfig()
	cfg.Master.MinPacketSize = 16
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	if got := svc.minPacketSize.Load(); got != 16 {
		t.Errorf("minPacketSize = %d, want 16", got)
	}
}

func TestLoadConfig_AddsMasters(t *testing.T) {
	cfg := testConfig()
	cfg.Master.Masters.Masters = []config.MasterEntry{
		{Name: "M1", Address: "m1.example.com:28000"},
		{Name: "M2", Address: "m2.example.com:28000"},
	}
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()

	count := 0
	svc.msList.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 2 {
		t.Errorf("master server count = %d, want 2", count)
	}
}

func TestLoadConfig_RemovesMasters(t *testing.T) {
	cfg := testConfig()
	cfg.Master.Masters.Masters = []config.MasterEntry{
		{Name: "M1", Address: "m1.example.com:28000"},
		{Name: "M2", Address: "m2.example.com:28000"},
	}
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()

	// Now reload with only one master
	cfg.Master.Masters.Masters = []config.MasterEntry{
		{Name: "M1", Address: "m1.example.com:28000"},
	}
	svc.LoadConfig()

	count := 0
	svc.msList.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("master server count after removal = %d, want 1", count)
	}
}

func TestLoadConfig_DuplicateMasterNotAdded(t *testing.T) {
	cfg := testConfig()
	cfg.Master.Masters.Masters = []config.MasterEntry{
		{Name: "M1", Address: "m1.example.com:28000"},
	}
	svc := NewService(log.New(os.Stderr, "", 0), cfg)
	svc.LoadConfig()
	svc.LoadConfig() // Load again — should not duplicate

	count := 0
	svc.msList.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count != 1 {
		t.Errorf("master server count = %d, want 1", count)
	}
}

// --- AddGameServer ---

func TestAddGameServer_New(t *testing.T) {
	svc := testService()
	svc.AddGameServer("1.2.3.4:28000")

	_, total := svc.ServerCounts()
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
}

func TestAddGameServer_ExistingUpdatesLastSeen(t *testing.T) {
	svc := testService()
	svc.AddGameServer("1.2.3.4:28000")

	valAny, _ := svc.gsList.Load("1.2.3.4:28000")
	gs := valAny.(*GameServer)
	gs.Mutex.Lock()
	gs.LastSeen = 100
	gs.Mutex.Unlock()

	svc.AddGameServer("1.2.3.4:28000")

	gs.Mutex.RLock()
	if gs.LastSeen == 100 {
		t.Error("LastSeen should have been updated")
	}
	gs.Mutex.RUnlock()
}

func TestAddGameServer_InvalidAddress(t *testing.T) {
	svc := testService()
	svc.AddGameServer("not-a-valid-address")
	_, total := svc.ServerCounts()
	if total != 0 {
		t.Errorf("total = %d, want 0 for invalid address", total)
	}
}

func TestAddGameServer_DuplicateLimit(t *testing.T) {
	svc := testService()
	// Add 21 servers with same IP, different ports
	for i := 1; i <= 21; i++ {
		svc.gsList.Store(
			net.JoinHostPort("10.0.0.1", string(rune('0'+i))),
			&GameServer{IP: net.ParseIP("10.0.0.1").To4(), Port: uint16(28000 + i), Data: nil},
		)
	}

	// 22nd should be rejected
	svc.AddGameServer("10.0.0.1:29999")

	found := false
	svc.gsList.Range(func(k, _ any) bool {
		if k.(string) == "10.0.0.1:29999" {
			found = true
			return false
		}
		return true
	})
	if found {
		t.Error("server should have been rejected due to duplicate limit")
	}
}

// --- ServerCounts ---

func TestServerCounts_Empty(t *testing.T) {
	svc := testService()
	verified, total := svc.ServerCounts()
	if verified != 0 || total != 0 {
		t.Errorf("ServerCounts = (%d, %d), want (0, 0)", verified, total)
	}
}

func TestServerCounts_Mixed(t *testing.T) {
	svc := testService()

	gs1 := &GameServer{IP: net.ParseIP("1.1.1.1").To4(), Port: 28000, Validated: Validated, Data: nil}
	gs2 := &GameServer{IP: net.ParseIP("2.2.2.2").To4(), Port: 28000, Validated: NotValidated, Data: nil}
	gs3 := &GameServer{IP: net.ParseIP("3.3.3.3").To4(), Port: 28000, Validated: Validated, Data: nil}
	gs4 := &GameServer{IP: net.ParseIP("4.4.4.4").To4(), Port: 28000, Validated: Expired, Data: nil}

	svc.gsList.Store("1.1.1.1:28000", gs1)
	svc.gsList.Store("2.2.2.2:28000", gs2)
	svc.gsList.Store("3.3.3.3:28000", gs3)
	svc.gsList.Store("4.4.4.4:28000", gs4)

	verified, total := svc.ServerCounts()
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
	if verified != 2 {
		t.Errorf("verified = %d, want 2", verified)
	}
}

// --- LastClient ---

func TestLastClient_None(t *testing.T) {
	svc := testService()
	_, _, found := svc.LastClient()
	if found {
		t.Error("LastClient should return false with no clients")
	}
}

func TestLastClient_Multiple(t *testing.T) {
	svc := testService()
	c1 := &Client{}
	c1.LastSeen.Store(100)
	c2 := &Client{}
	c2.LastSeen.Store(200)
	c3 := &Client{}
	c3.LastSeen.Store(150)

	svc.clientList.Store("10.0.0.1", c1)
	svc.clientList.Store("10.0.0.2", c2)
	svc.clientList.Store("10.0.0.3", c3)

	ip, client, found := svc.LastClient()
	if !found {
		t.Fatal("LastClient should return true")
	}
	if ip != "10.0.0.2" {
		t.Errorf("ip = %q, want %q", ip, "10.0.0.2")
	}
	if client.LastSeen.Load() != 200 {
		t.Errorf("LastSeen = %d, want 200", client.LastSeen.Load())
	}
}

// --- InvalidPackets ---

func TestInvalidPackets(t *testing.T) {
	svc := testService()
	if svc.InvalidPackets() != 0 {
		t.Errorf("InvalidPackets = %d, want 0", svc.InvalidPackets())
	}
	svc.invalidPackets.Add(5)
	if svc.InvalidPackets() != 5 {
		t.Errorf("InvalidPackets = %d, want 5", svc.InvalidPackets())
	}
}

// --- Range methods ---

func TestRangeGameServers(t *testing.T) {
	svc := testService()
	svc.gsList.Store("1.1.1.1:28000", &GameServer{IP: net.ParseIP("1.1.1.1").To4(), Data: nil})

	count := 0
	svc.RangeGameServers(func(addr string, gs *GameServer) bool {
		count++
		if addr != "1.1.1.1:28000" {
			t.Errorf("addr = %q, want %q", addr, "1.1.1.1:28000")
		}
		return true
	})
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestRangeMasterServers(t *testing.T) {
	svc := testService()
	svc.msList.Store("m1:28000", &MasterServer{Name: "M1", Address: "m1:28000", Data: nil})

	count := 0
	svc.RangeMasterServers(func(addr string, ms *MasterServer) bool {
		count++
		if ms.Name != "M1" {
			t.Errorf("Name = %q, want %q", ms.Name, "M1")
		}
		return true
	})
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestRangeClients(t *testing.T) {
	svc := testService()
	c := &Client{}
	c.Queries.Store(10)
	svc.clientList.Store("10.0.0.1", c)

	count := 0
	svc.RangeClients(func(ip string, cl *Client) bool {
		count++
		if cl.Queries.Load() != 10 {
			t.Errorf("Queries = %d, want 10", cl.Queries.Load())
		}
		return true
	})
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

// --- buildPackets ---

func TestBuildPackets_NoServers(t *testing.T) {
	svc := testService()
	svc.LoadConfig()
	svc.buildPackets()

	svc.packetsMu.RLock()
	defer svc.packetsMu.RUnlock()

	if len(svc.packets) != 1 {
		t.Fatalf("packets count = %d, want 1", len(svc.packets))
	}
	pkt := svc.packets[0]
	if len(pkt) < 8 {
		t.Errorf("packet length = %d, want >= 8", len(pkt))
	}
	if pkt[0] != 0x10 || pkt[1] != 0x06 {
		t.Errorf("packet header = [%#x, %#x], want [0x10, 0x06]", pkt[0], pkt[1])
	}
}

func TestBuildPackets_WithServers(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	gs := &GameServer{
		IP:        net.ParseIP("5.6.7.8").To4(),
		Port:      28001,
		Validated: Validated,
		Data:      nil,
	}
	svc.gsList.Store("5.6.7.8:28001", gs)

	svc.buildPackets()

	svc.packetsMu.RLock()
	defer svc.packetsMu.RUnlock()

	if len(svc.packets) < 1 {
		t.Fatal("expected at least 1 packet")
	}
	// Packet should be longer than without servers (includes server address data)
	if len(svc.packets[0]) < 12 {
		t.Errorf("packet too short with server: %d bytes", len(svc.packets[0]))
	}
}

func TestBuildPackets_IndependentBuffers(t *testing.T) {
	svc := testService()
	svc.LoadConfig()
	svc.buildPackets()

	svc.packetsMu.RLock()
	pkt1 := svc.packets[0]
	svc.packetsMu.RUnlock()

	// Build again — old packets should not be corrupted
	saved := make([]byte, len(pkt1))
	copy(saved, pkt1)

	svc.buildPackets()

	for i := range saved {
		if saved[i] != pkt1[i] {
			t.Fatalf("previous packet buffer was corrupted at byte %d", i)
		}
	}
}

// --- watchTick ---

func TestWatchTick_ExpiresValidatedServer(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	gameQuery := svc.gameQueryTimeSecs.Load()

	gs := &GameServer{
		IP:        net.ParseIP("1.1.1.1").To4(),
		Port:      28000,
		Validated: Validated,
		LastPong:  now - (gameQuery * 3),
		LastSeen:  now,
		LastPing:  now,
		Data:      nil,
	}
	gs.Querying.Store(true) // prevent actual query
	svc.gsList.Store("1.1.1.1:28000", gs)

	svc.watchTick(now)

	gs.Mutex.RLock()
	defer gs.Mutex.RUnlock()
	if gs.Validated != Expired {
		t.Errorf("Validated = %d, want Expired (%d)", gs.Validated, Expired)
	}
}

func TestWatchTick_DeletesOldServer(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	gameQuery := svc.gameQueryTimeSecs.Load()

	gs := &GameServer{
		IP:        net.ParseIP("1.1.1.1").To4(),
		Port:      28000,
		Validated: Expired,
		LastSeen:  now - (gameQuery * 11),
		LastPing:  now,
		Data:      nil,
	}
	gs.Querying.Store(true)
	svc.gsList.Store("1.1.1.1:28000", gs)

	svc.watchTick(now)

	if _, ok := svc.gsList.Load("1.1.1.1:28000"); ok {
		t.Error("server should have been deleted")
	}
}

func TestWatchTick_DecrementsClientRate(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	c := &Client{}
	c.Rate.Store(5)
	c.LastSeen.Store(time.Now().UTC().Unix())
	svc.clientList.Store("10.0.0.1", c)

	svc.watchTick(time.Now().UTC().Unix())

	if got := c.Rate.Load(); got != 4 {
		t.Errorf("Rate = %d, want 4", got)
	}
}

func TestWatchTick_DoesNotDecrementZeroRate(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	c := &Client{}
	c.Rate.Store(0)
	c.LastSeen.Store(time.Now().UTC().Unix())
	svc.clientList.Store("10.0.0.1", c)

	svc.watchTick(time.Now().UTC().Unix())

	if got := c.Rate.Load(); got != 0 {
		t.Errorf("Rate = %d, want 0", got)
	}
}

func TestWatchTick_CleansUpOldClients(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	c := &Client{}
	c.LastSeen.Store(time.Now().UTC().Unix() - 86401)
	svc.clientList.Store("10.0.0.1", c)

	svc.watchTick(time.Now().UTC().Unix())

	if _, ok := svc.clientList.Load("10.0.0.1"); ok {
		t.Error("old client should have been removed")
	}
}

func TestWatchTick_KeepsRecentClients(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	c := &Client{}
	c.LastSeen.Store(time.Now().UTC().Unix())
	svc.clientList.Store("10.0.0.1", c)

	svc.watchTick(time.Now().UTC().Unix())

	if _, ok := svc.clientList.Load("10.0.0.1"); !ok {
		t.Error("recent client should be kept")
	}
}

func TestWatchTick_TriggersMasterQuery(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	masterQuery := svc.masterQueryTimeSecs.Load()

	ms := &MasterServer{
		Name:    "Test",
		Address: "192.0.2.1:28000", // RFC 5737 TEST-NET, won't resolve
		Data:    nil,
	}
	ms.LastQuery = now - masterQuery - 10
	svc.msList.Store("192.0.2.1:28000", ms)

	svc.watchTick(now)

	// The query should have been triggered
	ms.Mutex.RLock()
	defer ms.Mutex.RUnlock()
	if ms.LastQuery != now {
		t.Errorf("LastQuery = %d, want %d", ms.LastQuery, now)
	}
	// Querying flag was set (queryMaster goroutine will clear it when done)
	// Give the goroutine a moment to start
	time.Sleep(50 * time.Millisecond)
}

func TestWatchTick_SkipsAlreadyQueryingMaster(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	masterQuery := svc.masterQueryTimeSecs.Load()

	ms := &MasterServer{
		Name:    "Test",
		Address: "192.0.2.1:28000",
		Data:    nil,
	}
	ms.LastQuery = now - masterQuery - 10
	ms.Querying.Store(true) // already querying
	originalLastQuery := ms.LastQuery
	svc.msList.Store("192.0.2.1:28000", ms)

	svc.watchTick(now)

	ms.Mutex.RLock()
	defer ms.Mutex.RUnlock()
	if ms.LastQuery != originalLastQuery {
		t.Error("LastQuery should not change when already querying")
	}
}

func TestWatchTick_TriggersGameServerQuery(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	gameQuery := svc.gameQueryTimeSecs.Load()

	gs := &GameServer{
		IP:        net.ParseIP("1.1.1.1").To4(),
		Port:      28000,
		Validated: NotValidated,
		LastPing:  now - gameQuery - 10,
		LastSeen:  now,
		Data:      nil,
	}
	svc.gsList.Store("1.1.1.1:28000", gs)

	svc.watchTick(now)

	gs.Mutex.RLock()
	defer gs.Mutex.RUnlock()
	if gs.LastPing != now {
		t.Errorf("LastPing = %d, want %d", gs.LastPing, now)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestWatchTick_SkipsAlreadyQueryingGameServer(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	gameQuery := svc.gameQueryTimeSecs.Load()

	gs := &GameServer{
		IP:        net.ParseIP("1.1.1.1").To4(),
		Port:      28000,
		Validated: NotValidated,
		LastPing:  now - gameQuery - 10,
		LastSeen:  now,
		Data:      nil,
	}
	gs.Querying.Store(true)
	originalLastPing := gs.LastPing
	svc.gsList.Store("1.1.1.1:28000", gs)

	svc.watchTick(now)

	gs.Mutex.RLock()
	defer gs.Mutex.RUnlock()
	if gs.LastPing != originalLastPing {
		t.Error("LastPing should not change when already querying")
	}
}

func TestWatchTick_DoesNotExpireNotValidated(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	now := time.Now().UTC().Unix()
	gameQuery := svc.gameQueryTimeSecs.Load()

	gs := &GameServer{
		IP:        net.ParseIP("1.1.1.1").To4(),
		Port:      28000,
		Validated: NotValidated,
		LastPong:  now - (gameQuery * 3),
		LastSeen:  now,
		LastPing:  now,
		Data:      nil,
	}
	gs.Querying.Store(true)
	svc.gsList.Store("1.1.1.1:28000", gs)

	svc.watchTick(now)

	gs.Mutex.RLock()
	defer gs.Mutex.RUnlock()
	if gs.Validated != NotValidated {
		t.Errorf("Validated = %d, want NotValidated (%d)", gs.Validated, NotValidated)
	}
}

// --- handleUDP ---

func setupUDP(t *testing.T) (*net.UDPConn, *net.UDPConn) {
	t.Helper()
	serverAddr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverConn, err := net.ListenUDP("udp4", serverAddr)
	if err != nil {
		t.Fatal(err)
	}

	actualAddr := serverConn.LocalAddr().(*net.UDPAddr)
	clientConn, err := net.DialUDP("udp4", nil, actualAddr)
	if err != nil {
		serverConn.Close()
		t.Fatal(err)
	}

	return serverConn, clientConn
}

func TestHandleUDP_Heartbeat(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	// Send heartbeat: type 0x05
	_, err := clientConn.Write([]byte{0x10, 0x05, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	serverConn.Close()
	<-errCh

	_, total := svc.ServerCounts()
	if total != 1 {
		t.Errorf("total servers = %d, want 1 after heartbeat", total)
	}

	_, _, found := svc.LastClient()
	if !found {
		t.Error("expected a client after heartbeat")
	}
}

func TestHandleUDP_Query(t *testing.T) {
	svc := testService()
	svc.LoadConfig()
	svc.buildPackets() // Ensure packets exist

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	// Send query: type 0x03, length 8, with key bytes at [4] and [5]
	_, err := clientConn.Write([]byte{0x10, 0x03, 0x00, 0x00, 0xAA, 0xBB, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}

	// Read response
	clientConn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1024)
	n, err := clientConn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	if n < 8 {
		t.Errorf("response too short: %d bytes", n)
	}
	if buf[4] != 0xAA || buf[5] != 0xBB {
		t.Errorf("key bytes = [%#x, %#x], want [0xAA, 0xBB]", buf[4], buf[5])
	}

	serverConn.Close()
	<-errCh
}

func TestHandleUDP_Query5Bytes(t *testing.T) {
	svc := testService()
	svc.LoadConfig()
	svc.buildPackets()

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	// 5-byte query is also valid
	_, err := clientConn.Write([]byte{0x10, 0x03, 0x00, 0x00, 0xCC})
	if err != nil {
		t.Fatal(err)
	}

	clientConn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 1024)
	_, err = clientConn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	serverConn.Close()
	<-errCh

	_, client, found := svc.LastClient()
	if !found {
		t.Fatal("expected a client")
	}
	if client.Queries.Load() != 1 {
		t.Errorf("Queries = %d, want 1", client.Queries.Load())
	}
}

func TestHandleUDP_InvalidType(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	_, err := clientConn.Write([]byte{0x10, 0xFF, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	serverConn.Close()
	<-errCh

	if svc.InvalidPackets() != 1 {
		t.Errorf("InvalidPackets = %d, want 1", svc.InvalidPackets())
	}
}

func TestHandleUDP_TooShort(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	// 1 byte — too short (n < 2)
	_, err := clientConn.Write([]byte{0x10})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	serverConn.Close()
	<-errCh

	if svc.InvalidPackets() != 1 {
		t.Errorf("InvalidPackets = %d, want 1", svc.InvalidPackets())
	}
}

func TestHandleUDP_InvalidQueryLength(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	// Query type but 6 bytes (not 5 or 8)
	_, err := clientConn.Write([]byte{0x10, 0x03, 0x00, 0x00, 0x00, 0x00})
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(50 * time.Millisecond)
	serverConn.Close()
	<-errCh

	if svc.InvalidPackets() != 1 {
		t.Errorf("InvalidPackets = %d, want 1", svc.InvalidPackets())
	}
}

func TestHandleUDP_RateLimit(t *testing.T) {
	svc := testService()
	svc.LoadConfig()
	svc.rateLimit.Store(1) // very low limit

	serverConn, clientConn := setupUDP(t)
	defer clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	// Send multiple packets to trigger rate limiting
	for i := 0; i < 5; i++ {
		clientConn.Write([]byte{0x10, 0x05, 0x00, 0x00})
		time.Sleep(5 * time.Millisecond)
	}

	time.Sleep(50 * time.Millisecond)
	serverConn.Close()
	<-errCh

	_, client, found := svc.LastClient()
	if !found {
		t.Fatal("expected a client")
	}
	if client.RateLimited.Load() == 0 {
		t.Error("expected client to be rate limited")
	}
}

func TestHandleUDP_ConnectionClosed(t *testing.T) {
	svc := testService()
	svc.LoadConfig()

	serverConn, clientConn := setupUDP(t)
	clientConn.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- svc.handleUDP(serverConn) }()

	serverConn.Close()
	err := <-errCh
	if err != nil {
		t.Errorf("handleUDP should return nil on close, got: %v", err)
	}
}
