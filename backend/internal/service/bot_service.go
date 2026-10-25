package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	pgcrypto "sadbot/pkg/crypto"

	"github.com/google/uuid"
)

type BotRepository interface {
	Create(ctx context.Context, name string, username *string, tokenEnc []byte, templateKey string, createdBy uuid.UUID) (*domain.Bot, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Bot, error)
	ListByAdmin(ctx context.Context, adminID uuid.UUID, isSuper bool) ([]domain.Bot, error)
	Update(ctx context.Context, id uuid.UUID, name *string, username *string, templateKey *string) (*domain.Bot, error)
	UpdateToken(ctx context.Context, id uuid.UUID, tokenEnc []byte, username *string) error
	SetStatus(ctx context.Context, id uuid.UUID, status string, lastError *string) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type ConfigRepository interface {
	Get(ctx context.Context, botID uuid.UUID) (*domain.BotConfig, error)
	Upsert(ctx context.Context, c *domain.BotConfig, updatedBy *uuid.UUID) (*domain.BotConfig, error)
	EnsureDefault(ctx context.Context, botID uuid.UUID, texts, menus, flags, prices []byte) (*domain.BotConfig, error)
}

// TokenValidator checks a candidate bot token against Telegram.
type TokenValidator interface {
	ValidateToken(ctx context.Context, token string) (username string, err error)
}

// RuntimeController starts/stops a bot's polling runtime.
type RuntimeController interface {
	Start(ctx context.Context, bot *domain.Bot) error
	Stop(ctx context.Context, botID uuid.UUID) error
	IsRunning(botID uuid.UUID) bool
}

type BotService struct {
	bots     BotRepository
	configs  ConfigRepository
	registry *engine.Registry
	validate TokenValidator
	runtime  RuntimeController
	audit    AuditLogger
	admins   AdminRepository
	key      []byte
}

func NewBotService(bots BotRepository, configs ConfigRepository, registry *engine.Registry, validate TokenValidator, runtime RuntimeController, audit AuditLogger, admins AdminRepository, encKey []byte) *BotService {
	return &BotService{bots: bots, configs: configs, registry: registry, validate: validate, runtime: runtime, audit: audit, admins: admins, key: encKey}
}

var tokenRe = regexp.MustCompile(`^\d{6,12}:[A-Za-z0-9_-]{30,}$`)

type CreateBotInput struct {
	Name        string `json:"name"`
	Token       string `json:"token"`
	TemplateKey string `json:"template_key"`
}

func (s *BotService) Create(ctx context.Context, actor *domain.Admin, in CreateBotInput, ip *string) (*domain.Bot, error) {
	if !actor.IsSuperAdmin() {
		return nil, errors.New("فقط superadmin می‌تواند بات بسازد")
	}
	if in.Name == "" {
		return nil, errors.New("نام بات لازم است")
	}
	if !tokenRe.MatchString(in.Token) {
		return nil, errors.New("توکن ربات نامعتبر است (فرمت: 123456:ABC-DEF...)")
	}
	if in.TemplateKey == "" {
		in.TemplateKey = domain.TemplateWelcomeMenu
	}
	if _, ok := s.registry.Get(in.TemplateKey); !ok {
		return nil, fmt.Errorf("تمپلیت نامعتبر: %s", in.TemplateKey)
	}
	username, err := s.validate.ValidateToken(ctx, in.Token)
	if err != nil {
		// do not echo the raw error: it may contain the token in the request URL
		return nil, errors.New("توکن توسط تلگرام پذیرفته نشد؛ توکن را بررسی کنید")
	}
	enc, err := pgcrypto.EncryptToken(s.key, in.Token)
	if err != nil {
		return nil, err
	}
	bot, err := s.bots.Create(ctx, in.Name, &username, enc, in.TemplateKey, actor.ID)
	if err != nil {
		return nil, err
	}
	// seed default config from template
	tpl, _ := s.registry.Get(in.TemplateKey)
	var defaultPrices []byte
	if priced, ok := tpl.(engine.Priced); ok {
		defaultPrices = priced.DefaultPrices()
	} else {
		defaultPrices = []byte("{}")
	}
	_, _ = s.configs.EnsureDefault(ctx, bot.ID, tpl.DefaultTexts(), tpl.DefaultMenus(), []byte("{}"), defaultPrices)
	_ = s.audit.Insert(ctx, &actor.ID, "bot.create", "bot", bot.ID.String(), &bot.ID, map[string]any{"name": in.Name, "template": in.TemplateKey}, ip)
	return bot, nil
}

func (s *BotService) List(ctx context.Context, actor *domain.Admin) ([]domain.Bot, error) {
	return s.bots.ListByAdmin(ctx, actor.ID, actor.IsSuperAdmin())
}

func (s *BotService) Get(ctx context.Context, actor *domain.Admin, id uuid.UUID) (*domain.Bot, error) {
	ok, err := s.Can(ctx, actor, id, domain.PermBotRead)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	return s.bots.GetByID(ctx, id)
}

type UpdateBotInput struct {
	Name        *string `json:"name"`
	TemplateKey *string `json:"template_key"`
	Token       *string `json:"token"`
}

func (s *BotService) Update(ctx context.Context, actor *domain.Admin, id uuid.UUID, in UpdateBotInput, ip *string) (*domain.Bot, error) {
	ok, err := s.Can(ctx, actor, id, domain.PermBotConfig)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	if in.TemplateKey != nil {
		if _, valid := s.registry.Get(*in.TemplateKey); !valid {
			return nil, fmt.Errorf("تمپلیت نامعتبر: %s", *in.TemplateKey)
		}
	}
	var username *string
	if in.Token != nil && *in.Token != "" {
		if !tokenRe.MatchString(*in.Token) {
			return nil, errors.New("توکن ربات نامعتبر است")
		}
		u, err := s.validate.ValidateToken(ctx, *in.Token)
		if err != nil {
			return nil, errors.New("توکن توسط تلگرام پذیرفته نشد؛ توکن را بررسی کنید")
		}
		enc, err := pgcrypto.EncryptToken(s.key, *in.Token)
		if err != nil {
			return nil, err
		}
		if err := s.bots.UpdateToken(ctx, id, enc, &u); err != nil {
			return nil, err
		}
		// restart runtime if it was running with the old token
		if s.runtime.IsRunning(id) {
			_ = s.runtime.Stop(ctx, id)
		}
	}
	bot, err := s.bots.Update(ctx, id, in.Name, username, in.TemplateKey)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "bot.update", "bot", id.String(), &id, nil, ip)
	return bot, nil
}

