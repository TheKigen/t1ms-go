package main

import (
	"encoding/xml"
	"io"
	"os"
)

func defaultConfig(configFilename string) {
	config.MasterConfig.ListenAddress = ":28000"
	config.MasterConfig.Name = "Tribes Master"
	config.MasterConfig.MOTD = "<jl><f3>Example MOTD<n>Have fun,<n>- Kigen"
	config.MasterConfig.RateLimit = 2
	config.MasterConfig.QueryTimeSeconds = 120
	config.MasterConfig.BuildPacketsEverySecs = 1
	config.MasterConfig.Masters.Masters = append(
		config.MasterConfig.Masters.Masters,
		MasterConfig{Name: "T1M1 Tribes1.co", Address: "t1m1.tribes1.co:28000"},
	)
	config.MasterConfig.Masters.Masters = append(
		config.MasterConfig.Masters.Masters,
		MasterConfig{Name: "T1M2 Tribes1.co", Address: "t1m2.tribes1.co:28000"},
	)
	config.MasterConfig.Masters.Masters = append(
		config.MasterConfig.Masters.Masters,
		MasterConfig{Name: "T1M3 Tribes1.co", Address: "t1m3.tribes1.co:28000"},
	)
	config.MasterConfig.Masters.Masters = append(
		config.MasterConfig.Masters.Masters,
		MasterConfig{Name: "T1M1 PU.net", Address: "t1m1.pu.net:28000"},
	)
	config.MasterConfig.Masters.Masters = append(
		config.MasterConfig.Masters.Masters,
		MasterConfig{Name: "T1M2 PU.net", Address: "t1m2.pu.net:28000"},
	)
	config.MasterConfig.Masters.Masters = append(
		config.MasterConfig.Masters.Masters,
		MasterConfig{Name: "T1M3 PU.net", Address: "t1m3.pu.net:28000"},
	)

	config.WebConfig.ListenAddress = "127.0.0.1:8080"
	config.WebConfig.Name = "Tribes Master"
	config.WebConfig.Debug = false
	config.WebConfig.BuildPagesEverySecs = 1

	configFile, err := os.OpenFile(configFilename, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		logger.Println(err)
		return
	}

	defer func(configFile *os.File) {
		err := configFile.Close()
		if err != nil {
			panic(err)
		}
	}(configFile)

	data, err := xml.MarshalIndent(&config, "", "\t")
	if err != nil {
		logger.Println(err)
		return
	}

	_, err = configFile.Write(data)
	if err != nil {
		logger.Println(err)
		return
	}
}

func readConfig(configFilename string) error {
	configFile, err := os.Open(configFilename)
	if err != nil {
		return err
	}

	defer func(configFile *os.File) {
		err := configFile.Close()
		if err != nil {
			panic(err)
		}
	}(configFile)

	data, _ := io.ReadAll(configFile)

	var newConfig Config

	err = xml.Unmarshal(data, &newConfig)
	if err != nil {
		return err
	}

	// TODO: Test Config

	config.Mutex.Lock()
	config.MasterConfig = newConfig.MasterConfig
	config.WebConfig = newConfig.WebConfig
	config.Mutex.Unlock()

	return nil
}
