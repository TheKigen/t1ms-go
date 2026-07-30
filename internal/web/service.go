/*
   Copyright 2022 Max Krivanek

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package web

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheKigen/t1ms-go/internal/config"
	"github.com/TheKigen/t1ms-go/internal/master"
)

const localAddressPlaceholder = "__T1MS_LOCAL_HOST__"

var cgnatRange = netip.MustParsePrefix("100.64.0.0/10")

func isNonRoutableIP(addr netip.Addr) bool {
	return addr.IsLoopback() || addr.IsMulticast() || addr.IsUnspecified() ||
		addr.IsPrivate() || addr.IsLinkLocalUnicast() || cgnatRange.Contains(addr)
}

//go:embed public
var content embed.FS

type Service struct {
	logger *log.Logger
	config *config.Config
	master *master.Service

	mu             sync.RWMutex
	listenAddress  string
	buildPagesSecs atomic.Int64

	addServerAPIKey   atomic.Value // string
	addServerRateMap  sync.Map     // string -> *atomic.Uint32
	addServerRateMax  atomic.Uint32

	masterPageXML  []byte
	serverPageXML  []byte
	statsPageXML   []byte
	masterPageJSON []byte
	serverPageJSON []byte
	statsPageJSON  []byte
}

func NewService(logger *log.Logger, cfg *config.Config, masterSvc *master.Service) *Service {
	return &Service{
		logger: logger,
		config: cfg,
		master: masterSvc,
	}
}

func (s *Service) LoadConfig() {
	s.config.RLock()
	defer s.config.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()

	s.listenAddress = s.config.Web.ListenAddress
	secs := s.config.Web.BuildPagesEverySecs
	if secs < 1 {
		secs = 1
	}
	s.buildPagesSecs.Store(secs)
	s.addServerAPIKey.Store(s.config.Web.AddServerAPIKey)
	rateLimit := s.config.Web.AddServerRateLimit
	if rateLimit < 1 {
		rateLimit = 10
	}
	s.addServerRateMax.Store(rateLimit)
}

func (s *Service) Run(ctx context.Context) error {
	s.LoadConfig()

	s.mu.RLock()
	addr := s.listenAddress
	s.mu.RUnlock()

	if addr == "" {
		s.logger.Println("No web address configured.")
		return nil
	}

	go s.buildPagesLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/masters.xml", s.handleAPI)
	mux.HandleFunc("/api/v1/servers.xml", s.handleAPI)
	mux.HandleFunc("/api/v1/stats.xml", s.handleAPI)
	mux.HandleFunc("/api/v1/masters.json", s.handleAPI)
	mux.HandleFunc("/api/v1/servers.json", s.handleAPI)
	mux.HandleFunc("/api/v1/stats.json", s.handleAPI)
	mux.HandleFunc("/api/v1/addserver", s.handleAddServer)
	mux.Handle("/", http.FileServer(s.embedRoot()))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 16, // 64 KB
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	s.logger.Printf("Web listening on %s.", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("web server: %w", err)
	}
	return nil
}

func (s *Service) embedRoot() http.FileSystem {
	publicFS, err := fs.Sub(content, "public")
	if err != nil {
		s.logger.Fatalf("embed public: %v", err)
	}
	return http.FS(publicFS)
}

func (s *Service) buildPagesLoop(ctx context.Context) {
	rateTicker := time.NewTicker(time.Second)
	defer rateTicker.Stop()

	for {
		s.buildPages()
		select {
		case <-ctx.Done():
			return
		case <-rateTicker.C:
			s.decayAddServerRates()
		case <-time.After(time.Duration(s.buildPagesSecs.Load()) * time.Second):
		}
	}
}

func (s *Service) decayAddServerRates() {
	s.addServerRateMap.Range(func(key, value any) bool {
		counter := value.(*atomic.Uint32)
		cur := counter.Load()
		if cur == 0 {
			s.addServerRateMap.Delete(key)
		} else {
			counter.Add(^uint32(0)) // decrement by 1
		}
		return true
	})
}

func (s *Service) buildPages() {
	stats := Stats{
		RefreshInterval: s.master.GameQueryTimeSecs(),
	}

	s.master.RangeClients(func(_ string, c *master.Client) bool {
		if c.Queries.Load() != 0 {
			stats.UniqueClients++
		}
		return true
	})

	s.config.RLock()
	localName := s.config.Master.Name
	localMOTD := s.config.Master.MOTD
	masterListenAddr := s.config.Master.ListenAddress
	s.config.RUnlock()

	_, masterPort, _ := net.SplitHostPort(masterListenAddr)
	localAddr := localAddressPlaceholder + ":" + masterPort

	var localServers []string
	s.master.RangeGameServers(func(addr string, gs *master.GameServer) bool {
		gs.Mutex.RLock()
		defer gs.Mutex.RUnlock()
		if gs.Validated == master.Validated {
			localServers = append(localServers, addr)
		}
		return true
	})

	msResp := MastersResponse{
		RefreshInterval: s.master.MasterQueryTimeSecs(),
		Masters: []Master{{
			Name:        localName,
			MOTD:        localMOTD,
			Address:     localAddr,
			ServerCount: uint16(len(localServers)),
			Server:      localServers,
			Local:       true,
		}},
	}

	s.master.RangeMasterServers(func(_ string, ms *master.MasterServer) bool {
		ms.Mutex.RLock()
		defer ms.Mutex.RUnlock()
		if ms.Data == nil {
			return true
		}
		msResp.Masters = append(msResp.Masters, Master{
			Name:        ms.Name,
			MOTD:        ms.Data.MOTD,
			Address:     ms.Address,
			Ping:        ms.Data.Ping.Milliseconds(),
			LastReply:   time.Unix(ms.LastReply, 0).UTC().String(),
			ServerCount: uint16(ms.Data.ServerCount),
			Server:      ms.Data.Servers,
		})
		return true
	})

	gsResp := ServersResponse{
		Name: localName,
		MOTD: localMOTD,
	}

	s.master.RangeGameServers(func(_ string, gs *master.GameServer) bool {
		gs.Mutex.RLock()
		defer gs.Mutex.RUnlock()
		if gs.Validated != master.Validated {
			return true
		}
		stats.TotalServers++
		if gs.Data == nil {
			return true
		}
		stats.TotalMaxPlayers += int(gs.Data.MaxPlayers)

		var players []Player
		var teams []Team
		for i, v := range gs.Data.Teams {
			teams = append(teams, Team{
				ID:    uint8(i),
				Name:  v.Name,
				Score: v.Score,
			})
		}
		for _, v := range gs.Data.Players {
			players = append(players, Player{
				Name:  v.Name,
				Score: v.Score,
				Team:  v.Team,
				PL:    v.PL,
				Ping:  v.Ping,
			})
			stats.TotalPlayers++
		}

		gsResp.Servers = append(gsResp.Servers, Server{
			Address:           fmt.Sprintf("%s:%d", gs.IP.String(), gs.Port),
			Ping:              gs.Data.Ping.Milliseconds(),
			FirstSeen:         time.Unix(gs.FirstSeen, 0).UTC().String(),
			LastSeen:          time.Unix(gs.LastSeen, 0).UTC().String(),
			Name:              gs.Data.Name,
			Game:              gs.Data.Game,
			Version:           gs.Data.Version,
			Dedicated:         gs.Data.Dedicated,
			Password:          gs.Data.Password,
			NumPlayers:        gs.Data.NumPlayers,
			MaxPlayers:        gs.Data.MaxPlayers,
			CPUSpeed:          uint16(gs.Data.CPUSpeed),
			Mod:               gs.Data.Mod,
			ServerType:        gs.Data.ServerType,
			Mission:           gs.Data.Mission,
			Info:              gs.Data.Info,
			NumTeams:          gs.Data.NumTeams,
			TeamScoreHeader:   gs.Data.TeamScoreHeader,
			PlayerScoreHeader: gs.Data.PlayerScoreHeader,
			Teams:             teams,
			Players:           players,
		})
		return true
	})

	gsResp.TotalServers = stats.TotalServers
	gsResp.TotalPlayers = stats.TotalPlayers
	gsResp.TotalMaxPlayers = stats.TotalMaxPlayers
	gsResp.UniqueClients = stats.UniqueClients
	gsResp.RefreshInterval = stats.RefreshInterval

	s.mu.Lock()
	defer s.mu.Unlock()

	var err error
	s.masterPageXML, err = xml.Marshal(&msResp)
	if err != nil {
		s.logger.Println(err)
	}
	s.serverPageXML, err = xml.Marshal(&gsResp)
	if err != nil {
		s.logger.Println(err)
	}
	s.statsPageXML, err = xml.Marshal(&stats)
	if err != nil {
		s.logger.Println(err)
	}
	s.masterPageJSON, err = json.Marshal(&msResp)
	if err != nil {
		s.logger.Println(err)
	}
	s.serverPageJSON, err = json.Marshal(&gsResp)
	if err != nil {
		s.logger.Println(err)
	}
	s.statsPageJSON, err = json.Marshal(&stats)
	if err != nil {
		s.logger.Println(err)
	}
}

func (s *Service) handleAPI(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var data []byte
	var contentType string

	switch req.URL.Path {
	case "/api/v1/masters.xml":
		data, contentType = s.masterPageXML, "application/xml; charset=utf-8"
	case "/api/v1/servers.xml":
		data, contentType = s.serverPageXML, "application/xml; charset=utf-8"
	case "/api/v1/stats.xml":
		data, contentType = s.statsPageXML, "application/xml; charset=utf-8"
	case "/api/v1/masters.json":
		data, contentType = s.masterPageJSON, "application/json; charset=utf-8"
	case "/api/v1/servers.json":
		data, contentType = s.serverPageJSON, "application/json; charset=utf-8"
	case "/api/v1/stats.json":
		data, contentType = s.statsPageJSON, "application/json; charset=utf-8"
	default:
		http.NotFound(w, req)
		return
	}

	if bytes.Contains(data, []byte(localAddressPlaceholder)) {
		host := req.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		data = bytes.ReplaceAll(data, []byte(localAddressPlaceholder), []byte(host))
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	if _, err := w.Write(data); err != nil {
		s.logger.Println(err)
	}
}

func (s *Service) handleAddServer(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// API key check
	if key, _ := s.addServerAPIKey.Load().(string); key != "" {
		provided := req.Header.Get("X-API-Key")
		if provided != key {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	// Per-IP rate limiting
	remoteIP, _, _ := net.SplitHostPort(req.RemoteAddr)
	if remoteIP == "" {
		remoteIP = req.RemoteAddr
	}
	counterAny, _ := s.addServerRateMap.LoadOrStore(remoteIP, &atomic.Uint32{})
	counter := counterAny.(*atomic.Uint32)
	if counter.Add(1) > s.addServerRateMax.Load() {
		http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
		return
	}

	address := req.URL.Query().Get("address")
	if address == "" {
		http.Error(w, "missing address parameter", http.StatusBadRequest)
		return
	}

	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		http.Error(w, "invalid address format, expected host:port", http.StatusBadRequest)
		return
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		http.Error(w, "invalid port number", http.StatusBadRequest)
		return
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		http.Error(w, "invalid IP address", http.StatusBadRequest)
		return
	}

	if isNonRoutableIP(addr) {
		http.Error(w, "address not allowed", http.StatusBadRequest)
		return
	}

	s.master.AddGameServer(address)
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}