func (s *BotService) Delete(ctx context.Context, actor *domain.Admin, id uuid.UUID, ip *string) error {
	if !actor.IsSuperAdmin() {
		return errors.New("فقط superadmin می‌تواند بات حذف کند")
	}
	_ = s.runtime.Stop(ctx, id)
	if err := s.bots.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "bot.delete", "bot", id.String(), &id, nil, ip)
	return nil
}

func (s *BotService) Start(ctx context.Context, actor *domain.Admin, id uuid.UUID, ip *string) error {
	ok, err := s.Can(ctx, actor, id, domain.PermBotRuntime)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("دسترسی ندارید")
	}
	bot, err := s.bots.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if err := s.runtime.Start(ctx, bot); err != nil {
		_ = s.bots.SetStatus(ctx, id, domain.BotStatusError, strPtr(err.Error()))
		return err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "bot.start", "bot", id.String(), &id, nil, ip)
	return nil
}

func (s *BotService) Stop(ctx context.Context, actor *domain.Admin, id uuid.UUID, ip *string) error {
	ok, err := s.Can(ctx, actor, id, domain.PermBotRuntime)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("دسترسی ندارید")
	}
	if err := s.runtime.Stop(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "bot.stop", "bot", id.String(), &id, nil, ip)
	return nil
}

func (s *BotService) GetConfig(ctx context.Context, actor *domain.Admin, id uuid.UUID) (*domain.BotConfig, error) {
	ok, err := s.Can(ctx, actor, id, domain.PermBotConfig)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	cfg, err := s.configs.Get(ctx, id)
	if err != nil {
		// no row yet: return defaults for the bot's template
		bot, berr := s.bots.GetByID(ctx, id)
		if berr != nil {
			return nil, err
		}
		tpl, _ := s.registry.Get(bot.TemplateKey)
		if tpl == nil {
			return nil, err
		}
		return &domain.BotConfig{
			BotID: id, Texts: tpl.DefaultTexts(), Menus: tpl.DefaultMenus(),
			FeatureFlags: []byte("{}"), Prices: []byte("{}"), Version: 0,
		}, nil
	}
	return cfg, nil
}

