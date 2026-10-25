package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
	"sadbot/internal/tgsidecar"

	"github.com/google/uuid"
)

// ContentService is the panel surface for the "content" template: a typed
// read/write view over the bot's content prices and prompts, live folder and
// channel pickers from the MTProto sidecar, and the collection ledger.
type ContentService struct {
	bots  *BotService
	tg    *TelegramAccountService
	posts *postgres.ContentRepo
}

func NewContentService(bots *BotService, tg *TelegramAccountService, posts *postgres.ContentRepo) *ContentService {
	return &ContentService{bots: bots, tg: tg, posts: posts}
}

// ContentSettings is the typed window onto prices/texts for the panel form.
type ContentSettings struct {
	Version            int              `json:"version"`
	AccountID          string           `json:"account_id"`
	Folder             string           `json:"folder"`
	Channel            string           `json:"channel"`
	PublishVia         string           `json:"publish_via"` // "bot" | "sidecar" (MTProto account posts)
	Exclude            []string         `json:"exclude"`
	Keywords           []string         `json:"keywords"`
	PollMinutes        int              `json:"poll_minutes"`
	BackfillLimit      int              `json:"backfill_limit"`
	MinLength          int              `json:"min_length"`
	SkipForwarded      bool             `json:"skip_forwarded"`
	SkipReplies        bool             `json:"skip_replies"`
	RenderCardSummary  bool             `json:"render_card_summary"`
	TextOnly           bool             `json:"text_only"`
	BrandKey           string           `json:"brand_key"`
	BrandNameEN        string           `json:"brand_name_en"`
	BrandNameFA        string           `json:"brand_name_fa"`
	BrandHandle        string           `json:"brand_handle"`
	BrandGlowA         string           `json:"brand_glow_a"`
	BrandGlowB         string           `json:"brand_glow_b"`
	BrandGlowC         string           `json:"brand_glow_c"`
	BrandAccent        string           `json:"brand_accent"`
	CleanPrompt        string           `json:"clean_prompt"`
	CardPrompt         string           `json:"card_prompt"`
	DefaultCleanPrompt string           `json:"default_clean_prompt"`
	DefaultCardPrompt  string           `json:"default_card_prompt"`
	Accounts           []ContentAccount `json:"accounts"` // picker of the acting admin's MTProto accounts
}

// ContentAccount is one MTProto account offered in the destination/source
// picker. SidecarID is what content_account_id stores in prices.
type ContentAccount struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Username   string `json:"username,omitempty"`
	Phone      string `json:"phone,omitempty"`
	SidecarID  string `json:"sidecar_account_id"`
	Configured bool   `json:"configured"`
}

// ContentPut is the write payload for PUT content/settings (flat, matches
// ContentSettings without view-only fields). Version guards concurrent edits
// like every config write in the panel.
type ContentPut struct {
	Version           int      `json:"version"`
	AccountID         string   `json:"account_id"`
	Folder            string   `json:"folder"`
	Channel           string   `json:"channel"`
	PublishVia        string   `json:"publish_via"` // "bot" | "sidecar"; empty → "bot"
	Exclude           []string `json:"exclude"`
	Keywords          []string `json:"keywords"`
	PollMinutes       int      `json:"poll_minutes"`
	BackfillLimit     int      `json:"backfill_limit"`
	MinLength         int      `json:"min_length"`
	SkipForwarded     bool     `json:"skip_forwarded"`
	SkipReplies       bool     `json:"skip_replies"`
	RenderCardSummary bool     `json:"render_card_summary"`
	TextOnly          bool     `json:"text_only"`
	BrandKey          string   `json:"brand_key"`
	BrandNameEN       string   `json:"brand_name_en"`
	BrandNameFA       string   `json:"brand_name_fa"`
	BrandHandle       string   `json:"brand_handle"`
	BrandGlowA        string   `json:"brand_glow_a"`
	BrandGlowB        string   `json:"brand_glow_b"`
	BrandGlowC        string   `json:"brand_glow_c"`
	BrandAccent       string   `json:"brand_accent"`
	CleanPrompt       string   `json:"clean_prompt"`
	CardPrompt        string   `json:"card_prompt"`
}

// requireContentBot loads the bot and ensures it uses the content template.
func (s *ContentService) requireContentBot(ctx context.Context, actor *domain.Admin, botID uuid.UUID) (*domain.Bot, error) {
	bot, err := s.bots.Get(ctx, actor, botID) // enforces read permission
	if err != nil {
		return nil, err
	}
	if bot.TemplateKey != domain.TemplateContent {
		return nil, errors.New("این بات از قالب محتوا نیست")
	}
	return bot, nil
}

