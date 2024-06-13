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

package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/TheKigen/t1net-go"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type Validation uint8

const (
	NotValidated Validation = 0
	Validated    Validation = 1
	Expired      Validation = 2
)

type GameServer struct {
	Mutex     sync.RWMutex
	IP        net.IP
	Port      uint16
	Validated Validation
	Querying  atomic.Bool
	Data      *t1net.GameServer
	FirstSeen int64
	LastSeen  int64
	LastPing  int64
	LastPong  int64
	LastError error
}

type ValidatedServer struct {
	ip   net.IP
	port uint16
}

type MasterServer struct {
	Mutex        sync.RWMutex
	Name         string
	Address      string
	ResolvedAddr string
	Querying     atomic.Bool
	Data         *t1net.MasterServer
	LastQuery    int64
	LastReply    int64
	LastError    error
}

type Client struct {
	Rate        atomic.Uint64
	RateLimited atomic.Uint64
	Invalids    atomic.Uint64
	Queries     atomic.Uint64
	Heartbeats  atomic.Uint64
	LastSeen    atomic.Int64
}

var masterMutex sync.RWMutex
var packets [][]byte
var masterWait sync.WaitGroup
var masterInvalidPackets atomic.Uint64
var gsList sync.Map     // [string, *GameServer]
var msList sync.Map     // [string, *MasterServer]
var clientList sync.Map // [string, *Client]
var rateLimit atomic.Uint32
var queryTimeSeconds atomic.Int64
var buildPacketsSecs atomic.Int64

func masterServerInit() {
}

func masterLoadConfig() {
	config.Mutex.RLock()
	masters := make([]struct {
		Name    string
		Address string
	}, 0, len(config.MasterConfig.Masters.Masters))
	for _, m := range config.MasterConfig.Masters.Masters {
		masters = append(masters, struct {
			Name    string
			Address string
		}{Name: m.Name, Address: m.Address})
	}
	if config.MasterConfig.RateLimit < 1 {
		rateLimit.Store(2)
	} else {
		rateLimit.Store(config.MasterConfig.RateLimit)
	}
	if config.MasterConfig.QueryTimeSeconds < 30 {
		queryTimeSeconds.Store(30)
	} else {
		queryTimeSeconds.Store(config.MasterConfig.QueryTimeSeconds)
	}
	if config.MasterConfig.BuildPacketsEverySecs < 1 {
		buildPacketsSecs.Store(1)
	} else {
		buildPacketsSecs.Store(config.MasterConfig.BuildPacketsEverySecs)
	}
	config.Mutex.RUnlock()
	msList.Range(func(keyAny any, value any) bool {
		key := keyAny.(string)
		for _, m := range masters {
			if key == m.Address {
				return true
			}
		}
		msList.Delete(key)
		logger.Printf("Removed master server %s.", key)
		return true
	})

	for _, m := range masters {
		addMasterServer(m.Name, m.Address)
	}
}

func masterServerLoop() {
	masterWait.Add(1)

	masterLoadConfig()

	go func() {
		for {
			buildPackets()
			time.Sleep(time.Duration(buildPacketsSecs.Load() * int64(time.Second)))
		}
	}()
	go masterListener()
	go masterWatcher()
}

