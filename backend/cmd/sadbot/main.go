package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"sadbot/internal/api"
	"sadbot/internal/api/handlers"
	anonychatpkg "sadbot/internal/anonychat"
	bomblandpkg "sadbot/internal/bombland"
	contentpkg "sadbot/internal/content"
	"sadbot/internal/config"
	datingpkg "sadbot/internal/dating"
	"sadbot/internal/domain"
	duelpkg "sadbot/internal/duel"
	"sadbot/internal/engine"
	"sadbot/internal/engine/templates"
	faalpkg "sadbot/internal/faal"
	favaanipkg "sadbot/internal/favaani"
	"sadbot/internal/llm"
	"sadbot/internal/payment"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/runtime"
	"sadbot/internal/service"
	shlinkspkg "sadbot/internal/shlinks"
	tapearnpkg "sadbot/internal/tapearn"
	whoseemepkg "sadbot/internal/whoseeme"
	"sadbot/internal/telegram"
	"sadbot/internal/tgsidecar"
	pgcrypto "sadbot/pkg/crypto"

	"github.com/google/uuid"
)

func main() {
	seedOnly := flag.Bool("seed-only", false, "create the seed superadmin and exit")
	flag.Parse()

	config.LoadDotEnv(".env")
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := postgres.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	// repos
	adminRepo := postgres.NewAdminRepo(pool)
	botRepo := postgres.NewBotRepo(pool)
	configRepo := postgres.NewConfigRepo(pool)
	userRepo := postgres.NewBotUserRepo(pool)
	eventRepo := postgres.NewEventRepo(pool)
	paymentRepo := postgres.NewPaymentRepo(pool)
	financeRepo := postgres.NewFinanceRepo(pool)
	auditRepo := postgres.NewAuditRepo(pool)

	// services
	authSvc := service.NewAuthService(adminRepo, cfg.JWTSecret)
	adminSvc := service.NewAdminService(adminRepo, auditRepo)

	// seed admin
	if cfg.SeedAdminEmail != "" && cfg.SeedAdminPassword != "" {
		if n, _ := adminRepo.Count(ctx); n == 0 {
			hash, err := service.HashPassword(cfg.SeedAdminPassword)
			if err != nil {
				log.Fatalf("seed password: %v", err)
			}
			if _, err := adminRepo.Create(ctx, cfg.SeedAdminEmail, hash, "مدیر اصلی", domain.RoleSuperAdmin); err != nil {
				log.Fatalf("seed admin: %v", err)
			}
			log.Printf("seeded superadmin %s", cfg.SeedAdminEmail)
		}
	}
	if *seedOnly {
		return
	}

	// engine + runtime
	registry := engine.NewRegistry()
	registry.Register(&templates.WelcomeMenu{})
	datingTemplate := &datingpkg.Template{}
	registry.Register(datingTemplate)
	contentTemplate := &contentpkg.Template{}
	registry.Register(contentTemplate)
	bomblandTemplate := &bomblandpkg.Template{}
	registry.Register(bomblandTemplate)
	duelTemplate := &duelpkg.Template{}
	registry.Register(duelTemplate)
	favaaniTemplate := &favaanipkg.Template{}
	registry.Register(favaaniTemplate)
	shlinksTemplate := &shlinkspkg.Template{}
	registry.Register(shlinksTemplate)
	whoseemeTemplate := &whoseemepkg.Template{}
	registry.Register(whoseemeTemplate)
	faalTemplate := &faalpkg.Template{}
	registry.Register(faalTemplate)
	anonychatTemplate := &anonychatpkg.Template{}
	registry.Register(anonychatTemplate)
	tapearnTemplate := &tapearnpkg.Template{}
	registry.Register(tapearnTemplate)

	userStore := runtime.NewUserStoreAdapter(userRepo)
	eventStore := runtime.NewEventStoreAdapter(eventRepo)
	configStore := runtime.NewConfigStoreAdapter(configRepo)
	eng := engine.New(registry, userStore, eventStore, configStore)

	datingRepo := postgres.NewDatingRepo(pool)
	datingStore := datingpkg.NewStore(datingRepo, userRepo)
	eng.SetData("dating", datingStore)

	bomblandRepo := postgres.NewBombLandRepo(pool)
	eng.SetData("bombland", bomblandRepo)
	duelRepo := postgres.NewDuelRepo(pool)
	eng.SetData("duel", duelRepo)

	favaaniRepo := postgres.NewFavaaniRepo(pool)
	eng.SetData("favaani", favaaniRepo)
	shlinksRepo := postgres.NewShlinksRepo(pool)
	eng.SetData("shlinks", shlinksRepo)
	whoseemeRepo := postgres.NewWhoseemeRepo(pool)
	eng.SetData("whoseeme", whoseemeRepo)
	faalRepo := postgres.NewFaalRepo(pool)
	eng.SetData("faal", faalRepo)
	anonychatRepo := postgres.NewAnonyChatRepo(pool)
	eng.SetData("anonychat", anonychatRepo)
	tapearnRepo := postgres.NewTapEarnRepo(pool)
	eng.SetData("tapearn", tapearnRepo)

	// per-bot merged prices for services outside the engine (matcher ghosts,
	// notification sweep, moderation channel posts)
	pricesOf := func(botID uuid.UUID) map[string]any {
		_, _, prices, err := configStore.GetMerged(ctx, botID.String(), datingTemplate)
		if err != nil {
			return map[string]any{}
		}
		return prices
	}
	eng.SetData("pricesof", func(botID string) map[string]any {
		id, err := uuid.Parse(botID)
		if err != nil {
			return map[string]any{}
		}
		return pricesOf(id)
	})

	// LLM companion (disabled without LLM_API_KEY; scripted fallback in template)
	llmClient := llm.NewFromEnv()
	eng.SetData("llm", llmClient)

	moderationRepo := postgres.NewModerationRepo(pool)
	eng.SetData("moderationRepo", moderationRepo)

	runtimeMgr := runtime.NewManager(eng, botRepo, userRepo, botRepo)
	runtime.ConfigureTokenDecryptor(func(b *domain.Bot) (string, error) {
		return pgcrypto.DecryptToken(cfg.TokenEncryptionKey, b.TokenEncrypted)
	})

	// global matcher (5s tick + search timeout sweep + ghost injection)
	matcher := datingpkg.NewMatcher(datingStore, botRepo,
		func(botID uuid.UUID, chatID int64, text string, kbd json.RawMessage) error {
			client, ok := runtimeMgr.ClientFor(botID)
			if !ok {
				return nil // bot not running; notifications skipped
			}
			_, err := client.SendMessage(ctx, telegram.SendMessagePayload{
				ChatID: chatID, Text: text, ReplyMarkup: kbd,
			})
			return err
		})
	matcher.SetPricesFunc(pricesOf)
	textsOf := func(botID uuid.UUID) map[string]any {
		texts, _, _, err := configStore.GetMerged(ctx, botID.String(), datingTemplate)
		if err != nil {
			return map[string]any{}
		}
		return texts
	}
	matcher.SetTextsFunc(textsOf)
	go matcher.Run(ctx)

	// re-engagement notifications (30m tick, hard caps 2/day + 8h gap + night quiet)
	go datingpkg.NewNotificationService(datingStore, botRepo,
		func(botID uuid.UUID, chatID int64, text string, kbd json.RawMessage) error {
			client, ok := runtimeMgr.ClientFor(botID)
			if !ok {
				return nil
			}
			_, err := client.SendMessage(ctx, telegram.SendMessagePayload{
				ChatID: chatID, Text: text, ReplyMarkup: kbd,
			})
			return err
		}, pricesOf).Run(ctx)

	// payment economy: gateway + service + auto-verify sweeper
	gateway := payment.NewGateway()
	notifier := &runtimeNotifier{users: userRepo, mgr: runtimeMgr}
	gameCreditor := payment.NewGameCreditor(pool, datingRepo)
	paySvc := payment.NewService(paymentRepo, gameCreditor, gateway, notifier, cfg.PaymentCallbackURL)
	go payment.NewSweeper(paySvc).Run(ctx)
	eng.SetData("payments", paySvc)
	eng.SetData("paymentRepo", paymentRepo)
	eng.SetData("channelcheck", func(botID uuid.UUID, channelID string, userID int64) (string, error) {
		client, ok := runtimeMgr.ClientFor(botID)
		if !ok {
			return "", nil
		}
		// numeric id or @username — Bot API accepts both
		return client.GetChatMemberStatus(ctx, strings.TrimSpace(channelID), userID)
	})
	eng.SetData("botusername", func(botID uuid.UUID) string {
		b, err := botRepo.GetByID(ctx, botID)
		if err != nil || b == nil || b.TelegramUsername == nil {
			return ""
		}
		return strings.TrimPrefix(*b.TelegramUsername, "@")
	})

	// panel extras: discount codes (bot UX) + broadcast service
	discountRepo := postgres.NewDiscountRepo(pool)
	eng.SetData("discountRepo", discountRepo)
	broadcastRepo := postgres.NewBroadcastRepo(pool)
	broadcastSvc := service.NewBroadcastService(broadcastRepo,
		func(c context.Context, botID uuid.UUID, chatID int64, text, photo, footer string) error {
			client, ok := runtimeMgr.ClientFor(botID)
			if !ok {
				return nil
			}
			if photo != "" {
				caption := text
				if footer != "" {
					caption = text + "\n\n" + footer
				}
				_, err := client.SendPhoto(c, telegram.SendPhotoPayload{ChatID: chatID, Photo: photo, Caption: caption})
				return err
			}
			_, err := client.SendMessage(c, telegram.SendMessagePayload{ChatID: chatID, Text: text})
			return err
		})

	botSvc := service.NewBotService(botRepo, configRepo, registry, service.NewTelegramValidator(), runtimeMgr, auditRepo, adminRepo, cfg.TokenEncryptionKey)

	// telegram accounts (MTProto via sidecar; nil client = feature disabled)
	var tgSidecar *tgsidecar.Client
	if cfg.TGSidecarURL != "" {
		tgSidecar = tgsidecar.New(cfg.TGSidecarURL, cfg.TGSidecarKey)
	}
	tgAccountRepo := postgres.NewTelegramAccountRepo(pool)
	tgAccountSvc := service.NewTelegramAccountService(tgAccountRepo, tgSidecar, auditRepo, botSvc)

	// content template: collect news from a Telegram folder (MTProto sidecar),
	// LLM-clean, render branded cards (renderer microservice) and publish to
	// the bot's destination channel
	contentRepo := postgres.NewContentRepo(pool)
	contentPricesOf := func(botID uuid.UUID) map[string]any {
		_, _, prices, err := configStore.GetMerged(ctx, botID.String(), contentTemplate)
		if err != nil {
			return map[string]any{}
		}
		return prices
	}
	contentTextsOf := func(botID uuid.UUID) map[string]any {
		texts, _, _, err := configStore.GetMerged(ctx, botID.String(), contentTemplate)
		if err != nil {
			return map[string]any{}
		}
		return texts
	}
	contentPublisher := func(c context.Context, botID uuid.UUID, channel, caption string, photos [][]byte, filenameBase string) error {
		client, ok := runtimeMgr.ClientFor(botID)
		if !ok {
			return errors.New("bot not running")
		}
		if len(photos) == 0 {
			_, err := client.SendMessage(c, telegram.SendMessagePayload{ChatID: channel, Text: caption})
			return err
		}
		for i, png := range photos {
			if i > 0 {
				time.Sleep(time.Second) // pacing between card uploads
			}
			name := filenameBase + ".png"
			if _, err := client.SendPhotoUpload(c, channel, name, png, caption); err != nil {
				return err
			}
		}
		return nil
	}
	collector := contentpkg.NewCollector(contentRepo, botRepo, tgSidecar, llmClient,
		contentpkg.NewRenderer(cfg.RendererURL), contentPublisher)
	collector.SetPricesFunc(contentPricesOf)
	collector.SetTextsFunc(contentTextsOf)
	collector.SetPublisherViaMeta(func(c context.Context, botID uuid.UUID, accountID, channel, caption string, photos [][]byte, filenameBase string) error {
		if tgSidecar == nil {
			return errors.New("sidecar unmapped: cannot publish via account")
		}
		if len(photos) == 0 {
			// text-only fallback: MTProto send_message does not serve caption restarts
			_, err := tgSidecar.SendMessage(c, accountID, channel, caption)
			return err
		}
		for i, png := range photos {
			if i > 0 {
				time.Sleep(time.Second) // pacing between card uploads
			}
			if _, err := tgSidecar.SendPhotoMeta(c, accountID, channel,
				base64.StdEncoding.EncodeToString(png), filenameBase+".png", caption); err != nil {
				return err
			}
		}
		return nil
	})
	go collector.Run(ctx)

	// handlers
	authH := handlers.NewAuthHandler(authSvc)
	adminsH := handlers.NewAdminsHandler(adminSvc)
	botsH := handlers.NewBotsHandler(botSvc, adminSvc, userRepo, eventRepo, paymentRepo, financeRepo)
	financeH := handlers.NewFinanceHandler(financeRepo, auditRepo)
	tgAccountsH := handlers.NewTelegramAccountsHandler(tgAccountSvc)
	paymentsH := handlers.NewPaymentsHandler(paySvc)

	// moderation (confessions + support tickets)
	sendToUser := func(c context.Context, botID uuid.UUID, userID int64, text string) error {
		u, err := userRepo.GetByID(c, botID, userID)
		if err != nil || u == nil {
			return err
		}
		client, ok := runtimeMgr.ClientFor(botID)
		if !ok {
			return nil
		}
		_, err = client.SendMessage(c, telegram.SendMessagePayload{ChatID: u.TelegramID, Text: text})
		return err
	}
	postToChannel := func(c context.Context, botID uuid.UUID, text string) (int64, error) {
		channelID := ""
		if v, ok := pricesOf(botID)["channel_id"].(string); ok {
			channelID = strings.TrimSpace(v)
		}
		if channelID == "" {
			return 0, nil
		}
		client, ok := runtimeMgr.ClientFor(botID)
		if !ok {
			return 0, nil
		}
		chatID := any(channelID) // "@username" or numeric string
		if n, err := strconv.ParseInt(channelID, 10, 64); err == nil {
			chatID = n
		}
		msg, err := client.SendMessage(c, telegram.SendMessagePayload{ChatID: chatID, Text: text})
		if err != nil {
			return 0, err
		}
		return msg.MessageID, nil
	}
	moderationSvc := service.NewModerationService(moderationRepo, sendToUser, postToChannel)
	moderationH := handlers.NewModerationHandler(moderationSvc)
	panelH := handlers.NewPanelHandler(datingRepo, broadcastSvc, discountRepo, userRepo, moderationRepo)

	// panel-driven per-user admin actions (coins, reset, block, stuck scan)
	adminOpsSvc := service.NewAdminOpsService(datingRepo, userRepo, auditRepo, configRepo,
		func(c context.Context, botID uuid.UUID, telegramID int64, text string) error {
			client, ok := runtimeMgr.ClientFor(botID)
			if !ok {
				return errors.New("bot not running")
			}
			_, err := client.SendMessage(c, telegram.SendMessagePayload{ChatID: telegramID, Text: text})
			return err
		})
	adminOpsH := handlers.NewAdminOpsHandler(adminOpsSvc)
	contentSvc := service.NewContentService(botSvc, tgAccountSvc, contentRepo)
	contentH := handlers.NewContentHandler(contentSvc)

	router := api.NewRouter(api.Deps{
		Auth: authSvc, Admins: adminSvc,
		AuthH: authH, AdminsH: adminsH, BotsH: botsH, FinanceH: financeH,
		TgAccountsH: tgAccountsH,
		PaymentsH:   paymentsH,
		ModerationH: moderationH,
		PanelH:      panelH,
		AdminOpsH:   adminOpsH,
		ContentH:    contentH,
	})

	// boot existing running bots
	if err := runtimeMgr.StartRunningBots(ctx); err != nil {
		log.Printf("start running bots: %v", err)
	}

	// daily retention purge
	go retentionLoop(ctx, eventRepo, cfg.EventRetentionDays)

	srv := &http.Server{Addr: cfg.Addr, Handler: router}
	go func() {
		log.Printf("sadbot listening on %s", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	runtimeMgr.StopAll(shutdownCtx)
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// runtimeNotifier sends payment-result messages via a running bot.
type runtimeNotifier struct {
	users *postgres.BotUserRepo
	mgr   *runtime.Manager
}

func (n *runtimeNotifier) NotifyUser(botID uuid.UUID, userID int64, text string) {
	u, err := n.users.GetByID(context.Background(), botID, userID)
	if err != nil || u == nil {
		return
	}
	client, ok := n.mgr.ClientFor(botID)
	if !ok {
		return
	}
	_, _ = client.SendMessage(context.Background(), telegram.SendMessagePayload{
		ChatID: u.TelegramID, Text: text,
	})
}

func retentionLoop(ctx context.Context, events *postgres.EventRepo, retentionDays int) {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	purge := func() {
		n, err := events.Purge(ctx, time.Duration(retentionDays)*24*time.Hour, 365*24*time.Hour)
		if err != nil {
			log.Printf("retention purge: %v", err)
			return
		}
		if n > 0 {
			log.Printf("retention: deleted %d old events", n)
		}
	}
	purge()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
}
