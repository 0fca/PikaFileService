package PikaFileService

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"strings"

	"PikaFileService/fs_watcher"
)

func Main() {
	data, err := ioutil.ReadFile("test.txt")
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

	fs_watcher.Main(c.Folders)
}

