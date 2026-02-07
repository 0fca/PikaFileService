package main

import (
	"PikaFileService/connectors"
	"log"
	"path/filepath"
	"strings"

	"github.com/radovskyb/watcher"
)

// PikaCloudHandler handles file operations with PikaCloud integration
type PikaCloudHandler struct {
	connector      *connectors.PikaCloudConnector
	enabled        bool
	bucketMappings []connectors.BucketMapping
}

// NewPikaCloudHandler creates a new PikaCloud handler
func NewPikaCloudHandler(config *connectors.PikaCloudConfig, mappings []connectors.BucketMapping) *PikaCloudHandler {
	if config == nil {
		return &PikaCloudHandler{enabled: false}
	}

	connector := connectors.NewPikaCloudConnector(*config)

	// Test connection on initialization
	if err := connector.TestConnection(); err != nil {
		log.Printf("Warning: PikaCloud connection test failed: %v", err)
		log.Println("PikaCloud uploads will be disabled")
		return &PikaCloudHandler{enabled: false}
	}

	log.Println("PikaCloud connector initialized successfully")
	if len(mappings) > 0 {
		log.Printf("Loaded %d bucket mapping(s)", len(mappings))
	}
	return &PikaCloudHandler{
		connector:      connector,
		enabled:        true,
		bucketMappings: mappings,
	}
}

// HandleFileOperation processes file operations with PikaCloud integration
func (pch *PikaCloudHandler) HandleFileOperation(event watcher.Event, workDir string) {
	if pch.enabled && pch.connector != nil {
		pch.handleCloudOperation(event, workDir)
	}
}

// handleCloudOperation handles cloud-specific operations
func (pch *PikaCloudHandler) handleCloudOperation(event watcher.Event, workDir string) {
	switch event.Op {
	case watcher.Create, watcher.Write:
		if !event.IsDir() {
			bucketID := pch.resolveBucketID(event.Path)
			if bucketID == "" {
				log.Printf("No bucket mapping found for %s, skipping cloud upload", event.Path)
				return
			}
			pch.uploadFile(event.Path, workDir, bucketID)
		}
	case watcher.Rename:
		log.Printf("File renamed locally: %s (PikaCloud rename not implemented)", event.Path)
	case watcher.Remove:
		log.Printf("File deleted locally: %s (PikaCloud delete not implemented)", event.Path)
	}
}

// resolveBucketID finds the bucket ID for a file path based on bucket mappings.
// Falls back to the global PikaCloudConfig.BucketID if no mapping matches.
func (pch *PikaCloudHandler) resolveBucketID(filePath string) string {
	// Check bucket mappings — find the longest matching folder prefix
	var bestMatch string
	var bestBucketID string
	for _, m := range pch.bucketMappings {
		folder := strings.TrimRight(m.Folder, string(filepath.Separator)) + string(filepath.Separator)
		if strings.HasPrefix(filePath, folder) && len(m.Folder) > len(bestMatch) {
			bestMatch = m.Folder
			bestBucketID = m.BucketID
		}
	}
	if bestBucketID != "" {
		return bestBucketID
	}
	// Fallback to global bucketId for backward compatibility
	return pch.connector.GetBucketID()
}

// uploadFile uploads a file to PikaCloud using the resolved bucket ID
func (pch *PikaCloudHandler) uploadFile(filePath, workDir, bucketID string) {
	// Create relative path for PikaCloud storage
	relativePath := strings.TrimPrefix(filePath, workDir)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))

	// Replace path separators with forward slashes for web compatibility
	relativePath = strings.ReplaceAll(relativePath, string(filepath.Separator), "/")

	log.Printf("Uploading file to PikaCloud: %s -> %s (bucket: %s)", filePath, relativePath, bucketID)

	go func() {
		// Upload in background to avoid blocking file watcher
		response, err := pch.connector.UploadFileToBucket(filePath, relativePath, bucketID)
		if err != nil {
			log.Printf("Failed to upload file to PikaCloud: %v", err)
			return
		}

		log.Printf("Successfully uploaded to PikaCloud: CGID=%s, Path=%s",
			response.CGID, response.TargetPath)
	}()
}

// IsEnabled returns whether PikaCloud integration is enabled
func (pch *PikaCloudHandler) IsEnabled() bool {
	return pch.enabled
}
