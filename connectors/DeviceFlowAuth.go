package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// OAuth2DeviceFlowConfig holds configuration for OAuth2 Device Authorization Flow (RFC 8628)
type OAuth2DeviceFlowConfig struct {
	IssuerURL string   `json:"issuerUrl"` // IdP base URL, e.g., "https://idp.lukas-bownik.net"
	Realm     string   `json:"realm"`     // Keycloak realm name
	ClientID  string   `json:"clientId"`  // OAuth2 client ID (public client, no secret)
	Scopes    []string `json:"scopes"`    // OAuth2 scopes to request, e.g., ["openid", "profile"]
	TokenFile string   `json:"tokenFile"` // Path to persist token for reuse across restarts (optional)
}

// oidcDiscoveryResponse represents relevant fields from the OpenID Connect discovery document
type oidcDiscoveryResponse struct {
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
}

// DeviceFlowAuth manages OAuth2 Device Authorization Flow lifecycle:
// OIDC discovery, interactive device authorization, token caching, and auto-refresh.
type DeviceFlowAuth struct {
	config      OAuth2DeviceFlowConfig
	oauthConfig *oauth2.Config
	tokenSource oauth2.TokenSource
	mu          sync.RWMutex
}

// NewDeviceFlowAuth creates a DeviceFlowAuth, discovers OIDC endpoints,
// loads cached tokens, and if needed runs the interactive device authorization flow.
func NewDeviceFlowAuth(config OAuth2DeviceFlowConfig) (*DeviceFlowAuth, error) {
	if config.IssuerURL == "" {
		return nil, fmt.Errorf("OAuth2 issuerUrl is required")
	}
	if config.Realm == "" {
		return nil, fmt.Errorf("OAuth2 realm is required")
	}
	if config.ClientID == "" {
		return nil, fmt.Errorf("OAuth2 clientId is required")
	}

	dfa := &DeviceFlowAuth{
		config: config,
	}

	// Discover OIDC endpoints from well-known URL
	discovery, err := dfa.discoverEndpoints()
	if err != nil {
		return nil, fmt.Errorf("OIDC discovery failed: %v", err)
	}

	dfa.oauthConfig = &oauth2.Config{
		ClientID: config.ClientID,
		Endpoint: oauth2.Endpoint{
			AuthURL:       discovery.AuthorizationEndpoint,
			TokenURL:      discovery.TokenEndpoint,
			DeviceAuthURL: discovery.DeviceAuthorizationEndpoint,
		},
		Scopes: config.Scopes,
	}

	// Attempt to reuse a cached token
	cachedToken := dfa.loadCachedToken()

	if cachedToken != nil && cachedToken.Valid() {
		log.Println("Using cached OAuth2 token (still valid)")
		dfa.tokenSource = oauth2.ReuseTokenSource(
			cachedToken,
			dfa.oauthConfig.TokenSource(context.Background(), cachedToken),
		)
		return dfa, nil
	}

	if cachedToken != nil && cachedToken.RefreshToken != "" {
		log.Println("Cached OAuth2 token expired, attempting refresh...")
		refreshSource := dfa.oauthConfig.TokenSource(context.Background(), cachedToken)
		newToken, err := refreshSource.Token()
		if err == nil {
			log.Println("OAuth2 token refreshed successfully")
			dfa.saveCachedToken(newToken)
			dfa.tokenSource = oauth2.ReuseTokenSource(
				newToken,
				dfa.oauthConfig.TokenSource(context.Background(), newToken),
			)
			return dfa, nil
		}
		log.Printf("Token refresh failed, falling back to device flow: %v", err)
	}

	// No usable cached token — run the interactive device authorization flow
	if err := dfa.runDeviceFlow(); err != nil {
		return nil, err
	}

	return dfa, nil
}

// GetWellKnownURL returns the OIDC discovery URL for the configured Keycloak realm.
// Format: {issuerUrl}/realms/{realm}/.well-known/openid-configuration
func (dfa *DeviceFlowAuth) GetWellKnownURL() string {
	return fmt.Sprintf("%s/realms/%s/.well-known/openid-configuration",
		strings.TrimRight(dfa.config.IssuerURL, "/"),
		dfa.config.Realm,
	)
}

