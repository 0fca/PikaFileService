package main

import (
	"PikaFileService/connectors"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func main() {
	var (
		configPath, logPath     string
		enableDebug, enableSync bool
	)

	flag.StringVar(&configPath, "c", "config.json", "Absolute path to configuration file")
	flag.StringVar(&logPath, "l", "default.log", "Absolute path to store log file, with filename")
	flag.BoolVar(&enableDebug, "d", false, "Use this to allow printing to console all information")
	flag.BoolVar(&enableSync, "s", true, "Set this to false to disallow automatic sync on start")
	flag.Parse()

	var f *os.File

	if !enableDebug {
		f = handleOutputToFile(logPath)
	}

	log.Println("PikaFileSync is starting...")

	data, err := ioutil.ReadFile(configPath)
	if err != nil {
		fmt.Println("File reading error", err)
		return
	}
	configContent := string(data)
	config := json.NewDecoder(strings.NewReader(configContent))

	var c Config
	if err := config.Decode(&c); err == io.EOF {
		return
	} else if err != nil {
		log.Fatal(err)
	}

	syncMsg := "On-startup directory contents sync is disabled"
	if enableSync {
		syncMsg = "On-startup directory content sync is enabled"
		connectors.SyncDestinationFs(c.Folders, c.Dst, c.StartSyncTimeOffset)
	}
	log.Println(syncMsg)
	StartFWatch(c.Folders, c.Dst)
	defer f.Close()
}

func handleOutputToFile(outPath string) *os.File {
	f, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0766)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	log.SetOutput(f)
	return f
}
