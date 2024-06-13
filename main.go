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
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type MasterConfig struct {
	XMLName xml.Name `xml:"master"`
	Name    string   `xml:"name,attr"`
	Address string   `xml:",chardata"`
}

type Config struct {
	Mutex        sync.RWMutex `xml:"-"`
	XMLName      xml.Name     `xml:"config"`
	MasterConfig struct {
		XMLName       xml.Name `xml:"master-config"`
		ListenAddress string   `xml:"listen-address"`
		LocalAddress  string   `xml:"local-address"`
		Name          string   `xml:"name"`
		MOTD          string   `xml:"message-of-the-day"`
		Masters       struct {
			Masters []MasterConfig `xml:"master"`
		} `xml:"masters"`
		RateLimit             uint32 `xml:"rate-limit"`
		QueryTimeSeconds      int64  `xml:"query-time-seconds"`
		BuildPacketsEverySecs int64  `xml:"build-packets-every-seconds"`
	} `xml:"master-config"`
	WebConfig struct {
		XMLName             xml.Name `xml:"web-config"`
		Name                string   `xml:"name"`
		ListenAddress       string   `xml:"listen-address"`
		Debug               bool     `xml:"debug"`
		BuildPagesEverySecs int64    `xml:"build-pages-every-seconds"`
	} `xml:"web-config"`
}

var logger *log.Logger
var config Config

func main() {
	var configFile string
	isService := false
	flag.StringVar(&configFile, "c", "config.xml", "Specify config file path.")
	flag.BoolVar(&isService, "k", false, "Is this a service.")
	flag.Parse()

	exitChan := make(chan int)
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	logger = log.Default()
	logger.Println("Tribes 1 Master Server")
	logger.Println("Copyright (c) 2022 Max Krivanek")

	masterServerInit()
	webServerInit()

	err := readConfig(configFile)
	if err != nil {
		logger.Printf("Failed to load config: %s", err.Error())
		logger.Println("Loading default configuration.")
		defaultConfig(configFile)
	}

	go masterServerLoop()
	go webServerLoop()

	var input string

	if !isService {
		go func(configFilename string) {
			for {
				_, err := fmt.Scanln(&input)
				if err != nil {
					if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
						logger.Println("stdin: ", err)
						break
					}
					time.Sleep(1 * time.Second)
					continue
				}
				switch input {
				case "a":
					fmt.Printf("There have been %d invalid packets.\n", masterInvalidPackets.Load())
				case "c":
					verified := 0
					count := 0
					gsList.Range(func(key any, valueAny any) bool {
						count += 1
						value := valueAny.(*GameServer)
						value.Mutex.RLock()
						defer value.Mutex.RUnlock()
						if value.Validated == Validated {
							verified += 1
						}
						return true
					})
					fmt.Printf("There are %d verified servers.\nThere are %d servers total.\n", verified, count)
				case "l":
					var lastClient *Client
					var lastClientIP string
					clientList.Range(func(keyAny any, valueAny any) bool {
						key := keyAny.(string)
						value := valueAny.(*Client)
						if lastClient == nil || lastClient.LastSeen.Load() < value.LastSeen.Load() {
							lastClient = value
							lastClientIP = key
						}
						return true
					})

					if lastClient == nil {
						fmt.Println("There hasn't been a client yet.")
						break
					}

					fmt.Printf(
						"Last client is %s.\nRate: %d\nRate Limited: %d\nQueries: %d\nHeartbeats:%d\n",
						lastClientIP,
						lastClient.Rate.Load(),
						lastClient.RateLimited.Load(),
						lastClient.Queries.Load(),
						lastClient.Heartbeats.Load(),
					)
				case "r":
					err := readConfig(configFilename)
					if err != nil {
						fmt.Printf("Config read got error: %s\n", err.Error())
						break
					}
					masterLoadConfig()
					webServerConfig()
					fmt.Println("Reloaded configuration.")
				case "s":
					var output string
					verified := 0
					gsList.Range(func(key any, valueAny any) bool {
						value := valueAny.(*GameServer)
						value.Mutex.RLock()
						defer value.Mutex.RUnlock()
						if value.Validated == Validated {
							verified += 1
						}
						return true
					})
					output = fmt.Sprintf("This master has %d servers verified.\n", verified)
					msList.Range(func(key any, valueAny any) bool {
						value := valueAny.(*MasterServer)
						value.Mutex.RLock()
						defer value.Mutex.RUnlock()
						output = fmt.Sprintf("%sMaster %s has last sent a total of %d servers.\n", output, value.Address, value.Data.ServerCount())
						return true
					})
					fmt.Print(output)
				case "x":
					logger.Println("Exiting...")
					exitChan <- 0
					return
				default:
					fmt.Printf("Unknown command %s\n", input)
				}
			}
		}(configFile)
	}

	go func(configFilename string) {
		for {
			sig := <-signalChan
			switch sig {
			case syscall.SIGTERM:
				logger.Println("Terminating...")
				exitChan <- 0
				return
			case syscall.SIGINT:
				logger.Println("Interrupt received. Exiting...")
				exitChan <- 0
				return
			case syscall.SIGHUP:
				err := readConfig(configFilename)
				if err != nil {
					fmt.Printf("Config read got error: %s\n", err.Error())
					break
				}
				masterLoadConfig()
				webServerConfig()
				fmt.Println("Reloaded configuration.")
			default:
				logger.Println("Unexpected signal: ", sig)
				exitChan <- 1
				return
			}
		}
	}(configFile)

	exitCode := <-exitChan
	os.Exit(exitCode)
}
