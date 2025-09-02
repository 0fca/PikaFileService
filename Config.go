package main

import "PikaFileService/connectors"

type Config struct {
	Folders          []string                    `json:"folders"`
	WorkingDirectory string                      `json:"workDir"`
	Dst              string                      `json:"dstPath"`
	PikaCloud        *connectors.PikaCloudConfig `json:"pikaCloud,omitempty"`
}
