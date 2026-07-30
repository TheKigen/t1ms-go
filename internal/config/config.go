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

package config

import (
	"encoding/xml"
	"io"
	"os"
	"sync"
)

type MasterEntry struct {
	XMLName xml.Name `xml:"master"`
	Name    string   `xml:"name,attr"`
	Address string   `xml:",chardata"`
}

type MasterSection struct {
	XMLName               xml.Name      `xml:"master-config"`
	ListenAddress         string        `xml:"listen-address"`
	LocalAddress          string        `xml:"local-address"`
	Name                  string        `xml:"name"`
	MOTD                  string        `xml:"message-of-the-day"`
	Masters               MastersList   `xml:"masters"`
	RateLimit             uint32        `xml:"rate-limit"`
	MasterQueryTimeSeconds int64        `xml:"master-query-time-seconds"`
	GameQueryTimeSeconds  int64         `xml:"game-query-time-seconds"`
	GameExpireQueries     int64         `xml:"game-expire-queries"`
	BuildPacketsEverySecs int64         `xml:"build-packets-every-seconds"`
	MinPacketSize         int64         `xml:"min-packet-size"`
}

type MastersList struct {
	Masters []MasterEntry `xml:"master"`
}

type WebSection struct {
	XMLName             xml.Name `xml:"web-config"`
	Name                string   `xml:"name"`
	ListenAddress       string   `xml:"listen-address"`
	Debug               bool     `xml:"debug"`
	BuildPagesEverySecs int64    `xml:"build-pages-every-seconds"`
	AddServerAPIKey     string   `xml:"add-server-api-key"`
	AddServerRateLimit  uint32   `xml:"add-server-rate-limit"`
}

type Config struct {
	mu      sync.RWMutex
	XMLName xml.Name      `xml:"config"`
	Master  MasterSection `xml:"master-config"`
	Web     WebSection    `xml:"web-config"`
}

func (c *Config) RLock()   { c.mu.RLock() }
func (c *Config) RUnlock() { c.mu.RUnlock() }

func Load(filename string) (*Config, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	if err := xml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) Reload(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	var newCfg Config
	if err := xml.Unmarshal(data, &newCfg); err != nil {
		return err
	}

	c.mu.Lock()
	c.Master = newCfg.Master
	c.Web = newCfg.Web
	c.mu.Unlock()
	return nil
}

func (c *Config) Save(filename string) error {
	c.mu.RLock()
	data, err := xml.MarshalIndent(c, "", "\t")
	c.mu.RUnlock()
	if err != nil {
		return err
	}

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

func WriteDefault(filename string) *Config {
	cfg := &Config{}
	cfg.Master.ListenAddress = ":28000"
	cfg.Master.Name = "Tribes Master"
	cfg.Master.MOTD = "<jl><f3>Example MOTD<n>Have fun,<n>- Kigen"
	cfg.Master.RateLimit = 2
	cfg.Master.MasterQueryTimeSeconds = 120
	cfg.Master.GameQueryTimeSeconds = 30
	cfg.Master.GameExpireQueries = 2
	cfg.Master.BuildPacketsEverySecs = 1
	cfg.Master.MinPacketSize = 0
	cfg.Master.Masters.Masters = []MasterEntry{
		{Name: "T1M1 Tribes1.co", Address: "t1m1.tribes1.co:28000"},
		{Name: "T1M2 Tribes1.co", Address: "t1m2.tribes1.co:28000"},
		{Name: "T1M3 Tribes1.co", Address: "t1m3.tribes1.co:28000"},
		{Name: "T1M1 PU.net", Address: "t1m1.pu.net:28000"},
		{Name: "T1M2 PU.net", Address: "t1m2.pu.net:28000"},
		{Name: "T1M3 PU.net", Address: "t1m3.pu.net:28000"},
	}

	cfg.Web.ListenAddress = "127.0.0.1:8080"
	cfg.Web.Name = "Tribes Master"
	cfg.Web.Debug = false
	cfg.Web.BuildPagesEverySecs = 1
	cfg.Web.AddServerRateLimit = 10

	f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return cfg
	}
	defer f.Close()

	data, err := xml.MarshalIndent(cfg, "", "\t")
	if err != nil {
		return cfg
	}
	f.Write(data)
	return cfg
}
