package engine

import "sadbot/internal/telegram"

// Template defines a bot behavior plugin. Bots are instances of a template
// whose texts/menus are overridable per-bot in bot_configs.
type Template interface {
	Key() string
	DefaultTexts() []byte // JSON
	DefaultMenus() []byte // JSON
}

// Priced lets a template ship default coin costs / payment packages (JSON),
// overridable per-bot via bot_configs.prices.
type Priced interface {
	DefaultPrices() []byte
}

// Starter lets a template take over /start (and deep-link payloads).
type Starter interface {
	Start(bc *BotContext, payload string) bool
}

// MessageHandler lets a template intercept free text before menu lookup.
type MessageHandler interface {
	HandleMessage(bc *BotContext, text string) bool
}

// MediaHandler lets a template consume non-text messages (photos for avatars).
type MediaHandler interface {
	HandleMedia(bc *BotContext, msg *telegram.Message) bool
}

// ActionHandler lets a template handle act:* keys itself.
type ActionHandler interface {
	HandleAction(bc *BotContext, action string) bool
}

// Registry holds all built-in templates.
type Registry struct {
	templates map[string]Template
}

func NewRegistry() *Registry {
	return &Registry{templates: map[string]Template{}}
}

func (r *Registry) Register(t Template) {
	r.templates[t.Key()] = t
}

func (r *Registry) Get(key string) (Template, bool) {
	t, ok := r.templates[key]
	return t, ok
}

func (r *Registry) Keys() []string {
	out := make([]string, 0, len(r.templates))
	for k := range r.templates {
		out = append(out, k)
	}
	return out
}
