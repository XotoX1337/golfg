// Package config loads application configuration from a TOML file and/or
// environment variables (viper), applies sensible defaults, and exposes a
// typed Config. Nothing firm-specific is hardcoded; everything is overridable
// so the public repo can be run by anyone without committing secrets.
package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// Application metadata. Not part of the user-editable config file on purpose —
// these identify the binary/service itself.
const (
	ServiceName = "golfg"
	DisplayName = "go LFG"
	Description = "GoLang Looking For Group Tool"
	Author      = "Frederic Leist"
	Version     = "v1.0.1"
)

// ConfigFileName is the base name (without extension) of the runtime config file.
const ConfigFileName = ServiceName // golfg(.toml)

// DefaultAccentColor is the accent used when none is configured. A deep teal:
// distinctive, theme-neutral and high-contrast against white button text.
const DefaultAccentColor = "#0f766e"

// Config is the fully-resolved application configuration.
type Config struct {
	App      AppConfig      `mapstructure:"app"`
	Auth     AuthConfig     `mapstructure:"auth"`
	Teams    TeamsConfig    `mapstructure:"teams"`
	Session  SessionConfig  `mapstructure:"session"`
	Branding BrandingConfig `mapstructure:"branding"`

	// Resolved runtime paths (not read from the file).
	DataDir    string `mapstructure:"-"`
	ConfigFile string `mapstructure:"-"`
	LogFile    string `mapstructure:"-"`
	DBFile     string `mapstructure:"-"`
}

type AppConfig struct {
	Host    string `mapstructure:"host"`
	Port    int    `mapstructure:"port"`
	BaseURL string `mapstructure:"base_url"`
}

// AuthConfig holds Entra/OIDC settings. Empty = dev mode without SSO.
type AuthConfig struct {
	TenantID     string `mapstructure:"tenant_id"`
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	// FetchPhotos, when true (default), requests the delegated Graph permission
	// User.Read at login and best-effort caches the user's own M365 profile photo
	// for display. Set false to keep logins to the bare OIDC scopes and never call
	// Graph. No effect in dev mode (no Graph token). Default true.
	FetchPhotos bool `mapstructure:"fetch_photos"`
}

// TeamsConfig holds the Power-Automate webhook. Empty = posts are only logged.
type TeamsConfig struct {
	WebhookURL string `mapstructure:"webhook_url"`
	// Lang is the language code (e.g. "en", "de") used for the channel
	// notifications. The Teams channel has no per-request locale, so the
	// language is fixed by config. Empty falls back to the i18n default.
	Lang string `mapstructure:"lang"`
	// MentionPlayers, when true, @-mentions the drawn players in the "teams are
	// set" post so they get a real Teams notification. Only members with an
	// Entra object id (SSO login) can be mentioned; dev-login users render as a
	// plain name. Default true. Disable with mention_players = false.
	MentionPlayers bool `mapstructure:"mention_players"`
}

// BrandingConfig holds white-label UI settings so the public repo carries no
// firm-specific name or color. All values are safe to expose (no secrets).
type BrandingConfig struct {
	// AppName is shown in the header, page title and footer. Empty = DisplayName.
	AppName string `mapstructure:"app_name"`
	// AccentColor is any CSS color (hex recommended). It drives every accent
	// surface; hover/focus shades are derived from it in CSS. Empty = default.
	AccentColor string `mapstructure:"accent_color"`
	// PlayCTA overrides the "I want to play" button label with a fixed literal
	// (e.g. "⚽ Ich will kickern"). It is NOT translated — same trade-off as
	// AppName. Empty = the localized session_start string.
	PlayCTA string `mapstructure:"play_cta"`
	// PlayAnnouncement overrides the Teams "session started" headline. It is a
	// text/template with a single {{.Name}} placeholder (the creator's name),
	// e.g. "{{.Name}} will kickern!". It is NOT translated. Empty = the localized
	// teams_notify_started_title (which also carries the activity name).
	PlayAnnouncement string `mapstructure:"play_announcement"`
}

type SessionConfig struct {
	ExpireMinutes int `mapstructure:"expire_minutes"`
	// CookieSecure marks the auth session cookie as Secure (HTTPS-only). Keep
	// false for local http dev; set true in production behind HTTPS.
	CookieSecure bool `mapstructure:"cookie_secure"`
}

// AuthEnabled reports whether SSO is configured. When false the app runs in
// dev mode (no SSO) so the project stays usable without an Entra tenant.
func (c *Config) AuthEnabled() bool {
	return c.Auth.TenantID != "" && c.Auth.ClientID != "" && c.Auth.ClientSecret != ""
}

// TeamsEnabled reports whether a Teams webhook is configured. When false,
// Teams posts are only logged (graceful degradation).
func (c *Config) TeamsEnabled() bool {
	return c.Teams.WebhookURL != ""
}

