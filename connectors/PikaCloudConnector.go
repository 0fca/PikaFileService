package connectors

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// PikaCloudConfig holds configuration for PikaCloud API connection
type PikaCloudConfig struct {
	BaseURL    string                  `json:"baseUrl"`    // e.g., "https://pikacore.example.com"
	BucketID   string                  `json:"bucketId"`   // Target bucket ID for uploads
	AuthToken  string                  `json:"authToken"`  // Static JWT token for .AspNet.Identity cookie (fallback)
	OAuth2     *OAuth2DeviceFlowConfig `json:"oauth2"`     // OAuth2 Device Flow config (preferred over static AuthToken)
	Timeout    int                     `json:"timeout"`    // Request timeout in seconds
	RetryCount int                     `json:"retryCount"` // Number of retry attempts
}

// UploadResponse represents the response from PikaCore upload API
type UploadResponse struct {
	Detail     string `json:"detail"`
	CGID       string `json:"cgid"`
	BucketID   string `json:"bucketId"`
	TargetPath string `json:"targetPath"`
}

// BucketInfo represents a single bucket returned by the PikaCore Buckets endpoint
type BucketInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Encrypted bool   `json:"encrypted"`
}

// PikaCloudConnector handles file uploads to PikaCore storage API
type PikaCloudConnector struct {
	config         PikaCloudConfig
	httpClient     *http.Client
	csrfToken      string
	deviceFlowAuth *DeviceFlowAuth
}

// RateLimitError represents a 429 Too Many Requests response
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	return e.Message
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

	jar, err := cookiejar.New(nil)
	if err != nil {
		log.Printf("Warning: failed to create cookie jar: %v", err)
	}

	connector := &PikaCloudConnector{
		config: config,
		httpClient: &http.Client{
			Timeout: timeout,
			Jar:     jar,
		},
	}

	// Set auth cookie: prefer OAuth2 device flow, fallback to static token
	if config.OAuth2 != nil {
		dfa, err := NewDeviceFlowAuth(*config.OAuth2)
		if err != nil {
			log.Printf("Warning: OAuth2 device flow initialization failed: %v", err)
			log.Println("Falling back to static AuthToken if available")
		} else {
			connector.deviceFlowAuth = dfa
			// Set initial auth cookie from device flow token
			if err := connector.refreshAuthToken(); err != nil {
				log.Printf("Warning: failed to set initial auth cookie from OAuth2: %v", err)
			}
		}
	}

	// Fallback: set static auth cookie if no device flow auth and static token provided
	if connector.deviceFlowAuth == nil && config.AuthToken != "" && jar != nil {
		connector.setAuthCookie()
	}

	return connector
}

// setAuthCookie adds the .AspNet.Identity JWT cookie to the cookie jar
func (pc *PikaCloudConnector) setAuthCookie() {
	baseURL, err := url.Parse(pc.config.BaseURL)
	if err != nil {
		log.Printf("Warning: failed to parse base URL for auth cookie: %v", err)
		return
	}
	pc.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{
		{
			Name:  ".AspNet.Identity",
			Value: pc.config.AuthToken,
			Path:  "/",
		},
	})
}

// GetBucketID returns the default bucket ID from the config.
// Used as a fallback when no per-folder bucket mapping is defined.
func (pc *PikaCloudConnector) GetBucketID() string {
	return pc.config.BucketID
}

// refreshAuthToken gets a fresh access token from the device flow auth and updates the .AspNet.Identity cookie.
// This is called before each request to ensure the cookie contains a non-expired JWT.
func (pc *PikaCloudConnector) refreshAuthToken() error {
	if pc.deviceFlowAuth == nil {
		return nil // Using static token, no refresh needed
	}

	token, err := pc.deviceFlowAuth.GetAccessToken()
	if err != nil {
		return fmt.Errorf("failed to refresh OAuth2 access token: %v", err)
	}

	baseURL, err := url.Parse(pc.config.BaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base URL for auth cookie: %v", err)
	}
	pc.httpClient.Jar.SetCookies(baseURL, []*http.Cookie{
		{
			Name:  ".AspNet.Identity",
			Value: token,
			Path:  "/",
		},
	})

	return nil
}

