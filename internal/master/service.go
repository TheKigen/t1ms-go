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

package master

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/TheKigen/t1ms-go/internal/config"
	"github.com/TheKigen/t1net-go"
)

type Service struct {
	logger *log.Logger
	config *config.Config

	packetsMu sync.RWMutex
	packets   [][]byte

	gsAddMu    sync.Mutex
	gsList     sync.Map // string -> *GameServer
	msList     sync.Map // string -> *MasterServer
	clientList sync.Map // string -> *Client

	invalidPackets      atomic.Uint64
	rateLimit           atomic.Uint32
	masterQueryTimeSecs atomic.Int64
	gameQueryTimeSecs   atomic.Int64
	gameExpireQueries   atomic.Int64
	buildPktSecs        atomic.Int64
	minPacketSize       atomic.Int64
}

func NewService(logger *log.Logger, cfg *config.Config) *Service {
	return &Service{
		logger: logger,
		config: cfg,
	}
}

func (s *Service) LoadConfig() {
	s.config.RLock()
	masters := make([]config.MasterEntry, len(s.config.Master.Masters.Masters))
	copy(masters, s.config.Master.Masters.Masters)
	s.rateLimit.Store(clampUint32(s.config.Master.RateLimit, 1, 2))
	s.masterQueryTimeSecs.Store(clampInt64Range(s.config.Master.MasterQueryTimeSeconds, 60, 600, 120))
	s.gameQueryTimeSecs.Store(clampInt64(s.config.Master.GameQueryTimeSeconds, 10, 30))
	s.gameExpireQueries.Store(clampInt64(s.config.Master.GameExpireQueries, 1, 2))
	s.buildPktSecs.Store(clampInt64(s.config.Master.BuildPacketsEverySecs, 1, 1))
	s.minPacketSize.Store(s.config.Master.MinPacketSize)
	s.config.RUnlock()

	s.msList.Range(func(k, _ any) bool {
		key := k.(string)
		for _, m := range masters {
			if key == m.Address {
				return true
			}
		}
		s.msList.Delete(key)
		s.logger.Printf("Removed master server %s.", key)
		return true
	})

	for _, m := range masters {
		s.addMasterServer(m.Name, m.Address)
	}
}

func clampUint32(val, min, def uint32) uint32 {
	if val < min {
		return def
	}
	return val
}

func clampInt64(val, min, def int64) int64 {
	if val < min {
		return def
	}
	return val
}

func clampInt64Range(val, min, max, def int64) int64 {
	if val < min || val > max {
		return def
	}
	return val
}

func (s *Service) Run(ctx context.Context) error {
	s.LoadConfig()
	go s.buildPacketsLoop(ctx)
	go s.watch(ctx)
	return s.listen(ctx)
}

func (s *Service) InvalidPackets() uint64 {
	return s.invalidPackets.Load()
}

func (s *Service) GameQueryTimeSecs() int64 {
	return s.gameQueryTimeSecs.Load()
}

func (s *Service) MasterQueryTimeSecs() int64 {
	return s.masterQueryTimeSecs.Load()
}

func (s *Service) ServerCounts() (verified, total int) {
	s.gsList.Range(func(_, valueAny any) bool {
		total++
		gs := valueAny.(*GameServer)
		gs.Mutex.RLock()
		defer gs.Mutex.RUnlock()
		if gs.Validated == Validated {
			verified++
		}
		return true
	})
	return
}

func (s *Service) LastClient() (string, *Client, bool) {
	var lastClient *Client
	var lastIP string
	s.clientList.Range(func(keyAny, valueAny any) bool {
		ip := keyAny.(string)
		c := valueAny.(*Client)
		if lastClient == nil || lastClient.LastSeen.Load() < c.LastSeen.Load() {
			lastClient = c
			lastIP = ip
		}
		return true
	})
	if lastClient == nil {
		return "", nil, false
	}
	return lastIP, lastClient, true
}

func (s *Service) RangeGameServers(fn func(string, *GameServer) bool) {
	s.gsList.Range(func(k, v any) bool {
		return fn(k.(string), v.(*GameServer))
	})
}

func (s *Service) RangeMasterServers(fn func(string, *MasterServer) bool) {
	s.msList.Range(func(k, v any) bool {
		return fn(k.(string), v.(*MasterServer))
	})
}

func (s *Service) RangeClients(fn func(string, *Client) bool) {
	s.clientList.Range(func(k, v any) bool {
		return fn(k.(string), v.(*Client))
	})
}

