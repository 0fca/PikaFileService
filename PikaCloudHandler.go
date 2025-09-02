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
	connector *connectors.PikaCloudConnector
	enabled   bool
}

// NewPikaCloudHandler creates a new PikaCloud handler
func NewPikaCloudHandler(config *connectors.PikaCloudConfig) *PikaCloudHandler {
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
	return &PikaCloudHandler{
		connector: connector,
		enabled:   true,
	}
}

// HandleFileOperation processes file operations with PikaCloud integration
func (pch *PikaCloudHandler) HandleFileOperation(event watcher.Event, dstPath string, workDir string) {
	// First, execute the standard filesystem operation
	executeFilesystemOperation(event, dstPath, workDir)

	// If PikaCloud is enabled, also upload to cloud
	if pch.enabled && pch.connector != nil {
		pch.handleCloudOperation(event, workDir)
	}
}

// handleCloudOperation handles cloud-specific operations
func (pch *PikaCloudHandler) handleCloudOperation(event watcher.Event, workDir string) {
	switch event.Op {
	case watcher.Create, watcher.Write:
		if !event.IsDir() {
			pch.uploadFile(event.Path, workDir)
		}
	case watcher.Rename:
		log.Printf("File renamed locally: %s (PikaCloud rename not implemented)", event.Path)
	case watcher.Remove:
		log.Printf("File deleted locally: %s (PikaCloud delete not implemented)", event.Path)
	}
}

// uploadFile uploads a file to PikaCloud
func (pch *PikaCloudHandler) uploadFile(filePath, workDir string) {
	// Create relative path for PikaCloud storage
	relativePath := strings.TrimPrefix(filePath, workDir)
	relativePath = strings.TrimPrefix(relativePath, string(filepath.Separator))

	// Replace path separators with forward slashes for web compatibility
	relativePath = strings.ReplaceAll(relativePath, string(filepath.Separator), "/")

	log.Printf("Uploading file to PikaCloud: %s -> %s", filePath, relativePath)

	go func() {
		// Upload in background to avoid blocking file watcher
		response, err := pch.connector.UploadFileWithCustomPath(filePath, relativePath)
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