// FetchCSRFToken retrieves the CSRF token from the Storage Index endpoint.
// The Index action has [GenerateAntiforgeryTokenCookie] which sets both the
// PikaCore.Antiforgery validation cookie and a RequestVerificationToken cookie.
// The RequestVerificationToken value is then sent as the X-CSRF-TOKEN header
// on subsequent upload requests.
func (pc *PikaCloudConnector) FetchCSRFToken() error {
	indexURL := fmt.Sprintf("%s/Api/v1/Storage/Index",
		strings.TrimRight(pc.config.BaseURL, "/"))

	request, err := http.NewRequest("GET", indexURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create CSRF token request: %v", err)
	}
	request.Header.Set("User-Agent", "PikaFileService/1.0")

	response, err := pc.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("failed to fetch CSRF token: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("CSRF token request failed with status %d (expected 200 OK)", response.StatusCode)
	}

	// Extract RequestVerificationToken from cookies captured by the jar
	baseURL, err := url.Parse(pc.config.BaseURL)
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %v", err)
	}
	for _, cookie := range pc.httpClient.Jar.Cookies(baseURL) {
		if cookie.Name == "RequestVerificationToken" {
			pc.csrfToken = cookie.Value
			log.Println("Successfully retrieved CSRF token from Storage Index")
			return nil
		}
	}

	return fmt.Errorf("RequestVerificationToken cookie not found in Index response")
}