func buildPackets() {
	// Max Packet size is 1024
	var writer bytes.Buffer
	var newPackets [][]byte
	var packetCount uint8 = 0
	var validatedServers []ValidatedServer
	var validatedCount uint16 = 0
	var serverPos uint16 = 0

	gsList.Range(func(key any, valueAny any) bool {
		value := valueAny.(*GameServer)
		value.Mutex.RLock()
		defer value.Mutex.RUnlock()
		if value.Validated == Validated {
			validatedCount += 1
			validatedServers = append(validatedServers, ValidatedServer{ip: value.IP, port: value.Port})
		}
		return true
	})

	for {
		packetCount += 1
		writer.Reset()
		writer.Write([]byte{0x10, 0x06, packetCount, packetCount, 0, 0, 0, 0x66})
		config.Mutex.RLock()
		err := t1net.WritePascalString(&writer, config.MasterConfig.Name)
		config.Mutex.RUnlock()
		if err != nil {
			logger.Fatal(err)
		}
		config.Mutex.RLock()
		err = t1net.WritePascalString(&writer, config.MasterConfig.MOTD)
		config.Mutex.RUnlock()
		if err != nil {
			logger.Fatal(err)
		}
		var shouldBreak = true
		var origPos = serverPos
		var maxServers = uint16((1024 - writer.Len()) / 7)
		var serversToWrite = validatedCount - origPos
		if serversToWrite > maxServers {
			serversToWrite = maxServers
			shouldBreak = false
		}
		err = binary.Write(&writer, binary.BigEndian, serversToWrite)
		if err != nil {
			logger.Fatal(err)
		}

		for serverPos-origPos < serversToWrite {
			server := &validatedServers[serverPos]
			serverPos += 1
			err = t1net.WriteAddressPort(&writer, server.ip, server.port)
			if err != nil {
				logger.Fatal(err)
			}
		}

		newPackets = append(newPackets, writer.Bytes())

		if shouldBreak {
			break
		}
	}

	if packetCount > 1 {
		for i := range newPackets {
			newPackets[i][3] = packetCount
		}
	}

	masterMutex.Lock()
	packets = newPackets
	masterMutex.Unlock()
}

func masterListener() {
	config.Mutex.RLock()
	s, err := net.ResolveUDPAddr("udp4", config.MasterConfig.ListenAddress)
	config.Mutex.RUnlock()
	if err != nil {
		logger.Fatal(err)
	}

	conn, err := net.ListenUDP("udp4", s)
	if err != nil {
		logger.Fatal(err)
	}

	defer func(conn *net.UDPConn) {
		err := conn.Close()
		if err != nil {
			logger.Fatal(err)
		}
	}(conn)

	buffer := make([]byte, 1024)
	reader := bytes.NewReader(buffer)

	go func() {
		for {
			var client *Client
			n, addr, err := conn.ReadFromUDP(buffer)
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				logger.Fatal(err)
			}

			clientAny, ok := clientList.Load(addr.IP.String())
			if !ok {
				client = &Client{}
				clientList.Store(addr.IP.String(), client)
			} else {
				client = clientAny.(*Client)
			}

			client.LastSeen.Store(time.Now().UTC().Unix())
			client.Rate.Add(1)

			if client.Rate.Load() > uint64(rateLimit.Load()) {
				if client.RateLimited.Load() == 0 {
					logger.Printf("Rate limiting %s.", addr.String())
				}
				client.RateLimited.Add(1)
				// Penalty
				client.Rate.Add(1)
				continue
			}

			if n < 2 || n > 1020 {
				client.Invalids.Add(1)
				masterInvalidPackets.Add(1)
				continue
			}

			_, err = reader.Seek(0, io.SeekStart)
			if err != nil {
				logger.Fatal(err)
			}

			switch buffer[1] {
			case 0x03:
				// Tribes Client Server List Request
				if n != 8 && n != 5 {
					client.Invalids.Add(1)
					masterInvalidPackets.Add(1)
					continue
				}

				client.Queries.Add(1)
				logger.Printf("Query from %s.", addr.String())
				masterMutex.RLock()
				packetsToSend := make([][]byte, len(packets))
				for i := range packets {
					packetsToSend[i] = make([]byte, len(packets[i]))
					copy(packetsToSend[i], packets[i])
				}
				masterMutex.RUnlock()
				for _, packet := range packetsToSend {
					packet[4] = buffer[4]
					packet[5] = buffer[5]
					_, err := conn.WriteToUDP(packet, addr)
					if err != nil {
						logger.Println(err)
					}
				}
			case 0x05:
				// Tribes Server Heartbeat
				logger.Printf("Heartbeat from %s.", addr.String())
				client.Heartbeats.Add(1)
				addGameServer(addr.String())
			default:
				client.Invalids.Add(1)
				masterInvalidPackets.Add(1)
			}
		}
	}()

	masterWait.Wait()
}

