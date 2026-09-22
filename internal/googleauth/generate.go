package googleauth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const tokenURI = "https://oauth2.googleapis.com/token"

// Settings controls the local OAuth authorization flow.
type Settings struct {
	ClientSecretPath string
	TokenOutputPath  string
	ScopesFilePath   string
	Scopes           []string
	RedirectURL      string
	ListenAddress    string
	OpenBrowser      bool
	ForceConsent     bool
	Timeout          time.Duration
}

type callbackResult struct {
	code string
	err  error
}

type tokenFile struct {
	Type           string   `json:"type"`
	Token          string   `json:"token"`
	RefreshToken   string   `json:"refresh_token"`
	TokenURI       string   `json:"token_uri"`
	ClientID       string   `json:"client_id"`
	ClientSecret   string   `json:"client_secret"`
	Scopes         []string `json:"scopes"`
	UniverseDomain string   `json:"universe_domain"`
	Account        string   `json:"account"`
	Expiry         string   `json:"expiry"`
}

// Generate obtains a Google OAuth user token and writes it to disk.
func Generate(ctx context.Context, settings Settings, output io.Writer) error {
	scopes, err := collectScopes(settings.ScopesFilePath, settings.Scopes)
	if err != nil {
		return err
	}
	if len(scopes) == 0 {
		return errors.New("at least one OAuth scope is required")
	}

	secretJSON, err := os.ReadFile(settings.ClientSecretPath)
	if err != nil {
		return fmt.Errorf("read client secret %q: %w", settings.ClientSecretPath, err)
	}

	config, err := google.ConfigFromJSON(secretJSON, scopes...)
	if err != nil {
		return fmt.Errorf("parse client secret %q: %w", settings.ClientSecretPath, err)
	}
	config.RedirectURL = settings.RedirectURL

	token, err := authorize(ctx, config, settings, output)
	if err != nil {
		return err
	}
	if token.RefreshToken == "" {
		return errors.New("google did not return a refresh token; revoke the application's access and retry with force-consent enabled")
	}
	if err := validateGrantedScopes(token, scopes); err != nil {
		return err
	}

	if err := writeToken(settings.TokenOutputPath, token, config); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(output, "Credentials written to %s\n", settings.TokenOutputPath)
	return nil
}

func authorize(ctx context.Context, config *oauth2.Config, settings Settings, output io.Writer) (*oauth2.Token, error) {
	redirect, err := url.Parse(config.RedirectURL)
	if err != nil {
		return nil, fmt.Errorf("parse redirect-url: %w", err)
	}
	if redirect.Scheme != "http" || redirect.Hostname() == "" || redirect.Path == "" {
		return nil, errors.New("redirect-url must be a complete local HTTP URL with a callback path")
	}

	state, err := randomState()
	if err != nil {
		return nil, fmt.Errorf("generate OAuth state: %w", err)
	}
	verifier := oauth2.GenerateVerifier()
	authOptions := []oauth2.AuthCodeOption{
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oauth2.SetAuthURLParam("include_granted_scopes", "true"),
	}
	if settings.ForceConsent {
		authOptions = append(authOptions, oauth2.SetAuthURLParam("prompt", "consent"))
	}
	authURL := config.AuthCodeURL(state, authOptions...)

	listener, err := net.Listen("tcp", settings.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", settings.ListenAddress, err)
	}

	resultCh := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc(redirect.Path, callbackHandler(state, resultCh))
	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	serverErrCh := make(chan error, 1)
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			serverErrCh <- serveErr
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	_, _ = fmt.Fprintf(output, "Open this URL to authorize the application:\n%s\n", authURL)
	if settings.OpenBrowser {
		if browserErr := openBrowser(authURL); browserErr != nil {
			_, _ = fmt.Fprintf(output, "Could not open the browser automatically: %v\n", browserErr)
		}
	}

	waitCtx, cancel := context.WithTimeout(ctx, settings.Timeout)
	defer cancel()

	var code string
	select {
	case result := <-resultCh:
		if result.err != nil {
			return nil, result.err
		}
		code = result.code
	case serveErr := <-serverErrCh:
		return nil, fmt.Errorf("callback server: %w", serveErr)
	case <-waitCtx.Done():
		return nil, fmt.Errorf("wait for OAuth callback: %w", waitCtx.Err())
	}

	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}
	return token, nil
}

func callbackHandler(expectedState string, resultCh chan<- callbackResult) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("state") != expectedState {
			http.Error(w, "Invalid OAuth state.", http.StatusBadRequest)
			select {
			case resultCh <- callbackResult{err: errors.New("OAuth callback state mismatch")}:
			default:
			}
			return
		}
		if oauthErr := query.Get("error"); oauthErr != "" {
			description := query.Get("error_description")
			http.Error(w, "Authorization was not completed.", http.StatusBadRequest)
			select {
			case resultCh <- callbackResult{err: fmt.Errorf("google authorization failed: %s: %s", oauthErr, description)}:
			default:
			}
			return
		}
		code := query.Get("code")
		if code == "" {
			http.Error(w, "Authorization code is missing.", http.StatusBadRequest)
			return
		}

		_, _ = io.WriteString(w, "Authorization succeeded. You can close this window.")
		select {
		case resultCh <- callbackResult{code: code}:
		default:
		}
	}
}

func randomState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func openBrowser(targetURL string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", targetURL)
	case "linux":
		command = exec.Command("xdg-open", targetURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	default:
		return fmt.Errorf("unsupported operating system %q", runtime.GOOS)
	}
	return command.Start()
}

func collectScopes(scopesFile string, configured []string) ([]string, error) {
	scopes := append([]string(nil), configured...)
	if scopesFile != "" {
		fromFile, err := loadScopes(scopesFile)
		if err != nil {
			return nil, fmt.Errorf("read scopes file %q: %w", scopesFile, err)
		}
		scopes = append(scopes, fromFile...)
	}

	unique := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		unique = append(unique, scope)
	}
	return unique, nil
}

func loadScopes(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	var scopes []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if index := strings.Index(line, " #"); index >= 0 {
			line = strings.TrimSpace(line[:index])
		}
		if line != "" {
			scopes = append(scopes, line)
		}
	}
	return scopes, scanner.Err()
}

func validateGrantedScopes(token *oauth2.Token, required []string) error {
	value := token.Extra("scope")
	if value == nil {
		return nil
	}
	grantedText, ok := value.(string)
	if !ok {
		return errors.New("google returned OAuth scopes in an unexpected format")
	}
	granted := strings.Fields(grantedText)
	for _, scope := range required {
		if !slices.Contains(granted, scope) {
			return fmt.Errorf("required OAuth scope was not granted: %s", scope)
		}
	}
	return nil
}

func writeToken(path string, token *oauth2.Token, config *oauth2.Config) error {
	data := tokenFile{
		Type:           "authorized_user",
		Token:          token.AccessToken,
		RefreshToken:   token.RefreshToken,
		TokenURI:       tokenURI,
		ClientID:       config.ClientID,
		ClientSecret:   config.ClientSecret,
		Scopes:         config.Scopes,
		UniverseDomain: "googleapis.com",
		Account:        "",
		Expiry:         token.Expiry.UTC().Format(time.RFC3339),
	}
	encoded, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("encode token: %w", err)
	}
	encoded = append(encoded, '\n')

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".google-token-*")
	if err != nil {
		return fmt.Errorf("create temporary token file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()

	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary token file: %w", err)
	}
	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write token: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close token file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace token file: %w", err)
	}
	return nil
}
