# PikaCloudConnector

PikaCloudConnector is a Go package that enables uploading files to PikaCore's Storage API through the StorageApiController#Upload action.

## Features

- **HTTP Multipart Upload**: Uploads files using multipart form data to match PikaCore's expected format
- **Retry Logic**: Configurable retry attempts with exponential backoff
- **Timeout Handling**: Configurable request timeouts
- **Custom Path Support**: Upload files with custom target paths
- **Connection Testing**: Test connectivity to PikaCore API
- **Background Upload**: Non-blocking file uploads when integrated with file watcher
- **Error Handling**: Comprehensive error handling and logging

## Configuration

The connector is configured using `PikaCloudConfig`:

```go
type PikaCloudConfig struct {
    BaseURL    string `json:"baseUrl"`    // e.g., "https://pikacore.example.com"
    BucketID   string `json:"bucketId"`   // Target bucket ID for uploads
    Timeout    int    `json:"timeout"`    // Request timeout in seconds
    RetryCount int    `json:"retryCount"` // Number of retry attempts
}
```

### Example Configuration

```json
{
    "pikaCloud": {
        "baseUrl": "https://pikacore.example.com",
        "bucketId": "12345678-1234-1234-1234-123456789012",
        "timeout": 30,
        "retryCount": 3
    }
}
```

## Usage

### Basic Upload

```go
config := connectors.PikaCloudConfig{
    BaseURL:    "https://pikacore.example.com",
    BucketID:   "your-bucket-id",
    Timeout:    30,
    RetryCount: 3,
}

connector := connectors.NewPikaCloudConnector(config)

// Upload a file
response, err := connector.UploadFile("/path/to/file.txt")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Upload successful: CGID=%s\n", response.CGID)
```

### Upload with Custom Path

```go
// Upload with specific target path structure
response, err := connector.UploadFileWithCustomPath(
    "/local/file.txt", 
    "documents/subfolder/file.txt")
```

### Test Connection

```go
if err := connector.TestConnection(); err != nil {
    log.Printf("Connection failed: %v", err)
}
```

## Integration with File Watcher

The connector integrates with PikaFileService's file watcher through `PikaCloudHandler`:

```go
// Initialize PikaCloud handler
pikaCloudHandler := NewPikaCloudHandler(config.PikaCloud)

// Start file watcher with PikaCloud integration
StartFWatchWithPikaCloud(folders, dstPath, workDir, pikaCloudHandler)
```

When files are created, modified, or renamed in watched directories, they are automatically uploaded to PikaCloud in addition to local filesystem operations.

## API Compatibility

The connector is designed to work with PikaCore's StorageApiController which expects:

- **HTTP Method**: POST
- **Content-Type**: multipart/form-data
- **URL Pattern**: `/Api/v1/Storage/{bucketId}`
- **Form Field**: File content with field name representing the target path
- **Response**: JSON with `detail`, `cgid`, `bucketId`, and `targetPath` fields

### Expected Response Format

```json
{
    "detail": "Wgrano plik",
    "cgid": "generated-guid",
    "bucketId": "bucket-id",
    "targetPath": "encoded-target-path"
}
```

## Testing

Use the test utility to verify your configuration:

```bash
# Build test utility
cd tools
go build -o test_pikacloud test_pikacloud.go

# Test with a specific file
./test_pikacloud -url https://pikacore.example.com -bucket your-bucket-id -file /path/to/test.txt

# Test with auto-generated test file
./test_pikacloud -url https://pikacore.example.com -bucket your-bucket-id -test
```

## File Watcher Integration

The PikaCloudHandler provides seamless integration with the file watcher:

### Automatic Operations

1. **File Creation**: New files are uploaded to PikaCloud
2. **File Modification**: Modified files are re-uploaded
3. **File Rename**: Renamed files are uploaded with new names
4. **File Deletion**: Local deletion is logged (delete API not yet implemented)

### Path Mapping

Files are uploaded to PikaCloud using relative paths from the working directory:

- Local: `/home/user/sync/documents/file.txt`
- Working Dir: `/home/user/sync`  
- PikaCloud Path: `documents/file.txt`

## Error Handling

The connector includes comprehensive error handling:

- **Network Errors**: Connection timeouts, DNS failures
- **HTTP Errors**: 4xx/5xx status codes with detailed messages
- **File Errors**: Missing files, permission issues
- **JSON Errors**: Invalid response parsing
- **Retry Logic**: Automatic retries with exponential backoff

## Security Notes

⚠️ **Current Limitations**:
- Authentication is not implemented (authorization bypassed as requested)
- CSRF protection is not implemented (antiforgery token validation bypassed)
- No TLS certificate validation

For production use, you should implement:
- Proper authentication (JWT tokens, API keys, etc.)
- CSRF protection
- TLS certificate validation
- Rate limiting compliance

## Dependencies

- Go 1.15+
- Standard library only (no external dependencies for the connector)
- `github.com/radovskyb/watcher` (for file watcher integration)

## Logging

The connector provides detailed logging for:
- Upload attempts and results
- Connection tests
- Error conditions
- Retry attempts
- File operations

Logs are written using Go's standard `log` package and integrate with PikaFileService's logging configuration.
