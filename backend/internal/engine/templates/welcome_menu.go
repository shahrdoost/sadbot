package templates

// WelcomeMenu is the MVP sample template: /start + a small menu with
// an about scene and a settings toggle that persists per-user state.
type WelcomeMenu struct{}

func (t *WelcomeMenu) Key() string { return "welcome_menu" }

func (t *WelcomeMenu) DefaultTexts() []byte {
	return []byte(`{
		"welcome": "سلام 👋\nبه بات خوش اومدی!\n\nاز منوی پایین یکی رو انتخاب کن:",
		"about": "این یک بات نمونه در پلتفرم صدبات است.\n\nبا دکمه‌های منو جلو برید و حالت «تنظیمات» رو تست کنید — وضعیتش ذخیره می‌شود.",
		"settings.title": "⚙️ تنظیمات",
		"settings.notify_on": "🔔 اعلان‌ها: روشن",
		"settings.notify_off": "🔕 اعلان‌ها: خاموش",
		"unknown": "متوجه نشدم 🤔\n\nاز دکمه‌های منو استفاده کن."
	}`)
}

func (t *WelcomeMenu) DefaultMenus() []byte {
	return []byte(`{
		"scenes": {
			"home": {
				"text_key": "welcome",
				"rows": [
					[{"label":"📖 درباره ما","action":"scene:about"}],
					[{"label":"⚙️ تنظیمات","action":"scene:settings"}]
				]
			},
			"about": {
				"text_key": "about",
				"rows": [
					[{"label":"🔙 بازگشت","action":"scene:home"}]
				]
			},
			"settings": {
				"text_key": "settings.title",
				"dynamic_text": "notify",
				"rows": [
					[{"label":"🔄 تغییر اعلان‌ها","action":"act:settings.notify"}],
					[{"label":"🔙 بازگشت","action":"scene:home"}]
				]
			}
		}
	}`)
}
