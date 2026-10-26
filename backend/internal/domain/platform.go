package domain

// Platform identifies which Bot-API endpoint an identity or bot token
// belongs to. Users are separate per platform; bots may serve several.
const (
	PlatformTelegram = "telegram"
	PlatformKat      = "kat"
)

// ValidPlatform reports whether p is a known platform key.
func ValidPlatform(p string) bool {
	return p == PlatformTelegram || p == PlatformKat
}