// discoverEndpoints fetches and parses the OIDC discovery document
func (dfa *DeviceFlowAuth) discoverEndpoints() (*oidcDiscoveryResponse, error) {
	wellKnownURL := dfa.GetWellKnownURL()
	log.Printf("Discovering OIDC endpoints from: %s", wellKnownURL)

	resp, err := http.Get(wellKnownURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch discovery document: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("discovery endpoint returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery response: %v", err)
	}

	var discovery oidcDiscoveryResponse
	if err := json.Unmarshal(body, &discovery); err != nil {
		return nil, fmt.Errorf("failed to parse discovery document: %v", err)
	}

	if discovery.DeviceAuthorizationEndpoint == "" {
		return nil, fmt.Errorf("IdP does not support device authorization grant (device_authorization_endpoint missing from discovery document)")
	}
	if discovery.TokenEndpoint == "" {
		return nil, fmt.Errorf("token_endpoint missing from discovery document")
	}

	log.Printf("Discovered token endpoint: %s", discovery.TokenEndpoint)
	log.Printf("Discovered device authorization endpoint: %s", discovery.DeviceAuthorizationEndpoint)

	return &discovery, nil
}

// runDeviceFlow performs the interactive device authorization flow (RFC 8628).
// It prints instructions for the user, then blocks polling until the user authorizes.
func (dfa *DeviceFlowAuth) runDeviceFlow() error {
	ctx := context.Background()

	deviceAuth, err := dfa.oauthConfig.DeviceAuth(ctx)
	if err != nil {
		return fmt.Errorf("device authorization request failed: %v", err)
	}

	// Print instructions for the user
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║       PikaCloud Authentication Required         ║")
	fmt.Println("╠══════════════════════════════════════════════════╣")
	fmt.Printf("║  Visit:  %-39s ║\n", deviceAuth.VerificationURI)
	fmt.Printf("║  Code:   %-39s ║\n", deviceAuth.UserCode)
	fmt.Println("╚══════════════════════════════════════════════════╝")
	if deviceAuth.VerificationURIComplete != "" {
		fmt.Printf("  Or open directly: %s\n", deviceAuth.VerificationURIComplete)
	}
	fmt.Printf("  Code expires: %s\n", deviceAuth.Expiry.Format(time.RFC1123))
	fmt.Println("  Waiting for authorization to complete. After you are done, hit Enter to continue...")
	fmt.Println()

	// Poll token endpoint until user authorizes (or code expires)
	token, err := dfa.oauthConfig.DeviceAccessToken(ctx, deviceAuth)
	if err != nil {
		return fmt.Errorf("device token exchange failed (user may not have authorized in time): %v", err)
	}

	log.Println("OAuth2 device flow authentication successful")

	dfa.saveCachedToken(token)

	dfa.mu.Lock()
	dfa.tokenSource = oauth2.ReuseTokenSource(
		token,
		dfa.oauthConfig.TokenSource(ctx, token),
	)
	dfa.mu.Unlock()

	return nil
}

// GetAccessToken returns a valid access token, auto-refreshing via the refresh token if needed.
func (dfa *DeviceFlowAuth) GetAccessToken() (string, error) {
	dfa.mu.RLock()
	ts := dfa.tokenSource
	dfa.mu.RUnlock()

	if ts == nil {
		return "", fmt.Errorf("no token source available (device flow may not have completed)")
	}

	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %v", err)
	}

	// Persist the (possibly refreshed) token
	dfa.saveCachedToken(token)

	return token.AccessToken, nil
}

// loadCachedToken loads a previously persisted token from disk
func (dfa *DeviceFlowAuth) loadCachedToken() *oauth2.Token {
	tokenFile := dfa.getTokenFilePath()
	if tokenFile == "" {
		return nil
	}

	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return nil
	}

	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		log.Printf("Warning: failed to parse cached token file %s: %v", tokenFile, err)
		return nil
	}

	return &token
}

// saveCachedToken persists the token to disk (with 0600 permissions) for reuse across restarts
func (dfa *DeviceFlowAuth) saveCachedToken(token *oauth2.Token) {
	tokenFile := dfa.getTokenFilePath()
	if tokenFile == "" {
		return
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		log.Printf("Warning: failed to serialize token for caching: %v", err)
		return
	}

	dir := filepath.Dir(tokenFile)
	if err := os.MkdirAll(dir, 0700); err != nil {
		log.Printf("Warning: failed to create token cache directory %s: %v", dir, err)
		return
	}

	if err := os.WriteFile(tokenFile, data, 0600); err != nil {
		log.Printf("Warning: failed to write cached token to %s: %v", tokenFile, err)
	}
}

// getTokenFilePath returns the resolved path for the token cache file.
// Falls back to ~/.pikafileservice/oauth2_token.json if not configured.
func (dfa *DeviceFlowAuth) getTokenFilePath() string {
	if dfa.config.TokenFile != "" {
		return dfa.config.TokenFile
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".pikafileservice", "oauth2_token.json")
}
