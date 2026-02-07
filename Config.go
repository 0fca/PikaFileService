package main

import "PikaFileService/connectors"

// BucketMapping maps a watched folder to a specific PikaCloud bucket.
// Each mapping results in files from that folder being uploaded to the designated bucket.
type BucketMapping struct {
	Folder   string `json:"folder"`   // Absolute path to the watched directory
	BucketID string `json:"bucketId"` // PikaCloud bucket UUID for this folder
}

type Config struct {
	Folders          []string                    `json:"folders"`
	WorkingDirectory string                      `json:"workDir"`
	Dst              string                      `json:"dstPath"`
	PikaCloud        *connectors.PikaCloudConfig `json:"pikaCloud,omitempty"`
	BucketMappings   []BucketMapping             `json:"bucketMappings,omitempty"`
}