// parseRetryAfter parses the Retry-After header value into a duration
func parseRetryAfter(header string) time.Duration {
	if header == "" {
		return 5 * time.Second
	}
	if seconds, err := strconv.Atoi(header); err == nil {
		return time.Duration(seconds) * time.Second
	}
	if t, err := http.ParseTime(header); err == nil {
		duration := time.Until(t)
		if duration > 0 {
			return duration
		}
	}
	return 5 * time.Second
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

	// Refresh auth token (auto-refresh from OAuth2 device flow if configured)
	if err := pc.refreshAuthToken(); err != nil {
		log.Printf("Warning: auth token refresh failed: %v", err)
	}

	// Fetch CSRF token before upload
	if err := pc.FetchCSRFToken(); err != nil {
		return nil, fmt.Errorf("failed to retrieve CSRF token: %v", err)
	}

	// Attempt upload with retry logic
	var lastErr error
	for attempt := 1; attempt <= pc.config.RetryCount; attempt++ {
		response, err := pc.attemptUpload(filePath, attempt)
		if err == nil {
			log.Printf("Successfully uploaded file: %s (attempt %d)", filePath, attempt)
			return response, nil
		}

		lastErr = err
		var rateLimitErr *RateLimitError
		if errors.As(err, &rateLimitErr) {
			log.Printf("Rate limited on attempt %d, waiting %v before retry", attempt, rateLimitErr.RetryAfter)
			time.Sleep(rateLimitErr.RetryAfter)
		} else if attempt < pc.config.RetryCount {
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
	if pc.csrfToken != "" {
		request.Header.Set("X-CSRF-TOKEN", pc.csrfToken)
	}

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

	// Handle rate limiting (429 Too Many Requests)
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(response.Header.Get("Retry-After"))
		return nil, &RateLimitError{
			RetryAfter: retryAfter,
			Message:    fmt.Sprintf("rate limited (429), retry after %v", retryAfter),
		}
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

	// Refresh auth token (auto-refresh from OAuth2 device flow if configured)
	if err := pc.refreshAuthToken(); err != nil {
		log.Printf("Warning: auth token refresh failed: %v", err)
	}

	// Fetch CSRF token before upload
	if err := pc.FetchCSRFToken(); err != nil {
		return nil, fmt.Errorf("failed to retrieve CSRF token: %v", err)
	}

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
		var rateLimitErr *RateLimitError
		if errors.As(err, &rateLimitErr) {
			log.Printf("Rate limited on attempt %d, waiting %v before retry", attempt, rateLimitErr.RetryAfter)
			time.Sleep(rateLimitErr.RetryAfter)
		} else if attempt < pc.config.RetryCount {
			log.Printf("Upload attempt %d failed, retrying: %v", attempt, err)
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return nil, fmt.Errorf("upload failed after %d attempts: %v", pc.config.RetryCount, lastErr)
}

// UploadFileToBucket uploads a file with a custom target path to a specific bucket.
// This is used when bucket mappings are configured — each folder maps to its own bucket.
func (pc *PikaCloudConnector) UploadFileToBucket(filePath, targetPath, bucketID string) (*UploadResponse, error) {
	if filePath == "" {
		return nil, fmt.Errorf("file path cannot be empty")
	}
	if targetPath == "" {
		return nil, fmt.Errorf("target path cannot be empty")
	}
	if bucketID == "" {
		return nil, fmt.Errorf("bucket ID cannot be empty")
	}

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("file does not exist: %s", filePath)
	}

	log.Printf("Starting upload of file: %s to bucket: %s (path: %s)", filePath, bucketID, targetPath)

	if err := pc.refreshAuthToken(); err != nil {
		log.Printf("Warning: auth token refresh failed: %v", err)
	}

	if err := pc.FetchCSRFToken(); err != nil {
		return nil, fmt.Errorf("failed to retrieve CSRF token: %v", err)
	}

	var lastErr error
	for attempt := 1; attempt <= pc.config.RetryCount; attempt++ {
		response, err := pc.attemptUploadToBucket(filePath, targetPath, bucketID, attempt)
		if err == nil {
			log.Printf("Successfully uploaded file: %s to bucket %s (attempt %d)",
				filePath, bucketID, attempt)
			return response, nil
		}

		lastErr = err
		var rateLimitErr *RateLimitError
		if errors.As(err, &rateLimitErr) {
			log.Printf("Rate limited on attempt %d, waiting %v before retry", attempt, rateLimitErr.RetryAfter)
			time.Sleep(rateLimitErr.RetryAfter)
		} else if attempt < pc.config.RetryCount {
			log.Printf("Upload attempt %d failed, retrying: %v", attempt, err)
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return nil, fmt.Errorf("upload failed after %d attempts: %v", pc.config.RetryCount, lastErr)
}

// attemptUploadToBucket performs a single upload attempt to a specific bucket
func (pc *PikaCloudConnector) attemptUploadToBucket(filePath, targetPath, bucketID string, attempt int) (*UploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %v", err)
	}

	fileName := filepath.Base(filePath)
	part, err := writer.CreateFormFile(targetPath, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	_, err = io.Copy(part, file)
	if err != nil {
		return nil, fmt.Errorf("failed to copy file content: %v", err)
	}

	err = writer.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %v", err)
	}

	uploadURL := fmt.Sprintf("%s/Api/v1/Storage/%s",
		strings.TrimRight(pc.config.BaseURL, "/"),
		bucketID)

	request, err := http.NewRequest("POST", uploadURL, &requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("User-Agent", "PikaFileService/1.0")
	if pc.csrfToken != "" {
		request.Header.Set("X-CSRF-TOKEN", pc.csrfToken)
	}

	log.Printf("Attempt %d: Uploading %s (%d bytes) to bucket %s with target path: %s",
		attempt, fileName, fileInfo.Size(), bucketID, targetPath)

	response, err := pc.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(response.Header.Get("Retry-After"))
		return nil, &RateLimitError{
			RetryAfter: retryAfter,
			Message:    fmt.Sprintf("rate limited (429), retry after %v", retryAfter),
		}
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
	if pc.csrfToken != "" {
		request.Header.Set("X-CSRF-TOKEN", pc.csrfToken)
	}

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

	// Handle rate limiting (429 Too Many Requests)
	if response.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(response.Header.Get("Retry-After"))
		return nil, &RateLimitError{
			RetryAfter: retryAfter,
			Message:    fmt.Sprintf("rate limited (429), retry after %v", retryAfter),
		}
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

// FetchBuckets retrieves the list of buckets the authenticated user has access to
// via the GET /Api/v1/Storage/Buckets endpoint.
func (pc *PikaCloudConnector) FetchBuckets() ([]BucketInfo, error) {
	// Refresh auth token before fetching buckets
	if err := pc.refreshAuthToken(); err != nil {
		return nil, fmt.Errorf("auth token refresh failed: %v", err)
	}

	bucketsURL := fmt.Sprintf("%s/Api/v1/Storage/Buckets",
		strings.TrimRight(pc.config.BaseURL, "/"))

	request, err := http.NewRequest("GET", bucketsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create buckets request: %v", err)
	}
	request.Header.Set("User-Agent", "PikaFileService/1.0")

	response, err := pc.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch buckets: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return nil, fmt.Errorf("buckets request failed with status %d: %s", response.StatusCode, string(body))
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read buckets response: %v", err)
	}

	var buckets []BucketInfo
	if err := json.Unmarshal(body, &buckets); err != nil {
		return nil, fmt.Errorf("failed to parse buckets response: %v", err)
	}

	log.Printf("Fetched %d bucket(s) from PikaCloud", len(buckets))
	return buckets, nil
}

// TestConnection tests the connection to PikaCore API
func (pc *PikaCloudConnector) TestConnection() error {
	// Refresh auth token before testing connection
	if err := pc.refreshAuthToken(); err != nil {
		log.Printf("Warning: auth token refresh failed before connection test: %v", err)
	}

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
	log.Printf("%d", response.StatusCode)
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("connection test failed with status %d (expected 200 OK)", response.StatusCode)
	}

	log.Printf("Connection test to PikaCore: Status %d OK", response.StatusCode)
	return nil
}