func (s *ContentService) canConfig(ctx context.Context, actor *domain.Admin, botID uuid.UUID) error {
	ok, err := s.bots.Can(ctx, actor, botID, domain.PermBotConfig)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("دسترسی ندارید")
	}
	return nil
}

// GetContentSettings returns the typed settings view merged onto the
// template defaults, with the acting admin's account picker attached.
func (s *ContentService) GetContentSettings(ctx context.Context, actor *domain.Admin, botID uuid.UUID) (*ContentSettings, error) {
	if _, err := s.requireContentBot(ctx, actor, botID); err != nil {
		return nil, err
	}
	if err := s.canConfig(ctx, actor, botID); err != nil {
		return nil, err
	}
	tpl, _ := s.bots.registry.Get(domain.TemplateContent)
	if tpl == nil {
		return nil, errors.New("قالب محتوا ثبت نشده است")
	}
	cfg, err := s.bots.GetConfig(ctx, actor, botID)
	if err != nil {
		return nil, err
	}

	// base = template defaults, overlay = bot's current prices (same merge as SettingsOf)
	raw := map[string]any{}
	if priced, ok := tpl.(engine.Priced); ok {
		var defPrices map[string]any
		if err := json.Unmarshal(priced.DefaultPrices(), &defPrices); err == nil {
			for k, v := range defPrices {
				raw[k] = v
			}
		}
	}
	if len(cfg.Prices) > 0 {
		var cur map[string]any
		if err := json.Unmarshal(cfg.Prices, &cur); err == nil {
			for k, v := range cur {
				raw[k] = v
			}
		}
	}
	texts := map[string]any{}
	_ = json.Unmarshal(cfg.Texts, &texts)
	defTexts := map[string]any{}
	_ = json.Unmarshal(tpl.DefaultTexts(), &defTexts)

	str := func(m map[string]any, key string) string {
		if v, ok := m[key].(string); ok {
			return v
		}
		return ""
	}
	num := func(key string, def int) int {
		switch n := raw[key].(type) {
		case float64:
			return int(n)
		case int:
			return n
		default:
			return def
		}
	}
	flag := func(key string, def bool) bool {
		switch b := raw[key].(type) {
		case bool:
			return b
		default:
			return def
		}
	}
	list := func(key string) []string {
		out := []string{}
		if arr, ok := raw[key].([]any); ok {
			for _, e := range arr {
				if v, ok := e.(string); ok && strings.TrimSpace(v) != "" {
					out = append(out, v)
				}
			}
		}
		return out
	}

	out := &ContentSettings{
		Version:            cfg.Version,
		AccountID:          str(raw, "content_account_id"),
		Folder:             str(raw, "content_folder"),
		Channel:            str(raw, "content_channel"),
		PublishVia:         defaultPublishVia(str(raw, "content_publish_via")),
		Exclude:            list("content_exclude"),
		Keywords:           list("content_keywords"),
		PollMinutes:        num("content_poll_minutes", 2),
		BackfillLimit:      num("content_backfill_limit", 20),
		MinLength:          num("content_min_length", 40),
		SkipForwarded:      flag("content_skip_forwarded", false),
		SkipReplies:        flag("content_skip_replies", false),
		RenderCardSummary:  flag("render_card_summary", true),
		TextOnly:           flag("content_text_only", false),
		BrandKey:           str(raw, "brand_key"),
		BrandNameEN:        str(raw, "brand_name_en"),
		BrandNameFA:        str(raw, "brand_name_fa"),
		BrandHandle:        str(raw, "brand_handle"),
		BrandGlowA:         str(raw, "brand_glow_a"),
		BrandGlowB:         str(raw, "brand_glow_b"),
		BrandGlowC:         str(raw, "brand_glow_c"),
		BrandAccent:        str(raw, "brand_accent"),
		CleanPrompt:        str(texts, "clean_prompt"),
		CardPrompt:         str(texts, "card_prompt"),
		DefaultCleanPrompt: str(defTexts, "clean_prompt"),
		DefaultCardPrompt:  str(defTexts, "card_prompt"),
	}

	// account picker (the acting admin's MTProto accounts, display-enriched)
	if accounts, err := s.tg.List(ctx, actor); err == nil {
		for _, a := range accounts {
			out.Accounts = append(out.Accounts, ContentAccount{
				ID:         a.ID.String(),
				Label:      a.Label,
				Username:   a.Username,
				Phone:      a.Phone,
				SidecarID:  a.SidecarAccountID,
				Configured: a.SidecarAccountID == out.AccountID,
			})
		}
	}
	return out, nil
}

