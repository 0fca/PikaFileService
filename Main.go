package main

import (
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
	var configPath, logPath string
	var enableDebug bool
	flag.StringVar(&configPath, "c", "config.json", "Absolute path to configuration file")
	flag.StringVar(&logPath, "l", "default.log", "Absolute path to store log file, with filename")
	flag.BoolVar(&enableDebug, "d", false, "Use this to allow printing to console all information")
	flag.Parse()

	if !enableDebug {
		handleErrorOutput(logPath)
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

	StartFWatch(c.Folders, c.Dst)
}

func handleErrorOutput(outPath string) {
	f, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	log.SetOutput(f)
}
