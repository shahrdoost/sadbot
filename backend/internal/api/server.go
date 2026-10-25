package api

import (
	"net/http"

	"sadbot/internal/api/handlers"
	mw "sadbot/internal/api/middleware"
	"sadbot/internal/domain"
	"sadbot/internal/service"
	"sadbot/pkg/respond"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type Deps struct {
	Auth     *service.AuthService
	Admins   *service.AdminService
	AuthH    *handlers.AuthHandler
	AdminsH  *handlers.AdminsHandler
	BotsH    *handlers.BotsHandler
	FinanceH *handlers.FinanceHandler
	// TgAccountsH may wrap a nil-able sidecar; routes still serve 503 when unconfigured.
	TgAccountsH *handlers.TelegramAccountsHandler
	// PaymentsH may be nil when no gateway service is configured.
	PaymentsH *handlers.PaymentsHandler
	// ModerationH serves confessions + support tickets per bot.
	ModerationH *handlers.ModerationHandler
	// PanelH serves dating user cards, broadcast, and discount codes.
	PanelH *handlers.PanelHandler
	// AdminOpsH serves per-user panel actions (coins, reset, block, stuck scan).
	AdminOpsH *handlers.AdminOpsHandler
	// ContentH serves the content-template panel (settings, folders, ledger, clone).
	ContentH *handlers.ContentHandler
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.RealIP)
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(cors)

	r.Get("/api/health", func(w http.ResponseWriter, req *http.Request) {
		respond.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Post("/api/v1/auth/login", d.AuthH.Login)

	// public: payment gateway callback (no JWT — the gateway redirects the user here)
	if d.PaymentsH != nil {
		r.Get("/api/v1/payments/callback", d.PaymentsH.Callback)
	}

	// everything below needs a valid JWT
	r.Group(func(r chi.Router) {
		r.Use(mw.NewAuthenticator(d.Auth).Handle)

		r.Get("/api/v1/auth/me", d.AuthH.Me)

		// admins — superadmin only
		r.With(mw.RequireSuper).Get("/api/v1/admins", d.AdminsH.List)
		r.With(mw.RequireSuper).Post("/api/v1/admins", d.AdminsH.Create)
		r.With(mw.RequireSuper).Patch("/api/v1/admins/{id}", d.AdminsH.Update)
		r.With(mw.RequireSuper).Delete("/api/v1/admins/{id}", d.AdminsH.Delete)
		r.With(mw.RequireSuper).Get("/api/v1/admins/{id}/bot-access", d.AdminsH.GetBotAccess)
		r.With(mw.RequireSuper).Put("/api/v1/admins/{id}/bot-access/{botID}", d.AdminsH.SetBotAccess)

		// bots
		r.Get("/api/v1/bots", d.BotsH.List)
		r.With(mw.RequireSuper).Post("/api/v1/bots", d.BotsH.Create)

		botPerm := func(perm string) func(http.Handler) http.Handler {
			return mw.RequireBotPerm(d.Admins, perm)
		}
		r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}", d.BotsH.Get)
		r.With(botPerm(domain.PermBotConfig)).Patch("/api/v1/bots/{botID}", d.BotsH.Update)
		r.With(mw.RequireSuper).Delete("/api/v1/bots/{botID}", d.BotsH.Delete)
		r.With(botPerm(domain.PermBotRuntime)).Post("/api/v1/bots/{botID}/start", d.BotsH.Start)
		r.With(botPerm(domain.PermBotRuntime)).Post("/api/v1/bots/{botID}/stop", d.BotsH.Stop)
		r.With(botPerm(domain.PermBotConfig)).Get("/api/v1/bots/{botID}/config", d.BotsH.GetConfig)
		r.With(botPerm(domain.PermBotConfig)).Put("/api/v1/bots/{botID}/config", d.BotsH.PutConfig)
		r.With(botPerm(domain.PermBotConfig)).Get("/api/v1/bots/{botID}/settings", d.BotsH.GetSettings)
		r.With(botPerm(domain.PermBotConfig)).Put("/api/v1/bots/{botID}/settings", d.BotsH.UpdateSettings)
		r.With(botPerm(domain.PermBotConfig)).Get("/api/v1/bots/{botID}/admin/channels", d.BotsH.GetChannels)
		r.With(botPerm(domain.PermBotConfig)).Put("/api/v1/bots/{botID}/admin/channels", d.BotsH.SetChannels)
		r.With(botPerm(domain.PermBotUsers)).Get("/api/v1/bots/{botID}/users", d.BotsH.Users)
		r.With(botPerm(domain.PermBotUsers)).Get("/api/v1/bots/{botID}/events", d.BotsH.Events)
		r.With(botPerm(domain.PermBotFinance)).Get("/api/v1/bots/{botID}/payments", d.BotsH.Payments)
		r.With(botPerm(domain.PermBotFinance)).Post("/api/v1/bots/{botID}/payments", d.BotsH.CreatePayment)
		r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/stats", d.BotsH.Stats)

		// content template panel: typed settings, folder/channel pickers,
		// collection ledger, and clone (superadmin, like create/delete)
		if d.ContentH != nil {
			r.With(botPerm(domain.PermBotConfig)).Get("/api/v1/bots/{botID}/content/settings", d.ContentH.GetSettings)
			r.With(botPerm(domain.PermBotConfig)).Put("/api/v1/bots/{botID}/content/settings", d.ContentH.PutSettings)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/content/folders", d.ContentH.Folders)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/content/chats", d.ContentH.Channels)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/content/posts", d.ContentH.Posts)
			r.With(mw.RequireSuper).Post("/api/v1/bots/{botID}/clone", d.ContentH.Clone)
		}

		// moderation: confessions + support tickets (bot-scoped admin)
		if d.ModerationH != nil {
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/confessions", d.ModerationH.ListConfessions)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/confessions/{id}/approve", d.ModerationH.ApproveConfession)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/confessions/{id}/reject", d.ModerationH.RejectConfession)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/tickets", d.ModerationH.ListTickets)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/tickets/{id}/reply", d.ModerationH.ReplyTicket)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/tickets/{id}/close", d.ModerationH.CloseTicket)
		}

		// panel extras: dating users, broadcast, discount codes
		if d.PanelH != nil {
			r.With(botPerm(domain.PermBotUsers)).Get("/api/v1/bots/{botID}/dating-users", d.PanelH.DatingUsers)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/broadcast/preview", d.PanelH.BroadcastPreview)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/broadcast", d.PanelH.StartBroadcast)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/broadcast/runs", d.PanelH.BroadcastRuns)
			r.With(botPerm(domain.PermBotConfig)).Get("/api/v1/bots/{botID}/discounts", d.PanelH.ListDiscounts)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/discounts", d.PanelH.CreateDiscount)
			r.With(botPerm(domain.PermBotConfig)).Delete("/api/v1/bots/{botID}/discounts/{id}", d.PanelH.DeleteDiscount)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/confessions/seed", d.PanelH.SeedConfessions)
		}

		// per-user panel actions: coins, reset, block, streak, direct message,
		// plus the stuck-user scan and bulk chat resets
		if d.AdminOpsH != nil {
			r.With(botPerm(domain.PermBotUsers)).Get("/api/v1/bots/{botID}/admin/users/{userID}", d.AdminOpsH.UserDetail)
			r.With(botPerm(domain.PermBotFinance)).Post("/api/v1/bots/{botID}/admin/users/{userID}/coins", d.AdminOpsH.AdjustCoins)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/users/{userID}/reset", d.AdminOpsH.ResetUser)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/users/{userID}/block", d.AdminOpsH.SetBlocked)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/users/{userID}/streak", d.AdminOpsH.ResetStreak)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/users/{userID}/message", d.AdminOpsH.DirectMessage)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/stuck", d.AdminOpsH.StuckUsers)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/stuck/reset", d.AdminOpsH.ResetStuck)
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/reset-all", d.AdminOpsH.ResetAllChats)
			// chat surveillance: live conversations, transcripts, global feed
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/chats", d.AdminOpsH.ActiveChats)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/chats/transcript", d.AdminOpsH.ChatTranscript)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/messages", d.AdminOpsH.RecentChatMessages)
			// AI harness: overview + per-user history + enable switch
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/ai", d.AdminOpsH.AIHarness)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/ai/users/{userID}", d.AdminOpsH.AIUserDetail)
			r.With(botPerm(domain.PermBotConfig)).Put("/api/v1/bots/{botID}/admin/ai", d.AdminOpsH.SetAIEnabled)
			// telegram-block probe + user-level block relationships
			r.With(botPerm(domain.PermBotConfig)).Post("/api/v1/bots/{botID}/admin/telegram-blocks/scan", d.AdminOpsH.ScanTelegramBlocks)
			r.With(botPerm(domain.PermBotRead)).Get("/api/v1/bots/{botID}/admin/blocks", d.AdminOpsH.BlockPairs)
		}

		// finance — superadmin only
		r.With(mw.RequireSuper).Get("/api/v1/finance/overview", d.FinanceH.Overview)
		r.With(mw.RequireSuper).Get("/api/v1/audit-logs", d.FinanceH.AuditLogs)

		// telegram accounts (MTProto user accounts, per-admin ownership)
		if d.TgAccountsH != nil {
			r.Post("/api/v1/telegram-accounts/start-login", d.TgAccountsH.StartLogin)
			r.Post("/api/v1/telegram-accounts/complete-login", d.TgAccountsH.CompleteLogin)
			r.Get("/api/v1/telegram-accounts", d.TgAccountsH.List)
			r.Delete("/api/v1/telegram-accounts/{id}", d.TgAccountsH.Delete)
			r.Post("/api/v1/telegram-accounts/{id}/create-bot", d.TgAccountsH.CreateBot)
			r.Get("/api/v1/telegram-accounts/{id}/chats", d.TgAccountsH.Chats)
			r.Get("/api/v1/telegram-accounts/{id}/chats/{chatID}/messages", d.TgAccountsH.Messages)
			r.Post("/api/v1/telegram-accounts/{id}/chats/{chatID}/messages", d.TgAccountsH.SendMessage)
			r.Get("/api/v1/telegram-accounts/{id}/chats/{chatID}/messages/{msgID}/photo", d.TgAccountsH.MessagePhoto)
			r.Get("/api/v1/telegram-accounts/{id}/chats/{chatID}/photo", d.TgAccountsH.ChatPhoto)
		}
	})

	return r
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if req.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, req)
	})
}
