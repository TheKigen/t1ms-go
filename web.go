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
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// import _ "net/http/pprof"

//go:embed public
var content embed.FS
var webMutex sync.RWMutex
var webListenAddress string
var webBuildPagesSecs atomic.Int64
var masterPageXML []byte
var serverPageXML []byte
var statsPageXML []byte
var masterPageJSON []byte
var serverPageJSON []byte
var statsPageJSON []byte

type WebMaster struct {
	XMLName     xml.Name `xml:"master" json:"-"`
	Name        string   `xml:"name" json:"name"`
	MOTD        string   `xml:"message-of-the-day" json:"message-of-the-day"`
	Address     string   `xml:"address" json:"address"`
	Ping        int64    `xml:"ping" json:"ping"`
	LastReply   string   `xml:"last-reply" json:"last-reply"`
	ServerCount uint16   `xml:"server-count" json:"server-count"`
	Server      []string `xml:"server" json:"servers"`
}

type WebTeam struct {
	XMLName xml.Name `xml:"team" json:"-"`
	ID      uint8    `xml:"id,attr" json:"id"`
	Name    string   `xml:"name" json:"name"`
	Score   string   `xml:"score" json:"score"`
}

type WebPlayer struct {
	XMLName xml.Name `xml:"player" json:"-"`
	Name    string   `xml:"name" json:"name"`
	Score   string   `xml:"score" json:"score"`
	Team    uint8    `xml:"team,attr" json:"team"`
	PL      uint8    `xml:"pl" json:"pl"`
	Ping    uint8    `xml:"ping" json:"ping"`
}

type WebServer struct {
	XMLName           xml.Name    `xml:"server" json:"-"`
	Name              string      `xml:"name" json:"name"`
	Address           string      `xml:"address" json:"address"`
	Ping              int64       `xml:"ping" json:"ping"`
	FirstSeen         string      `xml:"first-seen" json:"first-seen"`
	LastSeen          string      `xml:"last-seen" json:"last-seen"`
	Game              string      `xml:"game" json:"game"`
	Version           string      `xml:"version" json:"version"`
	Dedicated         bool        `xml:"dedicated" json:"dedicated"`
	Password          bool        `xml:"password" json:"password"`
	NumPlayers        uint8       `xml:"num-players" json:"num-players"`
	MaxPlayers        uint8       `xml:"max-players" json:"max-players"`
	CPUSpeed          uint16      `xml:"cpu-speed" json:"cpu-speed"`
	Mod               string      `xml:"mod" json:"mod"`
	ServerType        string      `xml:"server-type" json:"server-type"`
	Mission           string      `xml:"mission" json:"mission"`
	Info              string      `xml:"info" json:"info"`
	NumTeams          uint8       `xml:"num-teams" json:"num-teams"`
	TeamScoreHeader   string      `xml:"team-score-header" json:"team-score-header"`
	PlayerScoreHeader string      `xml:"player-score-header" json:"player-score-header"`
	Team              []WebTeam   `xml:"team" json:"teams"`
	Player            []WebPlayer `xml:"player" json:"players"`
}

type WebStats struct {
	XMLName         xml.Name `xml:"stats" json:"-"`
	TotalServers    int      `xml:"total-servers" json:"total-servers"`
	TotalPlayers    int      `xml:"total-players" json:"total-players"`
	TotalMaxPlayers int      `xml:"total-max-players" json:"total-max-players"`
	UniqueClients   int      `xml:"unique-clients" json:"unique-clients"`
}

func webServerInit() {
}

func webServerConfig() {
	config.Mutex.RLock()
	defer config.Mutex.RUnlock()
	webMutex.Lock()
	defer webMutex.Unlock()
	webListenAddress = config.WebConfig.ListenAddress
	buildPagesSecs := config.WebConfig.BuildPagesEverySecs
	if buildPagesSecs < 1 {
		buildPagesSecs = 1
	}
	webBuildPagesSecs.Store(buildPagesSecs)
}

func embedRoot() http.FileSystem {
	publicFS, err := fs.Sub(content, "public")
	if err != nil {
		logger.Fatal(err)
	}
	return http.FS(publicFS)
}

func webServerLoop() {
	var localListenAddress string

	webServerConfig()

	webMutex.RLock()
	localListenAddress = webListenAddress
	webMutex.RUnlock()

	if len(localListenAddress) == 0 {
		logger.Println("No web address.")
		return
	}

	go func() {
		for {
			buildPages()
			time.Sleep(time.Duration(webBuildPagesSecs.Load() * int64(time.Second)))
		}
	}()

	http.HandleFunc("/api/v1/masters.xml", xmlPage)
	http.HandleFunc("/api/v1/servers.xml", xmlPage)
	http.HandleFunc("/api/v1/stats.xml", xmlPage)
	http.HandleFunc("/api/v1/masters.json", jsonPage)
	http.HandleFunc("/api/v1/servers.json", jsonPage)
	http.HandleFunc("/api/v1/stats.json", jsonPage)
	http.Handle("/", http.FileServer(embedRoot()))
	logger.Printf("Web Listening to %s.", localListenAddress)
	logger.Fatal(http.ListenAndServe(localListenAddress, nil))
}

