// Package testutil provides in-memory fakes for unit tests across packages.
// The fakes structurally satisfy the service-layer interfaces without importing
// service, so both service and api tests can share them.
package testutil

import (
	"context"
	"errors"
	"sync"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Hash returns a cheap bcrypt hash for tests (MinCost).
func Hash(pwd string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.MinCost)
	if err != nil {
		panic(err)
	}
	return string(h)
}

// ---- AdminRepository ----

type AdminRepo struct {
	mu     sync.Mutex
	byID   map[uuid.UUID]*domain.Admin
	access map[uuid.UUID]map[uuid.UUID][]string
}

func NewAdminRepo() *AdminRepo {
	return &AdminRepo{byID: map[uuid.UUID]*domain.Admin{}, access: map[uuid.UUID]map[uuid.UUID][]string{}}
}

// Add inserts an admin as-is (test seeding).
func (f *AdminRepo) Add(a *domain.Admin) *domain.Admin {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.byID[a.ID] = a
	return a
}

func (f *AdminRepo) Create(ctx context.Context, email, passwordHash, displayName, role string) (*domain.Admin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := &domain.Admin{ID: uuid.New(), Email: email, PasswordHash: passwordHash, DisplayName: displayName, Role: role, Status: domain.AdminStatusActive}
	f.byID[a.ID] = a
	cp := *a
	return &cp, nil
}

func (f *AdminRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Admin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}

func (f *AdminRepo) GetByEmail(ctx context.Context, email string) (*domain.Admin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, a := range f.byID {
		if a.Email == email {
			cp := *a
			return &cp, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *AdminRepo) List(ctx context.Context) ([]domain.Admin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]domain.Admin, 0, len(f.byID))
	for _, a := range f.byID {
		out = append(out, *a)
	}
	return out, nil
}

func (f *AdminRepo) Update(ctx context.Context, id uuid.UUID, displayName, role, status *string) (*domain.Admin, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	if displayName != nil {
		a.DisplayName = *displayName
	}
	if role != nil {
		a.Role = *role
	}
	if status != nil {
		a.Status = *status
	}
	cp := *a
	return &cp, nil
}

func (f *AdminRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return errors.New("not implemented")
}

func (f *AdminRepo) Delete(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.byID, id)
	return nil
}

func (f *AdminRepo) Count(ctx context.Context) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.byID)), nil
}

func (f *AdminRepo) GetBotAccess(ctx context.Context, adminID, botID uuid.UUID) (*domain.BotAccess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	perms := f.access[adminID][botID]
	if perms == nil {
		return nil, errors.New("not found")
	}
	return &domain.BotAccess{AdminID: adminID, BotID: botID, Permissions: perms}, nil
}

func (f *AdminRepo) ListBotAccessByAdmin(ctx context.Context, adminID uuid.UUID) ([]domain.BotAccess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.BotAccess{}
	for botID, perms := range f.access[adminID] {
		out = append(out, domain.BotAccess{AdminID: adminID, BotID: botID, Permissions: perms})
	}
	return out, nil
}

func (f *AdminRepo) UpsertBotAccess(ctx context.Context, adminID, botID uuid.UUID, perms []string, grantedBy *uuid.UUID) (*domain.BotAccess, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.access[adminID] == nil {
		f.access[adminID] = map[uuid.UUID][]string{}
	}
	f.access[adminID][botID] = perms
	return &domain.BotAccess{AdminID: adminID, BotID: botID, Permissions: perms}, nil
}

func (f *AdminRepo) HasBotPermission(ctx context.Context, adminID, botID uuid.UUID, perm string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.access[adminID][botID] {
		if p == perm {
			return true, nil
		}
	}
	return false, nil
}

// ---- TelegramAccountRepository ----

type TelegramAccountRepo struct {
	mu       sync.Mutex
	accounts map[uuid.UUID]*domain.TelegramAccount
}

func NewTelegramAccountRepo() *TelegramAccountRepo {
	return &TelegramAccountRepo{accounts: map[uuid.UUID]*domain.TelegramAccount{}}
}

func (f *TelegramAccountRepo) Create(ctx context.Context, adminID uuid.UUID, label string, tgUserID int64, phone, sidecarID string) (*domain.TelegramAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a := &domain.TelegramAccount{ID: uuid.New(), AdminID: adminID, Label: label, TgUserID: tgUserID, Phone: phone, SidecarAccountID: sidecarID}
	f.accounts[a.ID] = a
	cp := *a
	return &cp, nil
}

func (f *TelegramAccountRepo) List(ctx context.Context, adminID *uuid.UUID) ([]domain.TelegramAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.TelegramAccount{}
	for _, a := range f.accounts {
		if adminID != nil && a.AdminID != *adminID {
			continue
		}
		cp := *a
		out = append(out, cp)
	}
	return out, nil
}

func (f *TelegramAccountRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.TelegramAccount, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.accounts[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}

func (f *TelegramAccountRepo) Delete(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.accounts, id)
	return nil
}

