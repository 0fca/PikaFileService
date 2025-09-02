package connectors

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PikaCloudConfig holds configuration for PikaCloud API connection
type PikaCloudConfig struct {
	BaseURL    string `json:"baseUrl"`    // e.g., "https://pikacore.example.com"
	BucketID   string `json:"bucketId"`   // Target bucket ID for uploads
	Timeout    int    `json:"timeout"`    // Request timeout in seconds
	RetryCount int    `json:"retryCount"` // Number of retry attempts
}

// UploadResponse represents the response from PikaCore upload API
type UploadResponse struct {
	Detail     string `json:"detail"`
	CGID       string `json:"cgid"`
	BucketID   string `json:"bucketId"`
	TargetPath string `json:"targetPath"`
}

// PikaCloudConnector handles file uploads to PikaCore storage API
type PikaCloudConnector struct {
	config     PikaCloudConfig
	httpClient *http.Client
}

// NewPikaCloudConnector creates a new PikaCloudConnector instance
func NewPikaCloudConnector(config PikaCloudConfig) *PikaCloudConnector {
	timeout := time.Duration(config.Timeout) * time.Second
	if config.Timeout <= 0 {
		timeout = 30 * time.Second // Default 30 seconds
	}

	if config.RetryCount <= 0 {
		config.RetryCount = 3 // Default 3 retries
	}

	return &PikaCloudConnector{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// UploadFile uploads a file to PikaCore storage using the StorageApiController
func (pc *PikaCloudConnector) UploadFile(filePath string) (*UploadResponse, error) {
	// Validate input
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	log.Printf("Starting upload of file: %s to PikaCore", filePath)

	// Attempt upload with retry logic
	var lastErr error
	for attempt := 1; attempt <= pc.config.RetryCount; attempt++ {
		response, err := pc.attemptUpload(filePath, attempt)
		if err == nil {
			log.Printf("Successfully uploaded file: %s (attempt %d)", filePath, attempt)
			return response, nil
		}

		lastErr = err
		if attempt < pc.config.RetryCount {
			log.Printf("Upload attempt %d failed, retrying: %v", attempt, err)
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
		}
	}

	return nil, fmt.Errorf("upload failed after %d attempts: %v", pc.config.RetryCount, lastErr)
}

// attemptUpload performs a single upload attempt
func (pc *PikaCloudConnector) attemptUpload(filePath string, attempt int) (*UploadResponse, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}

	// Create form file field
	fileName := filepath.Base(filePath)
	// Use relative path as the field name (mimicking the expected structure)
	fieldName := strings.TrimPrefix(filePath, pc.getBasePath(filePath))
	if fieldName == "" || fieldName == filePath {
		fieldName = "file" // Fallback field name
	}

	part, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	// Copy file content to form
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file content: %v", err)
	}

	// Close the writer to finalize the form
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// Create the request
	uploadURL := fmt.Sprintf("%s/Api/v1/Storage/%s",
		strings.TrimRight(pc.config.BaseURL, "/"),
		pc.config.BucketID)

	request, err := http.NewRequest("POST", uploadURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Set headers
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("User-Agent", "PikaFileService/1.0")

	log.Printf("Attempt %d: Uploading %s (%d bytes) to %s",
		attempt, fileName, fileInfo.Size(), uploadURL)

	// Send the request
	response, err := pc.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer response.Body.Close()

	// Read response body
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	// Check response status
	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed with status %d: %s",
			response.StatusCode, string(responseBody))
	}

	// Parse response JSON
	var uploadResponse UploadResponse
	err = json.Unmarshal(responseBody, &uploadResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %v", err)
	}

	log.Printf("Upload successful: CGID=%s, TargetPath=%s",
		uploadResponse.CGID, uploadResponse.TargetPath)

	return &uploadResponse, nil
}

// getBasePath extracts the base path for relative path calculation
func (pc *PikaCloudConnector) getBasePath(filePath string) string {
	// This is a simple implementation - in a real scenario, you might want
	// to configure this based on your sync folder structure
	dir := filepath.Dir(filePath)
	return dir + string(filepath.Separator)
}

// UploadFileWithCustomPath uploads a file with a custom target path structure
func (pc *PikaCloudConnector) UploadFileWithCustomPath(filePath, targetPath string) (*UploadResponse, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if targetPath == "" {
		return nil, fmt.Errorf("target path cannot be empty")
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	log.Printf("Starting upload of file: %s to target path: %s", filePath, targetPath)

	// Attempt upload with retry logic
	var lastErr error
	for attempt := 1; attempt <= pc.config.RetryCount; attempt++ {
		response, err := pc.attemptUploadWithPath(filePath, targetPath, attempt)
		if err == nil {
			log.Printf("Successfully uploaded file: %s to path: %s (attempt %d)",
				filePath, targetPath, attempt)
			return response, nil
		}

		lastErr = err
		if attempt < pc.config.RetryCount {
			log.Printf("Upload attempt %d failed, retrying: %v", attempt, err)
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return nil, fmt.Errorf("upload failed after %d attempts: %v", pc.config.RetryCount, lastErr)
}

// attemptUploadWithPath performs a single upload attempt with custom path
func (pc *PikaCloudConnector) attemptUploadWithPath(filePath, targetPath string, attempt int) (*UploadResponse, error) {
	// Open the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create multipart form
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}

	// Create form file field with custom target path
	fileName := filepath.Base(filePath)
	part, err := writer.CreateFormFile(targetPath, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	// Copy file content to form
	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file content: %v", err)
	}

	// Close the writer
	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %v", err)
	}

	// Create and send request
	uploadURL := fmt.Sprintf("%s/Api/v1/Storage/%s",
		strings.TrimRight(pc.config.BaseURL, "/"),
		pc.config.BucketID)

	request, err := http.NewRequest("POST", uploadURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("User-Agent", "PikaFileService/1.0")

	log.Printf("Attempt %d: Uploading %s (%d bytes) to %s with target path: %s",
		attempt, fileName, fileInfo.Size(), uploadURL, targetPath)

	response, err := pc.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if response.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("upload failed with status %d: %s",
			response.StatusCode, string(responseBody))
	}

	var uploadResponse UploadResponse
	err = json.Unmarshal(responseBody, &uploadResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response JSON: %v", err)
	}

	return &uploadResponse, nil
}

// TestConnection tests the connection to PikaCore API
func (pc *PikaCloudConnector) TestConnection() error {
	testURL := fmt.Sprintf("%s/Api/v1/Storage/Index",
		strings.TrimRight(pc.config.BaseURL, "/"))

	request, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create test request: %v", err)
	}

	request.Header.Set("User-Agent", "PikaFileService/1.0")

	response, err := pc.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("failed to connect to PikaCore: %v", err)
	}
	defer response.Body.Close()

	log.Printf("Connection test to PikaCore: Status %d", response.StatusCode)
	return nil
}
