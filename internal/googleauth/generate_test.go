package googleauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/oauth2"
)

func TestCollectScopes(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "scopes.txt")
	contents := "# comment\nhttps://example.test/one\n\n// comment\nhttps://example.test/two # inline\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	scopes, err := collectScopes(path, []string{"https://example.test/one", " openid "})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"https://example.test/one", "openid", "https://example.test/two"}
	if len(scopes) != len(want) {
		t.Fatalf("got %v, want %v", scopes, want)
	}
	for index := range want {
		if scopes[index] != want[index] {
			t.Fatalf("got %v, want %v", scopes, want)
		}
	}
}

func TestCallbackHandler(t *testing.T) {
	t.Parallel()
	resultCh := make(chan callbackResult, 1)
	handler := callbackHandler("expected-state", resultCh)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=expected-state&code=auth-code", nil)
	response := httptest.NewRecorder()
	handler(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("got status %d", response.Code)
	}
	result := <-resultCh
	if result.err != nil || result.code != "auth-code" {
		t.Fatalf("unexpected callback result: %+v", result)
	}
}

func TestCallbackHandlerRejectsWrongState(t *testing.T) {
	t.Parallel()
	resultCh := make(chan callbackResult, 1)
	handler := callbackHandler("expected-state", resultCh)

	request := httptest.NewRequest(http.MethodGet, "/callback?state=wrong&code=auth-code", nil)
	response := httptest.NewRecorder()
	handler(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("got status %d", response.Code)
	}
	if result := <-resultCh; result.err == nil {
		t.Fatal("expected state validation error")
	}
}

func TestValidateGrantedScopesUsesExactMatches(t *testing.T) {
	t.Parallel()
	token := (&oauth2.Token{AccessToken: "token"}).WithExtra(map[string]any{
		"scope": "scope.read scope.write.extra",
	})
	if err := validateGrantedScopes(token, []string{"scope.write"}); err == nil {
		t.Fatal("expected missing scope error")
	}
}

func TestWriteTokenUsesPrivatePermissions(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "nested", "token.json")
	token := &oauth2.Token{AccessToken: "access", RefreshToken: "refresh"}
	config := &oauth2.Config{ClientID: "id", ClientSecret: "secret", Scopes: []string{"scope"}}

	if err := writeToken(path, token, config); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("got permissions %o, want 600", got)
	}
}

func TestRedirectURLPathParsing(t *testing.T) {
	t.Parallel()
	parsed, err := url.Parse("http://localhost:8080/oauth/callback")
	if err != nil || parsed.Path != "/oauth/callback" {
		t.Fatalf("unexpected parsed URL: %v, %v", parsed, err)
	}
}