// ---- AuditLogger ----

type Audit struct {
	mu      sync.Mutex
	Entries []string
}

func (f *Audit) Insert(ctx context.Context, actorAdminID *uuid.UUID, action, targetType, targetID string, botID *uuid.UUID, details map[string]any, ip *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Entries = append(f.Entries, action)
	return nil
}

// ---- BotRepository ----

type BotRepo struct {
	mu   sync.Mutex
	Bots map[uuid.UUID]*domain.Bot
}

func NewBotRepo() *BotRepo { return &BotRepo{Bots: map[uuid.UUID]*domain.Bot{}} }

func (f *BotRepo) Create(ctx context.Context, name string, username *string, tokenEnc []byte, templateKey string, createdBy uuid.UUID) (*domain.Bot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b := &domain.Bot{ID: uuid.New(), Name: name, TelegramUsername: username, TokenEncrypted: tokenEnc, TemplateKey: templateKey, Status: domain.BotStatusStopped, CreatedBy: createdBy}
	f.Bots[b.ID] = b
	cp := *b
	return &cp, nil
}

func (f *BotRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Bot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.Bots[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *b
	return &cp, nil
}

func (f *BotRepo) ListByAdmin(ctx context.Context, adminID uuid.UUID, isSuper bool) ([]domain.Bot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []domain.Bot{}
	for _, b := range f.Bots {
		out = append(out, *b)
	}
	return out, nil
}

func (f *BotRepo) Update(ctx context.Context, id uuid.UUID, name *string, username *string, templateKey *string) (*domain.Bot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.Bots[id]
	if !ok {
		return nil, errors.New("not found")
	}
	if name != nil {
		b.Name = *name
	}
	if username != nil {
		b.TelegramUsername = username
	}
	if templateKey != nil {
		b.TemplateKey = *templateKey
	}
	cp := *b
	return &cp, nil
}

func (f *BotRepo) UpdateToken(ctx context.Context, id uuid.UUID, tokenEnc []byte, username *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.Bots[id]
	if !ok {
		return errors.New("not found")
	}
	b.TokenEncrypted = tokenEnc
	b.TelegramUsername = username
	return nil
}

func (f *BotRepo) SetStatus(ctx context.Context, id uuid.UUID, status string, lastError *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.Bots[id]
	if !ok {
		return errors.New("not found")
	}
	b.Status = status
	b.LastError = lastError
	return nil
}

func (f *BotRepo) Delete(ctx context.Context, id uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Bots, id)
	return nil
}

// ---- ConfigRepository ----

type ConfigRepo struct {
	mu   sync.Mutex
	Cfgs map[uuid.UUID]*domain.BotConfig
	// ConflictOnUpsert makes Upsert return domain.ErrVersionConflict when true.
	ConflictOnUpsert bool
}

func NewConfigRepo() *ConfigRepo {
	return &ConfigRepo{Cfgs: map[uuid.UUID]*domain.BotConfig{}}
}

func (f *ConfigRepo) Get(ctx context.Context, botID uuid.UUID) (*domain.BotConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.Cfgs[botID]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *c
	return &cp, nil
}

func (f *ConfigRepo) Upsert(ctx context.Context, c *domain.BotConfig, updatedBy *uuid.UUID) (*domain.BotConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.ConflictOnUpsert {
		return nil, domain.ErrVersionConflict{}
	}
	cp := *c
	cp.Version = c.Version + 1
	f.Cfgs[c.BotID] = &cp
	return &cp, nil
}

func (f *ConfigRepo) EnsureDefault(ctx context.Context, botID uuid.UUID, texts, menus, flags, prices []byte) (*domain.BotConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := &domain.BotConfig{BotID: botID, Texts: texts, Menus: menus, FeatureFlags: flags, Prices: prices, Version: 0}
	f.Cfgs[botID] = c
	cp := *c
	return &cp, nil
}

// ---- TokenValidator ----

type Validator struct {
	Username string
	Err      error
	Calls    int
	LastTok  string
}

func (f *Validator) ValidateToken(ctx context.Context, token string) (string, error) {
	f.Calls++
	f.LastTok = token
	return f.Username, f.Err
}

// ---- RuntimeController ----

type Runtime struct {
	mu       sync.Mutex
	running  map[uuid.UUID]bool
	StartErr error
	Started  int
	Stopped  int
}

func NewRuntime() *Runtime { return &Runtime{running: map[uuid.UUID]bool{}} }

func (f *Runtime) Start(ctx context.Context, bot *domain.Bot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.StartErr != nil {
		return f.StartErr
	}
	f.Started++
	f.running[bot.ID] = true
	return nil
}

func (f *Runtime) Stop(ctx context.Context, botID uuid.UUID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Stopped++
	delete(f.running, botID)
	return nil
}

func (f *Runtime) IsRunning(botID uuid.UUID) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.running[botID]
}
