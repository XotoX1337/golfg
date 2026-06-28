// Package i18n provides request-scoped UI translations backed by go-i18n, with
// TOML message catalogs embedded per language. English is the default and
// fallback locale; strings missing from another locale fall back to English.
package i18n

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	toml "github.com/pelletier/go-toml/v2"
	"golang.org/x/text/language"
)

// DefaultLang is the source and fallback language code.
const DefaultLang = "en"

// supported lists the languages with a catalog; the first entry is the default.
var supported = []language.Tag{language.English, language.German}

// Bundle holds the loaded message catalogs and a matcher over the supported
// languages. It is safe for concurrent use and built once at startup.
type Bundle struct {
	inner   *i18n.Bundle
	matcher language.Matcher
}

// New loads every locales/*.toml catalog into a bundle.
func New() (*Bundle, error) {
	b := i18n.NewBundle(language.English)
	b.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if _, err := b.LoadMessageFileFS(localesFS, "locales/"+e.Name()); err != nil {
			return nil, err
		}
	}
	return &Bundle{inner: b, matcher: language.NewMatcher(supported)}, nil
}

// Localizer is a request-scoped translator bound to a single resolved language.
type Localizer struct {
	loc *i18n.Localizer
	// Lang is the resolved base language code (e.g. "en", "de"), suitable for
	// the <html lang> attribute.
	Lang string
}

// Localizer resolves the best supported language from the given preferences
// (e.g. an Accept-Language header value, or an explicit "de") and returns a
// translator for it. Empty or unrecognized preferences yield the default.
func (b *Bundle) Localizer(prefs ...string) *Localizer {
	tag, _ := language.MatchStrings(b.matcher, prefs...)
	lang := DefaultLang
	if base, conf := tag.Base(); conf != language.No {
		lang = base.String()
	}
	return &Localizer{
		loc:  i18n.NewLocalizer(b.inner, append([]string{lang}, prefs...)...),
		Lang: lang,
	}
}

// T translates a message by ID. Optional key/value pairs supply template data
// for messages with placeholders, e.g. T("lobby_open", "Count", 1, "Required", 2).
// A missing translation falls back to the message ID so gaps stay visible.
func (l *Localizer) T(id string, pairs ...any) string {
	cfg := &i18n.LocalizeConfig{MessageID: id}
	if len(pairs) > 0 {
		data := make(map[string]any, len(pairs)/2)
		for i := 0; i+1 < len(pairs); i += 2 {
			if key, ok := pairs[i].(string); ok {
				data[key] = pairs[i+1]
			}
		}
		cfg.TemplateData = data
	}
	s, err := l.loc.Localize(cfg)
	if err != nil {
		return id
	}
	return s
}