// PutContentSettings validates and saves the typed settings, preserving every
// non-listed price key and the rest of the texts document.
func (s *ContentService) PutContentSettings(ctx context.Context, actor *domain.Admin, botID uuid.UUID, in ContentPut, ip *string) (*domain.BotConfig, error) {
	if _, err := s.requireContentBot(ctx, actor, botID); err != nil {
		return nil, err
	}
	if err := s.canConfig(ctx, actor, botID); err != nil {
		return nil, err
	}
	// fail-closed guard: an empty source/destination silently stops the
	// collector mid-flight, so the panel refuses to save them blank
	if strings.TrimSpace(in.AccountID) == "" {
		return nil, errors.New("اکانت تلگرام منبع را انتخاب کنید")
	}
	if strings.TrimSpace(in.Folder) == "" {
		return nil, errors.New("پوشهٔ منبع (فولدر تلگرام) را انتخاب کنید")
	}
	if strings.TrimSpace(in.Channel) == "" {
		return nil, errors.New("کانال مقصد را انتخاب کنید")
	}
	// validate BEFORE normalizing so a bogus value is rejected, not collapsed
	via := strings.TrimSpace(in.PublishVia)
	if via == "" {
		via = "bot" // older payloads without the field
	}
	if via != "bot" && via != "sidecar" {
		return nil, errors.New("نوع انتشار باید «بات» یا «اکانت تلگرام» باشد")
	}
	in.PublishVia = via
	if in.PollMinutes < 1 || in.PollMinutes > 1440 {
		return nil, errors.New("فاصلهٔ بررسی باید بین ۱ و ۱۴۴۰ دقیقه باشد")
	}
	if in.BackfillLimit < 1 || in.BackfillLimit > 100 {
		return nil, errors.New("سقف بک‌فیل باید بین ۱ و ۱۰۰ باشد")
	}
	if in.MinLength < 0 || in.MinLength > 4096 {
		return nil, errors.New("حداقل طول متن باید بین ۰ و ۴۰۹۶ باشد")
	}

	cur, err := s.bots.GetConfig(ctx, actor, botID)
	if err != nil {
		return nil, err
	}
	prices := map[string]any{}
	if len(cur.Prices) > 0 && json.Unmarshal(cur.Prices, &prices) != nil {
		return nil, errors.New("تنظیمات فعلی نامعتبر است")
	}
	prices["content_account_id"] = strings.TrimSpace(in.AccountID)
	prices["content_folder"] = strings.TrimSpace(in.Folder)
	prices["content_channel"] = strings.TrimSpace(in.Channel)
	prices["content_publish_via"] = in.PublishVia
	prices["content_exclude"] = nonEmptyTrimmed(in.Exclude)
	prices["content_keywords"] = nonEmptyTrimmed(in.Keywords)
	prices["content_poll_minutes"] = in.PollMinutes
	prices["content_backfill_limit"] = in.BackfillLimit
	prices["content_min_length"] = in.MinLength
	prices["content_skip_forwarded"] = in.SkipForwarded
	prices["content_skip_replies"] = in.SkipReplies
	prices["render_card_summary"] = in.RenderCardSummary
	prices["content_text_only"] = in.TextOnly
	prices["brand_key"] = strings.TrimSpace(in.BrandKey)
	prices["brand_name_en"] = strings.TrimSpace(in.BrandNameEN)
	prices["brand_name_fa"] = strings.TrimSpace(in.BrandNameFA)
	prices["brand_handle"] = strings.TrimSpace(in.BrandHandle)
	prices["brand_glow_a"] = strings.TrimSpace(in.BrandGlowA)
	prices["brand_glow_b"] = strings.TrimSpace(in.BrandGlowB)
	prices["brand_glow_c"] = strings.TrimSpace(in.BrandGlowC)
	prices["brand_accent"] = strings.TrimSpace(in.BrandAccent)
	newPrices, err := json.Marshal(prices)
	if err != nil {
		return nil, err
	}

	// prompts live in texts — preserve every other key
	texts := map[string]any{}
	if len(cur.Texts) > 0 && json.Unmarshal(cur.Texts, &texts) != nil {
		return nil, errors.New("متن‌های فعلی نامعتبر است")
	}
	// an empty prompt means "use the built-in default" (collector falls back)
	if p := in.CleanPrompt; strings.TrimSpace(p) != "" {
		texts["clean_prompt"] = p
	} else {
		delete(texts, "clean_prompt")
	}
	if p := in.CardPrompt; strings.TrimSpace(p) != "" {
		texts["card_prompt"] = p
	} else {
		delete(texts, "card_prompt")
	}
	newTexts, err := json.Marshal(texts)
	if err != nil {
		return nil, err
	}

	return s.bots.UpdateConfig(ctx, actor, botID, UpdateConfigInput{
		Version:      in.Version,
		Texts:        newTexts,
		Menus:        cur.Menus,
		FeatureFlags: cur.FeatureFlags,
		Prices:       newPrices,
	}, ip)
}

