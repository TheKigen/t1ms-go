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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TheKigen/t1ms-go/internal/config"
	"github.com/TheKigen/t1ms-go/internal/master"
	"github.com/TheKigen/t1ms-go/internal/web"
)

func main() {
	var configFile string
	var isService bool
	flag.StringVar(&configFile, "c", "config.xml", "Specify config file path.")
	flag.BoolVar(&isService, "k", false, "Is this a service.")
	flag.Parse()

	logger := log.Default()
	logger.Println("Tribes 1 Master Server")
	logger.Println("Copyright (c) 2022 Max Krivanek")

	cfg, err := config.Load(configFile)
	if err != nil {
		logger.Printf("Failed to load config: %s", err.Error())
		logger.Println("Loading default configuration.")
		cfg = config.WriteDefault(configFile)
	} else if err := cfg.Save(configFile); err != nil {
		logger.Printf("Failed to save config: %s", err.Error())
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	masterSvc := master.NewService(logger, cfg)
	webSvc := web.NewService(logger, cfg, masterSvc)

	exitChan := make(chan int, 1)
	errChan := make(chan error, 2)

	go func() {
		if err := masterSvc.Run(ctx); err != nil {
			errChan <- fmt.Errorf("master: %w", err)
		}
	}()

	go func() {
		if err := webSvc.Run(ctx); err != nil {
			errChan <- fmt.Errorf("web: %w", err)
		}
	}()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM)

	if !isService {
		go consoleLoop(configFile, logger, cfg, masterSvc, webSvc, exitChan)
	}

	go func() {
		for {
			sig := <-signalChan
			switch sig {
			case syscall.SIGHUP:
				reloadConfig(configFile, logger, cfg, masterSvc, webSvc)
			case syscall.SIGTERM:
				logger.Println("Terminating...")
				exitChan <- 0
				return
			case syscall.SIGINT:
				logger.Println("Interrupt received. Exiting...")
				exitChan <- 0
				return
			default:
				logger.Println("Unexpected signal:", sig)
				exitChan <- 1
				return
			}
		}
	}()

	go func() {
		err := <-errChan
		logger.Printf("Fatal error: %v", err)
		exitChan <- 1
	}()

	exitCode := <-exitChan
	cancel()
	os.Exit(exitCode)
}

func reloadConfig(filename string, logger *log.Logger, cfg *config.Config, masterSvc *master.Service, webSvc *web.Service) {
	if err := cfg.Reload(filename); err != nil {
		logger.Printf("Config reload error: %s", err.Error())
		return
	}
	masterSvc.LoadConfig()
	webSvc.LoadConfig()
	logger.Println("Reloaded configuration.")
}

func consoleLoop(configFile string, logger *log.Logger, cfg *config.Config, masterSvc *master.Service, webSvc *web.Service, exitChan chan<- int) {
	var input string
	for {
		_, err := fmt.Scanln(&input)
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				logger.Println("stdin:", err)
				return
			}
			time.Sleep(time.Second)
			continue
		}

		switch input {
		case "a":
			fmt.Printf("There have been %d invalid packets.\n", masterSvc.InvalidPackets())
		case "c":
			verified, total := masterSvc.ServerCounts()
			fmt.Printf("There are %d verified servers.\nThere are %d servers total.\n", verified, total)
		case "l":
			ip, client, found := masterSvc.LastClient()
			if !found {
				fmt.Println("There hasn't been a client yet.")
				break
			}
			fmt.Printf(
				"Last client is %s.\nRate: %d\nRate Limited: %d\nQueries: %d\nHeartbeats:%d\n",
				ip,
				client.Rate.Load(),
				client.RateLimited.Load(),
				client.Queries.Load(),
				client.Heartbeats.Load(),
			)
		case "r":
			reloadConfig(configFile, logger, cfg, masterSvc, webSvc)
		case "s":
			verified, _ := masterSvc.ServerCounts()
			output := fmt.Sprintf("This master has %d servers verified.\n", verified)
			masterSvc.RangeMasterServers(func(_ string, ms *master.MasterServer) bool {
				ms.Mutex.RLock()
				defer ms.Mutex.RUnlock()
				if ms.Data != nil {
					output += fmt.Sprintf("Master %s has last sent a total of %d servers.\n", ms.Address, ms.Data.ServerCount)
				}
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
}
