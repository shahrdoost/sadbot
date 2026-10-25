// Package content implements the "content" bot template: collect news from a
// Telegram folder (via the MTProto sidecar account), clean it with an LLM,
// render branded PNG cards and publish them to the bot's destination channel.
//
// The behavioral reference is sadcontent (Python, on the Mac): filters, AI
// prompts, NO_CONTENT handling, card summary and render-failure fallback are
// ports of that pipeline. Instagram publishing is intentionally out of scope.
package content

import (
	"encoding/json"

	"sadbot/internal/domain"
)

// Template is the engine.Template for content bots. Content bots have no
// interactive menus — behavior lives in the Collector background loop; the
// template exists so bots can be created with template_key "content" and so
// texts/prices (prompts, brand, source folder, destination) are per-bot
// editable through the panel config.
type Template struct{}

func (t *Template) Key() string { return domain.TemplateContent }

func (t *Template) DefaultTexts() []byte {
	return []byte(`{
  "clean_prompt": ` + jsonString(CleanSystem) + `,
  "card_prompt": ` + jsonString(CardSystem) + `,
  "unknown_message": "این بات یک بات محتواست و به پیام مستقیم پاسخ نمی‌دهد."
}`)
}

func (t *Template) DefaultMenus() []byte { return []byte(`{}`) }

// DefaultPrices — the per-bot content config. All keys are editable via the
// panel Config tab (raw JSON) or Settings form.
func (t *Template) DefaultPrices() []byte {
	return []byte(`{
  "content_account_id": "",
  "content_folder": "Crypto",
  "content_channel": "",
  "content_poll_minutes": 2,
  "content_backfill_limit": 20,
  "content_min_length": 40,
  "content_skip_forwarded": false,
  "content_skip_replies": false,
  "content_keywords": [],
  "content_exclude": [],
  "content_publish_via": "bot",
  "content_text_only": false,
  "render_card_summary": true,
  "brand_key": "brand",
  "brand_name_en": "my.brand",
  "brand_name_fa": "برند من",
  "brand_handle": "@my.brand",
  "brand_glow_a": "rgba(239, 68, 68, 0.28)",
  "brand_glow_b": "rgba(249, 115, 22, 0.20)",
  "brand_glow_c": "rgba(251, 146, 60, 0.12)",
  "brand_accent": "#FFA070"
}`)
}

// jsonString escapes s as a JSON string literal (for embedding prompts).
func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}