// Folders lists the dialog folders of the selected MTProto account (the
// account must belong to the acting admin; bot permission checked by caller).
func (s *ContentService) Folders(ctx context.Context, actor *domain.Admin, accountID string) ([]tgsidecar.Folder, error) {
	id, err := s.resolveAccountID(ctx, actor, accountID)
	if err != nil {
		return nil, err
	}
	return s.tg.Folders(ctx, actor, id)
}

// resolveAccountID accepts either the panel account UUID (tg_accounts.id) or
// the sidecar account id (content_account_id in prices — what the panel's
// content picker sends) and returns the panel UUID after an ownership check.
// Go's uuid.Parse also accepts the hex sidecar id, so the match against the
// acting admin's accounts must come FIRST.
func (s *ContentService) resolveAccountID(ctx context.Context, actor *domain.Admin, ref string) (uuid.UUID, error) {
	accounts, listErr := s.tg.List(ctx, actor) // superadmin sees all, admin only their own
	if listErr == nil {
		for _, a := range accounts {
			if strings.EqualFold(a.SidecarAccountID, ref) || a.ID.String() == strings.ToLower(ref) {
				return a.ID, nil
			}
		}
	}
	if id, err := uuid.Parse(ref); err == nil {
		// a panel UUID the admin can't see: let owned() inside Folders/Chats
		// give the definitive error (covers superadmin passing any panel uuid)
		return id, nil
	}
	return uuid.Nil, errors.New("اکانت یافت نشد")
}

// Channels lists channels/groups of the selected account for the destination
// picker (chats carry title + numeric chat_id; username is only in folders).
func (s *ContentService) Channels(ctx context.Context, actor *domain.Admin, accountID string) ([]tgsidecar.Chat, error) {
	id, err := s.resolveAccountID(ctx, actor, accountID)
	if err != nil {
		return nil, err
	}
	chats, err := s.tg.Chats(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	out := make([]tgsidecar.Chat, 0, len(chats))
	for _, c := range chats {
		if c.Type == "channel" || c.Type == "group" {
			out = append(out, c)
		}
	}
	return out, nil
}

// ContentPostPage is one ledger page plus the stat box counts.
type ContentPostPage struct {
	Posts   []domain.ContentPost `json:"posts"`
	Total   int                  `json:"total"`
	Counts  map[string]int64     `json:"counts"`
	Last24h int64                `json:"last_24h"`
}

// Posts returns a ledger page (newest first) with status/text filters.
func (s *ContentService) Posts(ctx context.Context, actor *domain.Admin, botID uuid.UUID, page, size int, status, q string) (*ContentPostPage, error) {
	if _, err := s.requireContentBot(ctx, actor, botID); err != nil {
		return nil, err
	}
	posts, total, err := s.posts.List(ctx, botID, page, size, status, q)
	if err != nil {
		return nil, err
	}
	counts, last24h, err := s.posts.Counts(ctx, botID)
	if err != nil {
		return nil, err
	}
	return &ContentPostPage{Posts: posts, Total: total, Counts: counts, Last24h: last24h}, nil
}

// CloneBot creates a new bot via BotFather on the given MTProto account using
// the source bot's template, then copies the source config onto it.
// ConfigErr is non-empty when the bot was created but the config copy failed —
// the new bot exists either way and the caller should surface both.
type CloneResult struct {
	Bot       *domain.Bot `json:"bot"`
	ConfigErr string      `json:"config_error,omitempty"`
}

func (s *ContentService) CloneBot(ctx context.Context, actor *domain.Admin, sourceID uuid.UUID, accountIDStr, name, hint string, ip *string) (*CloneResult, error) {
	src, err := s.requireContentBot(ctx, actor, sourceID)
	if err != nil {
		return nil, err
	}
	accID, err := uuid.Parse(accountIDStr)
	if err != nil {
		return nil, errors.New("اکانت تلگرام را برای ساخت بات انتخاب کنید")
	}
	if strings.TrimSpace(name) == "" {
		name = src.Name + " (کلون)"
	}
	bot, err := s.tg.CreateBot(ctx, actor, accID, CreateBotViaAccountInput{
		Name:         name,
		TemplateKey:  domain.TemplateContent,
		UsernameHint: hint,
	}, ip)
	if err != nil {
		return nil, err
	}
	res := &CloneResult{Bot: bot}
	if _, cerr := s.bots.CopyConfig(ctx, actor, sourceID, bot.ID, ip); cerr != nil {
		res.ConfigErr = cerr.Error()
	}
	return res, nil
}

func nonEmptyTrimmed(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		t := strings.TrimSpace(v)
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// defaultPublishVia normalizes the publish path: "" (older panel payloads)
// and unknown strings fall back to the running bot.
func defaultPublishVia(v string) string {
	if v == "sidecar" {
		return "sidecar"
	}
	return "bot"
}
