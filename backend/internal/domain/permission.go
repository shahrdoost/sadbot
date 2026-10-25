package domain

const (
	PermBotRead    = "bot:read"
	PermBotUsers   = "bot:users"
	PermBotConfig  = "bot:config"
	PermBotFinance = "bot:finance"
	PermBotRuntime = "bot:runtime"
)

var AllBotPermissions = []string{PermBotRead, PermBotUsers, PermBotConfig, PermBotFinance, PermBotRuntime}

func ValidPermission(p string) bool {
	for _, v := range AllBotPermissions {
		if v == p {
			return true
		}
	}
	return false
}