// GetSettings returns the editable numeric/bool price fields merged onto the
// template defaults, for the panel's settings form.
func (s *BotService) GetSettings(ctx context.Context, actor *domain.Admin, id uuid.UUID) ([]SettingField, error) {
	if ok, err := s.Can(ctx, actor, id, domain.PermBotConfig); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	bot, err := s.bots.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	tpl, _ := s.registry.Get(bot.TemplateKey)
	if tpl == nil {
		return nil, errors.New("قالب بات یافت نشد")
	}
	var defaults []byte
	if priced, ok := tpl.(engine.Priced); ok {
		defaults = priced.DefaultPrices()
	}
	cfg, err := s.GetConfig(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return SettingsOfTemplate(bot.TemplateKey, defaults, cfg.Prices)
}

// UpdateSettings validates and applies the submitted numeric fields, keeping
// every non-listed price key (channels, packages, streak_rewards) intact.
func (s *BotService) UpdateSettings(ctx context.Context, actor *domain.Admin, id uuid.UUID, version int, values map[string]int, ip *string) (*domain.BotConfig, error) {
	if ok, err := s.Can(ctx, actor, id, domain.PermBotConfig); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	bot, err := s.bots.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	cur, err := s.GetConfig(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	merged, err := ApplySettingsTemplate(bot.TemplateKey, cur.Prices, values)
	if err != nil {
		return nil, err
	}
	return s.UpdateConfig(ctx, actor, id, UpdateConfigInput{
		Version: version,
		Texts:   cur.Texts, Menus: cur.Menus,
		FeatureFlags: cur.FeatureFlags, Prices: merged,
	}, ip)
}

// ChannelEntry is one membership channel as edited in the panel. Only Id is
// required by the bot; Title and Link are panel display metadata.
type ChannelEntry struct {
	Id    string `json:"id"`
	Title string `json:"title,omitempty"`
	Link  string `json:"link,omitempty"`
}

// GetChannels returns the bot's membership channels for the panel editor.
func (s *BotService) GetChannels(ctx context.Context, actor *domain.Admin, id uuid.UUID) ([]ChannelEntry, error) {
	if ok, err := s.Can(ctx, actor, id, domain.PermBotConfig); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	cfg, err := s.GetConfig(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	var prices map[string]any
	if len(cfg.Prices) > 0 {
		if err := json.Unmarshal(cfg.Prices, &prices); err != nil {
			return nil, err
		}
	}
	arr, _ := prices["channels"].([]any)
	out := make([]ChannelEntry, 0, len(arr))
	for _, v := range arr {
		switch ch := v.(type) {
		case string:
			if t := strings.TrimSpace(ch); t != "" {
				out = append(out, ChannelEntry{Id: t})
			}
		case map[string]any:
			e := ChannelEntry{}
			if s, ok := ch["id"].(string); ok {
				e.Id = strings.TrimSpace(s)
			}
			if s, ok := ch["title"].(string); ok {
				e.Title = s
			}
			if s, ok := ch["link"].(string); ok {
				e.Link = s
			}
			if e.Id != "" {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// SetChannels replaces the membership channel list, leaving every other price
// key (bonus, costs, packages…) untouched.
func (s *BotService) SetChannels(ctx context.Context, actor *domain.Admin, id uuid.UUID, version int, entries []ChannelEntry, ip *string) (*domain.BotConfig, error) {
	if ok, err := s.Can(ctx, actor, id, domain.PermBotConfig); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	cur, err := s.GetConfig(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	prices := map[string]any{}
	if len(cur.Prices) > 0 {
		if err := json.Unmarshal(cur.Prices, &prices); err != nil {
			return nil, errors.New("تنظیمات فعلی نامعتبر است")
		}
	}
	list := make([]any, 0, len(entries))
	seen := map[string]bool{}
	for _, e := range entries {
		chID := strings.TrimSpace(e.Id)
		if chID == "" || seen[chID] {
			continue
		}
		seen[chID] = true
		item := map[string]any{"id": chID}
		if t := strings.TrimSpace(e.Title); t != "" {
			item["title"] = t
		}
		if l := strings.TrimSpace(e.Link); l != "" {
			item["link"] = l
		}
		list = append(list, item)
	}
	prices["channels"] = list
	merged, err := json.Marshal(prices)
	if err != nil {
		return nil, err
	}
	return s.UpdateConfig(ctx, actor, id, UpdateConfigInput{
		Version: version,
		Texts:   cur.Texts, Menus: cur.Menus,
		FeatureFlags: cur.FeatureFlags, Prices: merged,
	}, ip)
}

type UpdateConfigInput struct {
	Version      int             `json:"version"`
	Texts        json.RawMessage `json:"texts"`
	Menus        json.RawMessage `json:"menus"`
	FeatureFlags json.RawMessage `json:"feature_flags"`
	Prices       json.RawMessage `json:"prices"`
}

// NormalizeConfigInput unwraps config fields sent as JSON strings holding JSON
// (the panel stringifies its textareas). Without this the value is stored as a
// jsonb *string* and the runtime merge silently ignores every panel edit.
// Object-shaped values pass through untouched.
func NormalizeConfigInput(in UpdateConfigInput) UpdateConfigInput {
	in.Texts = unquoteJSON(in.Texts)
	in.Menus = unquoteJSON(in.Menus)
	in.FeatureFlags = unquoteJSON(in.FeatureFlags)
	in.Prices = unquoteJSON(in.Prices)
	return in
}

func unquoteJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return raw
	}
	if json.Valid([]byte(s)) {
		return json.RawMessage(s)
	}
	return raw
}

func (s *BotService) UpdateConfig(ctx context.Context, actor *domain.Admin, id uuid.UUID, in UpdateConfigInput, ip *string) (*domain.BotConfig, error) {
	ok, err := s.Can(ctx, actor, id, domain.PermBotConfig)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	in = NormalizeConfigInput(in)
	if !json.Valid(in.Texts) || !json.Valid(in.Menus) {
		return nil, errors.New("JSON نامعتبر")
	}
	cfg := &domain.BotConfig{
		BotID: id, Texts: in.Texts, Menus: in.Menus,
		FeatureFlags: in.FeatureFlags, Prices: in.Prices,
		Version: in.Version,
	}
	if len(cfg.FeatureFlags) == 0 {
		cfg.FeatureFlags = []byte("{}")
	}
	if len(cfg.Prices) == 0 {
		cfg.Prices = []byte("{}")
	}
	saved, err := s.configs.Upsert(ctx, cfg, &actor.ID)
	if err != nil {
		var conflict domain.ErrVersionConflict
		if errors.As(err, &conflict) {
			return nil, errors.New("تنظیمات همزمان توسط کاربر دیگری ویرایش شده؛ صفحه را تازه کنید")
		}
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "bot.config_update", "bot", id.String(), &id, map[string]any{"version": saved.Version}, ip)
	return saved, nil
}

func (s *BotService) Can(ctx context.Context, actor *domain.Admin, botID uuid.UUID, perm string) (bool, error) {
	if actor.IsSuperAdmin() {
		return true, nil
	}
	return s.admins.HasBotPermission(ctx, actor.ID, botID, perm)
}

func (s *BotService) IsRunning(botID uuid.UUID) bool { return s.runtime.IsRunning(botID) }

func strPtr(s string) *string { return &s }

// CopyConfig copies src's whole config document onto dst. For content bots
// both the destination channel and the source account are stripped: the
// collector treats an empty account/channel as "not configured" and stays
// silent, so a clone cannot start publishing into the source's channel before
// an admin points it at its own source/destination.
func (s *BotService) CopyConfig(ctx context.Context, actor *domain.Admin, src, dst uuid.UUID, ip *string) (*domain.BotConfig, error) {
	if ok, err := s.Can(ctx, actor, src, domain.PermBotConfig); err != nil {
		return nil, err
	} else if !ok {
		return nil, errors.New("دسترسی ندارید")
	}
	from, err := s.GetConfig(ctx, actor, src)
	if err != nil {
		return nil, err
	}
	// Read dst without the version requirement — a fresh clone's seeded config
	// must be overwritten even when its version differs (probe: v0 insert).
	dstCfg, dstErr := s.configs.Get(ctx, dst)
	if dstErr != nil {
		dstCfg = &domain.BotConfig{BotID: dst, Version: 0}
	}
	prices := from.Prices
	if srcBot, err := s.bots.GetByID(ctx, src); err == nil && srcBot.TemplateKey == domain.TemplateContent {
		stripped, err := stripCloneBlankPrices(prices)
		if err != nil {
			return nil, err
		}
		prices = stripped
	}
	saved, err := s.configs.Upsert(ctx, &domain.BotConfig{
		BotID: dst, Texts: from.Texts, Menus: from.Menus,
		FeatureFlags: from.FeatureFlags, Prices: prices,
		Version: dstCfg.Version,
	}, &actor.ID)
	if err != nil {
		var conflict domain.ErrVersionConflict
		if errors.As(err, &conflict) {
			return nil, errors.New("تنظیمات مقصد همزمان تغییر کرده؛ دوباره تلاش کنید")
		}
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "bot.clone_config", "bot", dst.String(), &dst,
		map[string]any{"from": src.String()}, ip)
	return saved, nil
}

// stripCloneBlankPrices drops the account/channel pair from content prices so
// a cloned bot never reads the source's folder or writes the source's channel.
func stripCloneBlankPrices(prices json.RawMessage) (json.RawMessage, error) {
	m := map[string]any{}
	if len(prices) > 0 {
		if err := json.Unmarshal(prices, &m); err != nil {
			return nil, err
		}
	}
	delete(m, "content_channel")
	delete(m, "content_account_id")
	return json.Marshal(m)
}
