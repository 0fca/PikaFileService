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
	var configPath, logPath string
	var enableDebug, fetchBuckets, authOnly bool
	flag.StringVar(&configPath, "c", "config.json", "Absolute path to configuration file")
	flag.StringVar(&logPath, "l", "default.log", "Absolute path to store log file, with filename")
	flag.BoolVar(&enableDebug, "d", false, "Use this to allow printing to console all information")
	flag.BoolVar(&fetchBuckets, "fetch-buckets", false, "Authenticate (if needed) and fetch buckets, then output JSON to stdout and exit")
	flag.BoolVar(&authOnly, "auth-only", false, "Perform OAuth2 device flow authentication, save token, and exit")
	flag.Parse()

	if !enableDebug && !fetchBuckets && !authOnly {
		handleErrorOutput(logPath)
	}

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

	// --auth-only mode: perform OAuth2 device flow authentication, save token, and exit.
	// This is used by configure.sh to authenticate without starting the file watcher.
	// The device flow displays a URL and code, polls until authorized, then saves the token.
	if authOnly {
		if c.PikaCloud == nil {
			fmt.Fprintln(os.Stderr, "ERROR: pikaCloud section missing from config")
			os.Exit(1)
		}
		if c.PikaCloud.OAuth2 == nil {
			fmt.Fprintln(os.Stderr, "ERROR: pikaCloud.oauth2 section missing from config")
			os.Exit(1)
		}
		// NewPikaCloudConnector triggers device flow auth when no cached token exists.
		// Once auth completes and the token is saved, we exit immediately.
		connector := connectors.NewPikaCloudConnector(*c.PikaCloud)
		// Verify the token was obtained by making a simple call
		_, err := connector.FetchBuckets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: auth completed but bucket test failed: %v\n", err)
			// Token may still be saved — exit 0 so the script can check the token file
		}
		fmt.Fprintln(os.Stderr, "Authentication complete. Token saved.")
		os.Exit(0)
	}

	// --fetch-buckets mode: authenticate, fetch buckets, print JSON to stdout, and exit.
	// This is used by the interactive configure.sh installer script.
	if fetchBuckets {
		if c.PikaCloud == nil {
			fmt.Fprintln(os.Stderr, "ERROR: pikaCloud section missing from config")
			os.Exit(1)
		}
		connector := connectors.NewPikaCloudConnector(*c.PikaCloud)
		buckets, err := connector.FetchBuckets()
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to fetch buckets: %v\n", err)
			os.Exit(1)
		}
		bucketsJSON, err := json.MarshalIndent(buckets, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: failed to marshal buckets: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(bucketsJSON))
		os.Exit(0)
	}

	log.Println("PikaFileSync is starting...")

	// Initialize PikaCloud handler with bucket mappings
	pikaCloudHandler := NewPikaCloudHandler(c.PikaCloud, c.PikaCloud.BucketMappings)
	if pikaCloudHandler.IsEnabled() {
		log.Println("PikaCloud integration enabled")
	} else {
		log.Println("PikaCloud integration disabled")
	}
	folders := []string{}
	for _, mapping := range c.PikaCloud.BucketMappings {
		folders = append(folders, mapping.Folder)
	}
	StartFWatch(folders, c.Dst, c.WorkingDirectory, pikaCloudHandler)
}

func handleErrorOutput(outPath string) {
	f, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	log.SetOutput(f)
}
