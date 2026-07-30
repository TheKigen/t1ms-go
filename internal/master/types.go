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
	"net"
	"sync"
	"sync/atomic"

	"github.com/TheKigen/t1net-go"
)

type Validation uint8

const (
	NotValidated Validation = iota
	Validated
	Expired
)

type GameServer struct {
	Mutex     sync.RWMutex
	IP        net.IP
	Port      uint16
	Validated Validation
	Querying  atomic.Bool
	Data      *t1net.GameResult
	FirstSeen int64
	LastSeen  int64
	LastPing  int64
	LastPong  int64
	LastError error
}

type validatedServer struct {
	ip   net.IP
	port uint16
}

type MasterServer struct {
	Mutex     sync.RWMutex
	Name      string
	Address   string
	Querying  atomic.Bool
	Data      *t1net.MasterResult
	LastQuery int64
	LastReply int64
	LastError error
}

type Client struct {
	Rate        atomic.Uint64
	RateLimited atomic.Uint64
	Invalids    atomic.Uint64
	Queries     atomic.Uint64
	Heartbeats  atomic.Uint64
	LastSeen    atomic.Int64
}
