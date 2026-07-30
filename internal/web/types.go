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

import "encoding/xml"

type Master struct {
	XMLName     xml.Name `xml:"master" json:"-"`
	Name        string   `xml:"name" json:"name"`
	MOTD        string   `xml:"message-of-the-day" json:"message-of-the-day"`
	Address     string   `xml:"address" json:"address"`
	Ping        int64    `xml:"ping" json:"ping"`
	LastReply   string   `xml:"last-reply,omitempty" json:"last-reply,omitempty"`
	ServerCount uint16   `xml:"server-count" json:"server-count"`
	Server      []string `xml:"server,omitempty" json:"servers,omitempty"`
	Local       bool     `xml:"local" json:"local"`
}

type Team struct {
	XMLName xml.Name `xml:"team" json:"-"`
	ID      uint8    `xml:"id,attr" json:"id"`
	Name    string   `xml:"name" json:"name"`
	Score   string   `xml:"score" json:"score"`
}

type Player struct {
	XMLName xml.Name `xml:"player" json:"-"`
	Name    string   `xml:"name" json:"name"`
	Score   string   `xml:"score" json:"score"`
	Team    uint8    `xml:"team,attr" json:"team"`
	PL      uint8    `xml:"pl" json:"pl"`
	Ping    uint8    `xml:"ping" json:"ping"`
}

type Server struct {
	XMLName           xml.Name `xml:"server" json:"-"`
	Name              string   `xml:"name" json:"name"`
	Address           string   `xml:"address" json:"address"`
	Ping              int64    `xml:"ping" json:"ping"`
	FirstSeen         string   `xml:"first-seen" json:"first-seen"`
	LastSeen          string   `xml:"last-seen" json:"last-seen"`
	Game              string   `xml:"game" json:"game"`
	Version           string   `xml:"version" json:"version"`
	Dedicated         bool     `xml:"dedicated" json:"dedicated"`
	Password          bool     `xml:"password" json:"password"`
	NumPlayers        uint8    `xml:"num-players" json:"num-players"`
	MaxPlayers        uint8    `xml:"max-players" json:"max-players"`
	CPUSpeed          uint16   `xml:"cpu-speed" json:"cpu-speed"`
	Mod               string   `xml:"mod" json:"mod"`
	ServerType        string   `xml:"server-type" json:"server-type"`
	Mission           string   `xml:"mission" json:"mission"`
	Info              string   `xml:"info" json:"info"`
	NumTeams          uint8    `xml:"num-teams" json:"num-teams"`
	TeamScoreHeader   string   `xml:"team-score-header" json:"team-score-header"`
	PlayerScoreHeader string   `xml:"player-score-header" json:"player-score-header"`
	Teams             []Team   `xml:"team" json:"teams"`
	Players           []Player `xml:"player" json:"players"`
}

type Stats struct {
	XMLName         xml.Name `xml:"stats" json:"-"`
	TotalServers    int      `xml:"total-servers" json:"total-servers"`
	TotalPlayers    int      `xml:"total-players" json:"total-players"`
	TotalMaxPlayers int      `xml:"total-max-players" json:"total-max-players"`
	UniqueClients   int      `xml:"unique-clients" json:"unique-clients"`
	RefreshInterval int64    `xml:"refresh-interval" json:"refresh-interval"`
}

type MastersResponse struct {
	XMLName         xml.Name `xml:"masters" json:"-"`
	RefreshInterval int64    `xml:"refresh-interval" json:"refresh-interval"`
	Masters         []Master `xml:"master" json:"masters"`
}

type ServersResponse struct {
	XMLName         xml.Name `xml:"servers" json:"-"`
	Name            string   `xml:"name" json:"name"`
	MOTD            string   `xml:"message-of-the-day" json:"message-of-the-day"`
	TotalServers    int      `xml:"total-servers" json:"total-servers"`
	TotalPlayers    int      `xml:"total-players" json:"total-players"`
	TotalMaxPlayers int      `xml:"total-max-players" json:"total-max-players"`
	UniqueClients   int      `xml:"unique-clients" json:"unique-clients"`
	RefreshInterval int64    `xml:"refresh-interval" json:"refresh-interval"`
	Servers         []Server `xml:"server" json:"servers"`
}