// AppName returns the configured branding name, falling back to DisplayName.
func (c *Config) AppName() string {
	if c.Branding.AppName != "" {
		return c.Branding.AppName
	}
	return DisplayName
}

// AccentColor returns the configured accent color, falling back to the default.
func (c *Config) AccentColor() string {
	if c.Branding.AccentColor != "" {
		return c.Branding.AccentColor
	}
	return DefaultAccentColor
}

// PlayCTA returns the configured custom "I want to play" button label, or "" to
// signal the caller should fall back to the localized session_start string.
func (c *Config) PlayCTA() string {
	return c.Branding.PlayCTA
}

// PlayAnnouncement returns the configured custom Teams "session started" title
// template ({{.Name}} placeholder), or "" to fall back to the localized default.
func (c *Config) PlayAnnouncement() string {
	return c.Branding.PlayAnnouncement
}

// Addr returns the host:port the server should bind to.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.App.Host, c.App.Port)
}

// envBindings maps each viper key to its GOLFG_<SECTION>_<KEY> env override and
// default value. Binding the env explicitly (rather than relying solely on
// AutomaticEnv) makes overrides work reliably through Unmarshal.
var envBindings = []struct {
	key, env string
	def      any
}{
	{"app.host", "GOLFG_APP_HOST", "0.0.0.0"},
	{"app.port", "GOLFG_APP_PORT", 9000},
	{"app.base_url", "GOLFG_APP_BASE_URL", "http://localhost:9000"},
	{"branding.app_name", "GOLFG_BRANDING_APP_NAME", DisplayName},
	{"branding.accent_color", "GOLFG_BRANDING_ACCENT_COLOR", DefaultAccentColor},
	{"branding.play_cta", "GOLFG_BRANDING_PLAY_CTA", ""},
	{"branding.play_announcement", "GOLFG_BRANDING_PLAY_ANNOUNCEMENT", ""},
	{"auth.tenant_id", "GOLFG_AUTH_TENANT_ID", ""},
	{"auth.client_id", "GOLFG_AUTH_CLIENT_ID", ""},
	{"auth.client_secret", "GOLFG_AUTH_CLIENT_SECRET", ""},
	{"auth.fetch_photos", "GOLFG_AUTH_FETCH_PHOTOS", true},
	{"teams.webhook_url", "GOLFG_TEAMS_WEBHOOK_URL", ""},
	{"teams.lang", "GOLFG_TEAMS_LANG", "en"},
	{"teams.mention_players", "GOLFG_TEAMS_MENTION_PLAYERS", true},
	{"session.expire_minutes", "GOLFG_SESSION_EXPIRE_MINUTES", 30},
	{"session.cookie_secure", "GOLFG_SESSION_COOKIE_SECURE", false},
}

// Load reads configuration for the given data directory. It searches dataDir and
// the current working directory for golfg.toml. If no file is found, a default
// one is written to dataDir so operators have something to edit. Environment
// variables (GOLFG_<SECTION>_<KEY>) always override file values.
func Load(dataDir string, logger *zap.Logger) (*Config, error) {
	v := viper.New()
	v.SetConfigName(ConfigFileName)
	v.SetConfigType("toml")
	v.AddConfigPath(dataDir)
	v.AddConfigPath(".")

	v.SetEnvPrefix("GOLFG")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	for _, b := range envBindings {
		v.SetDefault(b.key, b.def)
		if err := v.BindEnv(b.key, b.env); err != nil {
			return nil, fmt.Errorf("bind env %s: %w", b.env, err)
		}
	}

	configFile := filepath.Join(dataDir, ConfigFileName+".toml")
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if werr := v.WriteConfigAs(configFile); werr != nil {
				logger.Warn("could not write default config", zap.Error(werr))
			} else {
				logger.Info("wrote default config", zap.String("path", configFile))
			}
		} else {
			return nil, fmt.Errorf("read config: %w", err)
		}
	} else {
		configFile = v.ConfigFileUsed()
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.DataDir = dataDir
	cfg.ConfigFile = configFile
	cfg.LogFile = filepath.Join(dataDir, ServiceName+".log")
	cfg.DBFile = filepath.Join(dataDir, ServiceName+".db")

	cfg.applyFallbacks(logger)
	return &cfg, nil
}

// applyFallbacks guards against obviously-invalid values.
func (c *Config) applyFallbacks(logger *zap.Logger) {
	if c.App.Host == "" {
		c.App.Host = "0.0.0.0"
	}
	if c.App.Port <= 0 || c.App.Port > 65535 {
		logger.Warn("invalid app.port, using default", zap.Int("port", c.App.Port))
		c.App.Port = 9000
	}
	if c.Session.ExpireMinutes <= 0 {
		c.Session.ExpireMinutes = 30
	}
}