func (s *Service) AddGameServer(address string) {
	if valAny, ok := s.gsList.Load(address); ok {
		val := valAny.(*GameServer)
		val.Mutex.Lock()
		defer val.Mutex.Unlock()
		val.LastSeen = time.Now().UTC().Unix()
		return
	}

	addr, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return
	}

	s.gsAddMu.Lock()
	defer s.gsAddMu.Unlock()

	if valAny, ok := s.gsList.Load(address); ok {
		val := valAny.(*GameServer)
		val.Mutex.Lock()
		defer val.Mutex.Unlock()
		val.LastSeen = time.Now().UTC().Unix()
		return
	}

	duplicateCount := 0
	s.gsList.Range(func(_, value any) bool {
		if value.(*GameServer).IP.Equal(addr.IP) {
			duplicateCount++
			if duplicateCount > 20 {
				return false
			}
		}
		return true
	})

	if duplicateCount > 20 {
		s.logger.Printf("Rejected %s due to more than 20 duplicates.", addr.String())
		return
	}

	now := time.Now().UTC().Unix()
	s.gsList.Store(address, &GameServer{
		IP:        addr.IP.To4(),
		Port:      uint16(addr.Port),
		Data:      nil, // Will be populated by queryGameServer
		FirstSeen: now,
		LastSeen:  now,
	})
	s.logger.Printf("Added game server %s.", address)
}

func (s *Service) addMasterServer(name, address string) {
	if _, ok := s.msList.Load(address); ok {
		return
	}
	s.msList.Store(address, &MasterServer{
		Name:    name,
		Address: address,
		Data:    nil, // Will be populated by queryMaster
	})
	s.logger.Printf("Added master server %s.", address)
}

func (s *Service) buildPacketsLoop(ctx context.Context) {
	for {
		s.buildPackets()
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Duration(s.buildPktSecs.Load()) * time.Second):
		}
	}
}

func (s *Service) buildPackets() {
	var buf bytes.Buffer
	var newPackets [][]byte
	var packetCount uint8
	var servers []validatedServer
	var validatedCount uint16
	var serverPos uint16

	s.gsList.Range(func(_, valueAny any) bool {
		gs := valueAny.(*GameServer)
		gs.Mutex.RLock()
		defer gs.Mutex.RUnlock()
		if gs.Validated == Validated {
			validatedCount++
			servers = append(servers, validatedServer{ip: gs.IP, port: gs.Port})
		}
		return true
	})

	s.config.RLock()
	name := s.config.Master.Name
	motd := s.config.Master.MOTD
	s.config.RUnlock()

	for {
		packetCount++
		buf.Reset()
		buf.Write([]byte{0x10, 0x06, packetCount, packetCount, 0, 0, 0, 0x66})

		if err := t1net.WritePascalString(&buf, name); err != nil {
			s.logger.Printf("buildPackets: write name: %v", err)
			return
		}
		if err := t1net.WritePascalString(&buf, motd); err != nil {
			s.logger.Printf("buildPackets: write MOTD: %v", err)
			return
		}

		origPos := serverPos
		maxServers := uint16((1024 - buf.Len()) / 7)
		serversToWrite := validatedCount - origPos
		shouldBreak := true
		if serversToWrite > maxServers {
			serversToWrite = maxServers
			shouldBreak = false
		}

		if err := binary.Write(&buf, binary.BigEndian, serversToWrite); err != nil {
			s.logger.Printf("buildPackets: write count: %v", err)
			return
		}

		for serverPos-origPos < serversToWrite {
			srv := &servers[serverPos]
			serverPos++
			if err := t1net.WriteAddressPort(&buf, srv.ip, srv.port); err != nil {
				s.logger.Printf("buildPackets: write server: %v", err)
				return
			}
		}

		// Copy buffer bytes — buf.Bytes() returns a reference invalidated by Reset
		pkt := make([]byte, buf.Len())
		copy(pkt, buf.Bytes())
		pkt = t1net.PadPacket(pkt, int(s.minPacketSize.Load()))
		newPackets = append(newPackets, pkt)

		if shouldBreak {
			break
		}
	}

	if packetCount > 1 {
		for i := range newPackets {
			newPackets[i][3] = packetCount
		}
	}

	s.packetsMu.Lock()
	s.packets = newPackets
	s.packetsMu.Unlock()
}

func (s *Service) listen(ctx context.Context) error {
	s.config.RLock()
	listenAddr := s.config.Master.ListenAddress
	s.config.RUnlock()

	udpAddr, err := net.ResolveUDPAddr("udp4", listenAddr)
	if err != nil {
		return fmt.Errorf("resolve listen address: %w", err)
	}

	conn, err := net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Close the connection when context is cancelled for clean shutdown
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	s.logger.Printf("Master listening on %s.", listenAddr)
	return s.handleUDP(conn)
}

