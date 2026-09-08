package main

import "PikaFileService/connectors"

type Config struct {
	Folders          []string                    `json:"folders,omitempty"`
	WorkingDirectory string                      `json:"workDir"`
	Dst              string                      `json:"dstPath,omitempty"`
	PikaCloud        *connectors.PikaCloudConfig `json:"pikaCloud,omitempty"`
}