func buildPages() {
	var err error

	stats := WebStats{
		TotalPlayers:  0,
		TotalServers:  0,
		UniqueClients: 0,
	}

	clientList.Range(func(key any, valueAny any) bool {
		value := valueAny.(*Client)
		if value.Queries.Load() != 0 {
			stats.UniqueClients++
		}
		return true
	})

	msResp := struct {
		XMLName xml.Name    `xml:"masters" json:"-"`
		Masters []WebMaster `xml:"master" json:"masters"`
	}{}

	msList.Range(func(key any, valueAny any) bool {
		value := valueAny.(*MasterServer)
		value.Mutex.RLock()
		defer value.Mutex.RUnlock()
		msResp.Masters = append(msResp.Masters, WebMaster{
			Name:        value.Name,
			MOTD:        value.Data.MOTD(),
			Address:     value.Address,
			Ping:        value.Data.Ping().Milliseconds(),
			LastReply:   time.Unix(value.LastReply, 0).UTC().String(),
			ServerCount: value.Data.ServerCount(),
			Server:      value.Data.Servers(),
		})
		return true
	})

	config.Mutex.RLock()
	gsResp := struct {
		XMLName         xml.Name    `xml:"servers" json:"-"`
		Name            string      `xml:"name" json:"name"`
		MOTD            string      `xml:"message-of-the-day" json:"message-of-the-day"`
		TotalServers    int         `xml:"total-servers" json:"total-servers"`
		TotalPlayers    int         `xml:"total-players" json:"total-players"`
		TotalMaxPlayers int         `xml:"total-max-players" json:"total-max-players"`
		UniqueClients   int         `xml:"unique-clients" json:"unique-clients"`
		Servers         []WebServer `xml:"server" json:"servers"`
	}{
		Name: config.MasterConfig.Name,
		MOTD: config.MasterConfig.MOTD,
	}
	config.Mutex.RUnlock()

	gsList.Range(func(key any, valueAny any) bool {
		value := valueAny.(*GameServer)
		value.Mutex.RLock()
		defer value.Mutex.RUnlock()
		if value.Validated != Validated {
			return true
		}
		stats.TotalServers += 1
		stats.TotalMaxPlayers += int(value.Data.MaxPlayers())
		var players []WebPlayer
		var teams []WebTeam
		for i, v := range value.Data.Teams() {
			teams = append(teams, WebTeam{
				ID:    uint8(i),
				Name:  v.Name,
				Score: v.Score,
			})
		}
		for _, v := range value.Data.Players() {
			players = append(players, WebPlayer{
				Name:  v.Name,
				Score: v.Score,
				Team:  v.Team,
				PL:    v.PL,
				Ping:  v.Ping,
			})
			stats.TotalPlayers += 1
		}
		gsResp.Servers = append(gsResp.Servers, WebServer{
			Address:           fmt.Sprintf("%s:%d", value.IP.String(), value.Port),
			Ping:              value.Data.Ping().Milliseconds(),
			FirstSeen:         time.Unix(value.FirstSeen, 0).UTC().String(),
			LastSeen:          time.Unix(value.LastSeen, 0).UTC().String(),
			Name:              value.Data.Name(),
			Game:              value.Data.Game(),
			Version:           value.Data.Version(),
			Dedicated:         value.Data.Dedicated(),
			Password:          value.Data.Password(),
			NumPlayers:        value.Data.NumPlayers(),
			MaxPlayers:        value.Data.MaxPlayers(),
			CPUSpeed:          value.Data.CPUSpeed(),
			Mod:               value.Data.Mod(),
			ServerType:        value.Data.ServerType(),
			Mission:           value.Data.Mission(),
			Info:              value.Data.Info(),
			NumTeams:          value.Data.NumTeams(),
			TeamScoreHeader:   value.Data.TeamScoreHeader(),
			PlayerScoreHeader: value.Data.PlayerScoreHeader(),
			Team:              teams,
			Player:            players,
		})
		return true
	})

	gsResp.TotalServers = stats.TotalServers
	gsResp.TotalPlayers = stats.TotalPlayers
	gsResp.TotalMaxPlayers = stats.TotalMaxPlayers
	gsResp.UniqueClients = stats.UniqueClients

	webMutex.Lock()
	defer webMutex.Unlock()

	masterPageXML, err = xml.Marshal(&msResp)
	if err != nil {
		logger.Println(err)
	}

	serverPageXML, err = xml.Marshal(&gsResp)
	if err != nil {
		logger.Println(err)
	}

	statsPageXML, err = xml.Marshal(&stats)
	if err != nil {
		logger.Println(err)
	}

	masterPageJSON, err = json.Marshal(&msResp)
	if err != nil {
		logger.Println(err)
	}

	serverPageJSON, err = json.Marshal(&gsResp)
	if err != nil {
		logger.Println(err)
	}

	statsPageJSON, err = json.Marshal(&stats)
	if err != nil {
		logger.Println(err)
	}
}

func xmlPage(w http.ResponseWriter, req *http.Request) {
	var err error

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	webMutex.RLock()
	defer webMutex.RUnlock()

	switch req.URL.Path {
	case "/api/v1/masters.xml":
		_, err = w.Write(masterPageXML)
	case "/api/v1/servers.xml":
		_, err = w.Write(serverPageXML)
	case "/api/v1/stats.xml":
		_, err = w.Write(statsPageXML)
	default:
		http.NotFound(w, req)
	}

	if err != nil {
		logger.Println(err)
		return
	}
}

func jsonPage(w http.ResponseWriter, req *http.Request) {
	var err error

	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)

	webMutex.RLock()
	defer webMutex.RUnlock()

	switch req.URL.Path {
	case "/api/v1/masters.json":
		_, err = w.Write(masterPageJSON)
	case "/api/v1/servers.json":
		_, err = w.Write(serverPageJSON)
	case "/api/v1/stats.json":
		_, err = w.Write(statsPageJSON)
	default:
		http.NotFound(w, req)
	}

	if err != nil {
		logger.Println(err)
		return
	}
}