func masterWatcher() {
	var exit atomic.Bool
	exit.Store(false)

	go func() {
		var now int64

		for !exit.Load() {
			time.Sleep(time.Second)
			now = time.Now().UTC().Unix()
			msList.Range(func(key any, valueAny any) bool {
				value := valueAny.(*MasterServer)
				value.Mutex.Lock()
				defer value.Mutex.Unlock()
				if now > value.LastQuery+queryTimeSeconds.Load() && !value.Querying.Load() {
					value.LastQuery = now
					value.Querying.Store(true)
					logger.Printf("Query master server %s...", value.Address)
					go func(master *MasterServer) {
						defer master.Querying.Store(false)
						config.Mutex.RLock()
						localAddress := config.MasterConfig.LocalAddress
						config.Mutex.RUnlock()
						err := master.Data.Query(3*time.Second, localAddress)
						master.Mutex.Lock()
						defer master.Mutex.Unlock()
						master.LastError = err
						if err != nil {
							logger.Println(err)
							return
						}
						logger.Printf("Master server %s sent %d servers.", master.Address, master.Data.ServerCount())
						master.LastReply = time.Now().UTC().Unix()
						for _, v := range master.Data.Servers() {
							addGameServer(v)
						}
					}(value)
				}
				return true
			})
			gsList.Range(func(key any, valueAny any) bool {
				value := valueAny.(*GameServer)
				value.Mutex.Lock()
				defer value.Mutex.Unlock()
				if now > value.LastPing+queryTimeSeconds.Load() && !value.Querying.Load() {
					value.LastPing = now
					value.Querying.Store(true)
					go func(game *GameServer) {
						defer game.Querying.Store(false)
						config.Mutex.RLock()
						localAddress := config.MasterConfig.LocalAddress
						config.Mutex.RUnlock()
						err := game.Data.Query(3*time.Second, localAddress)
						game.Mutex.Lock()
						defer game.Mutex.Unlock()
						game.LastError = err
						if err != nil {
							// logger.Println(err)
							return
						}
						if game.Validated != Validated {
							logger.Printf("Validated game server %s:%d.", game.IP.String(), game.Port)
						}
						game.LastPong = time.Now().UTC().Unix()
						game.LastSeen = game.LastPong
						game.Validated = Validated
					}(value)
				}
				if value.Validated == Validated && now > value.LastPong+(queryTimeSeconds.Load()*2) {
					value.Validated = Expired
					logger.Printf("Expired game server %s:%d.", value.IP.String(), value.Port)
				}
				if now > value.LastSeen+(queryTimeSeconds.Load()*10) {
					gsList.Delete(key)
					logger.Printf("Deleted game server %s:%d.", value.IP.String(), value.Port)
				}
				return true
			})
			clientList.Range(func(key any, valueAny any) bool {
				value := valueAny.(*Client)
				if now > value.LastSeen.Load()+86400 {
					clientList.Delete(key)
					return true
				}
				if value.Rate.Load() > 0 {
					value.Rate.Add(^uint64(0))
				}
				return true
			})
		}
	}()

	masterWait.Wait()
	exit.Store(true)
	for {
		time.Sleep(time.Second)
		if !exit.Load() {
			return
		}
	}
}

func addGameServer(address string) {
	if valAny, ok := gsList.Load(address); ok {
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

	duplicateCount := 0
	gsList.Range(func(key any, value any) bool {
		if value.(*GameServer).IP.Equal(addr.IP) {
			duplicateCount += 1
			if duplicateCount > 20 {
				return false
			}
		}
		return true
	})

	if duplicateCount > 20 {
		logger.Printf("Rejected %s due to more than 20 duplicates.", addr.String())
		return
	}

	now := time.Now().UTC().Unix()

	gsList.Store(address, &GameServer{
		IP:        addr.IP.To4(),
		Port:      uint16(addr.Port),
		Data:      t1net.NewGameServer(address),
		FirstSeen: now,
		LastSeen:  now,
	})
	logger.Printf("Added game server %s.", address)
}

func addMasterServer(name string, address string) {
	if _, ok := msList.Load(address); ok {
		return
	}

	msList.Store(address, &MasterServer{Name: name, Address: address, Data: t1net.NewMasterServer(address)})
	logger.Printf("Added master server %s.", address)
}