func (s *Service) handleUDP(conn *net.UDPConn) error {
	buffer := make([]byte, 1024)
	for {
		n, addr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("read UDP: %w", err)
		}

		clientAny, _ := s.clientList.LoadOrStore(addr.IP.String(), &Client{})
		client := clientAny.(*Client)

		client.LastSeen.Store(time.Now().UTC().Unix())
		client.Rate.Add(1)

		if client.Rate.Load() > uint64(s.rateLimit.Load()) {
			if client.RateLimited.Load() == 0 {
				s.logger.Printf("Rate limiting %s.", addr.String())
			}
			client.RateLimited.Add(1)
			client.Rate.Add(1) // Penalty
			continue
		}

		if n < 2 || n > 1020 {
			client.Invalids.Add(1)
			s.invalidPackets.Add(1)
			continue
		}

		switch buffer[1] {
		case 0x03: // Tribes Client Server List Request
			if n != 8 && n != 5 {
				client.Invalids.Add(1)
				s.invalidPackets.Add(1)
				continue
			}

			client.Queries.Add(1)
			s.logger.Printf("Query from %s.", addr.String())

			s.packetsMu.RLock()
			packetsToSend := make([][]byte, len(s.packets))
			for i := range s.packets {
				packetsToSend[i] = make([]byte, len(s.packets[i]))
				copy(packetsToSend[i], s.packets[i])
			}
			s.packetsMu.RUnlock()

			for _, pkt := range packetsToSend {
				pkt = t1net.PadPacket(pkt, int(s.minPacketSize.Load()))
				pkt[4] = buffer[4]
				pkt[5] = buffer[5]
				if _, err := conn.WriteToUDP(pkt, addr); err != nil {
					s.logger.Println(err)
				}
			}

		case 0x05: // Tribes Server Heartbeat
			s.logger.Printf("Heartbeat from %s.", addr.String())
			client.Heartbeats.Add(1)
			s.AddGameServer(addr.String())

		default:
			client.Invalids.Add(1)
			s.invalidPackets.Add(1)
		}
	}
}

func (s *Service) watch(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		s.watchTick(time.Now().UTC().Unix())
	}
}

func (s *Service) watchTick(now int64) {
	masterQueryTime := s.masterQueryTimeSecs.Load()
	gameQueryTime := s.gameQueryTimeSecs.Load()
	gameExpireQueries := s.gameExpireQueries.Load()

	s.msList.Range(func(_, valueAny any) bool {
		ms := valueAny.(*MasterServer)
		ms.Mutex.Lock()
		defer ms.Mutex.Unlock()
		if now > ms.LastQuery+masterQueryTime && !ms.Querying.Load() {
			ms.LastQuery = now
			ms.Querying.Store(true)
			s.logger.Printf("Query master server %s...", ms.Address)
			go s.queryMaster(ms)
		}
		return true
	})

	s.gsList.Range(func(key, valueAny any) bool {
		gs := valueAny.(*GameServer)
		gs.Mutex.Lock()
		defer gs.Mutex.Unlock()
		if now > gs.LastPing+gameQueryTime && !gs.Querying.Load() {
			gs.LastPing = now
			gs.Querying.Store(true)
			go s.queryGameServer(gs)
		}
		if gs.Validated == Validated && now > gs.LastPong+(gameQueryTime*gameExpireQueries) {
			gs.Validated = Expired
			s.logger.Printf("Expired game server %s:%d.", gs.IP.String(), gs.Port)
		}
		if now > gs.LastSeen+(gameQueryTime*10) {
			s.gsList.Delete(key)
			s.logger.Printf("Deleted game server %s:%d.", gs.IP.String(), gs.Port)
		}
		return true
	})

	s.clientList.Range(func(key, valueAny any) bool {
		c := valueAny.(*Client)
		if now > c.LastSeen.Load()+86400 {
			s.clientList.Delete(key)
			return true
		}
		if c.Rate.Load() > 0 {
			c.Rate.Add(^uint64(0))
		}
		return true
	})
}

func (s *Service) queryMaster(ms *MasterServer) {
	defer ms.Querying.Store(false)

	result, err := t1net.MasterQuery(ms.Address, &t1net.QueryOptions{
		Timeout: 3 * time.Second,
	})

	ms.Mutex.Lock()
	defer ms.Mutex.Unlock()
	ms.LastError = err
	if err != nil {
		s.logger.Println(err)
		return
	}

	ms.Data = result
	s.logger.Printf("Master server %s sent %d servers.", ms.Address, result.ServerCount)
	ms.LastReply = time.Now().UTC().Unix()
	for _, v := range result.Servers {
		s.AddGameServer(v)
	}
}

func (s *Service) queryGameServer(gs *GameServer) {
	defer gs.Querying.Store(false)

	s.config.RLock()
	localAddr := s.config.Master.LocalAddress
	s.config.RUnlock()

	result, err := t1net.GameInfoQuery(fmt.Sprintf("%s:%d", gs.IP.String(), gs.Port), &t1net.QueryOptions{
		Timeout:      3 * time.Second,
		LocalAddress: localAddr,
	})

	gs.Mutex.Lock()
	defer gs.Mutex.Unlock()
	gs.LastError = err
	if err != nil {
		return
	}

	gs.Data = result
	if gs.Validated != Validated {
		s.logger.Printf("Validated game server %s:%d.", gs.IP.String(), gs.Port)
	}
	gs.LastPong = time.Now().UTC().Unix()
	gs.LastSeen = gs.LastPong
	gs.Validated = Validated
}
