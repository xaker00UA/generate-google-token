package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xaker00UA/generate-google-token/internal/googleauth"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

var version = "dev"

// Execute runs the command-line application.
func Execute(ctx context.Context) error {
	return newRootCommand().ExecuteContext(ctx)
}

func newRootCommand() *cobra.Command {
	v := viper.New()

	root := &cobra.Command{
		Use:           "generate-google-cred",
		Short:         "Generate Google OAuth user credentials",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			return initializeConfig(cmd, v)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			settings, err := settingsFromViper(v)
			if err != nil {
				return err
			}

			return googleauth.Generate(cmd.Context(), settings, cmd.OutOrStdout())
		},
	}

	flags := root.Flags()
	flags.String("config", "", "YAML configuration file")
	flags.String("client-secret", "client_secret.json", "Google OAuth client secret JSON")
	flags.String("token-output", "token.json", "destination for generated token JSON")
	flags.String("scopes-file", "scopes.txt", "file containing one OAuth scope per line")
	flags.StringSlice("scope", nil, "additional OAuth scope (repeat flag or use comma-separated values)")
	flags.String("redirect-url", "http://localhost:8080/callback", "OAuth redirect URL registered in Google Cloud")
	flags.String("listen-address", "127.0.0.1:8080", "local callback server address")
	flags.Bool("open-browser", true, "open the authorization URL in the default browser")
	flags.Bool("force-consent", true, "force the consent screen to obtain a refresh token")
	flags.Duration("timeout", 5*time.Minute, "maximum time to wait for OAuth callback")

	return root
}

func initializeConfig(cmd *cobra.Command, v *viper.Viper) error {
	v.SetEnvPrefix("GGC")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	var bindErr error
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if bindErr == nil {
			key := flag.Name
			if flag.Name == "scope" {
				key = "scopes"
			}
			bindErr = v.BindPFlag(key, flag)
		}
	})
	if bindErr != nil {
		return fmt.Errorf("bind flags: %w", bindErr)
	}

	if configFile := v.GetString("config"); configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("read config %q: %w", configFile, err)
		}
		return nil
	}

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return fmt.Errorf("read config: %w", err)
		}
	}

	return nil
}

func settingsFromViper(v *viper.Viper) (googleauth.Settings, error) {
	settings := googleauth.Settings{
		ClientSecretPath: v.GetString("client-secret"),
		TokenOutputPath:  v.GetString("token-output"),
		ScopesFilePath:   v.GetString("scopes-file"),
		Scopes:           v.GetStringSlice("scopes"),
		RedirectURL:      v.GetString("redirect-url"),
		ListenAddress:    v.GetString("listen-address"),
		OpenBrowser:      v.GetBool("open-browser"),
		ForceConsent:     v.GetBool("force-consent"),
		Timeout:          v.GetDuration("timeout"),
	}

	if settings.ClientSecretPath == "" {
		return googleauth.Settings{}, errors.New("client-secret must not be empty")
	}
	if settings.TokenOutputPath == "" {
		return googleauth.Settings{}, errors.New("token-output must not be empty")
	}
	if settings.ListenAddress == "" {
		return googleauth.Settings{}, errors.New("listen-address must not be empty")
	}
	if settings.Timeout <= 0 {
		return googleauth.Settings{}, errors.New("timeout must be greater than zero")
	}

	return settings, nil
}
