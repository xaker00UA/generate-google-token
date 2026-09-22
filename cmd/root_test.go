package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestSettingsFromViper(t *testing.T) {
	t.Parallel()
	v := viper.New()
	v.Set("client-secret", "client.json")
	v.Set("token-output", "token.json")
	v.Set("scopes-file", "")
	v.Set("scopes", []string{"openid"})
	v.Set("redirect-url", "http://localhost:9090/callback")
	v.Set("listen-address", "127.0.0.1:9090")
	v.Set("open-browser", false)
	v.Set("force-consent", true)
	v.Set("timeout", "1m")

	settings, err := settingsFromViper(v)
	if err != nil {
		t.Fatal(err)
	}
	if len(settings.Scopes) != 1 || settings.Scopes[0] != "openid" {
		t.Fatalf("unexpected scopes: %v", settings.Scopes)
	}
	if settings.OpenBrowser {
		t.Fatal("open-browser should be false")
	}
}

func TestConfigFileAndFlagPrecedence(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "settings.yaml")
	config := []byte("client-secret: from-config.json\nscopes-file: \"\"\nscopes:\n  - openid\n")
	if err := os.WriteFile(configPath, config, 0o600); err != nil {
		t.Fatal(err)
	}

	command := newRootCommand()
	if err := command.ParseFlags([]string{"--config", configPath, "--client-secret", "from-flag.json"}); err != nil {
		t.Fatal(err)
	}

	v := viper.New()
	if err := initializeConfig(command, v); err != nil {
		t.Fatal(err)
	}
	if got := v.GetString("client-secret"); got != "from-flag.json" {
		t.Fatalf("got client-secret %q", got)
	}
}
