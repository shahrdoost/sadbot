package postgres

import (
	"context"
	"errors"
	"fmt"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type BotRepo struct {
	db *pgxpool.Pool
}

func NewBotRepo(db *pgxpool.Pool) *BotRepo { return &BotRepo{db: db} }

const botCols = `id, name, telegram_username, token_encrypted, template_key, status, last_error, token_validated_at, created_by, created_at, updated_at`

func scanBot(row pgx.Row) (*domain.Bot, error) {
	b := &domain.Bot{}
	err := row.Scan(&b.ID, &b.Name, &b.TelegramUsername, &b.TokenEncrypted, &b.TemplateKey, &b.Status, &b.LastError, &b.TokenValidatedAt, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *BotRepo) Create(ctx context.Context, name string, username *string, tokenEnc []byte, templateKey string, createdBy uuid.UUID) (*domain.Bot, error) {
	return scanBot(r.db.QueryRow(ctx, `
		INSERT INTO bots (name, telegram_username, token_encrypted, template_key, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+botCols,
		name, username, tokenEnc, templateKey, createdBy))
}

func (r *BotRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.Bot, error) {
	b, err := scanBot(r.db.QueryRow(ctx, `SELECT `+botCols+` FROM bots WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("bot not found")
	}
	return b, err
}

func (r *BotRepo) List(ctx context.Context) ([]domain.Bot, error) {
	rows, err := r.db.Query(ctx, `SELECT `+botCols+` FROM bots ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Bot
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// ListByAdmin returns all bots for a superadmin, or only granted bots otherwise.
func (r *BotRepo) ListByAdmin(ctx context.Context, adminID uuid.UUID, isSuper bool) ([]domain.Bot, error) {
	if isSuper {
		return r.List(ctx)
	}
	rows, err := r.db.Query(ctx, `
		SELECT b.id, b.name, b.telegram_username, b.token_encrypted, b.template_key, b.status, b.last_error, b.token_validated_at, b.created_by, b.created_at, b.updated_at
		FROM bots b JOIN admin_bot_access a ON a.bot_id = b.id
		WHERE a.admin_id = $1 ORDER BY b.created_at`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Bot
	for rows.Next() {
		b, err := scanBot(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *BotRepo) Update(ctx context.Context, id uuid.UUID, name *string, username *string, templateKey *string) (*domain.Bot, error) {
	b, err := scanBot(r.db.QueryRow(ctx, `
		UPDATE bots SET
			name = COALESCE($2, name),
			telegram_username = COALESCE($3, telegram_username),
			template_key = COALESCE($4, template_key),
			updated_at = now()
		WHERE id = $1
		RETURNING `+botCols,
		id, name, username, templateKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("bot not found")
	}
	return b, err
}

func (r *BotRepo) UpdateToken(ctx context.Context, id uuid.UUID, tokenEnc []byte, username *string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE bots SET token_encrypted = $2, telegram_username = $3, token_validated_at = now(), updated_at = now()
		WHERE id = $1`, id, tokenEnc, username)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("bot not found")
	}
	return nil
}

func (r *BotRepo) SetStatus(ctx context.Context, id uuid.UUID, status string, lastError *string) error {
	_, err := r.db.Exec(ctx, `UPDATE bots SET status = $2, last_error = $3, updated_at = now() WHERE id = $1`, id, status, lastError)
	return err
}

func (r *BotRepo) Delete(ctx context.Context, id uuid.UUID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM bots WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("bot not found")
	}
	return nil
}

// ListRunningByTemplate returns ids of running bots with the given template.
func (r *BotRepo) ListRunningByTemplate(ctx context.Context, templateKey string) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id FROM bots WHERE status = 'running' AND template_key = $1`, templateKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// --- config ---

type ConfigRepo struct {
	db *pgxpool.Pool
}

func NewConfigRepo(db *pgxpool.Pool) *ConfigRepo { return &ConfigRepo{db: db} }

const configCols = `bot_id, texts, menus, feature_flags, prices, version, updated_by, updated_at`

func (r *ConfigRepo) Get(ctx context.Context, botID uuid.UUID) (*domain.BotConfig, error) {
	c := &domain.BotConfig{}
	err := r.db.QueryRow(ctx, `SELECT `+configCols+` FROM bot_configs WHERE bot_id = $1`, botID).
		Scan(&c.BotID, &c.Texts, &c.Menus, &c.FeatureFlags, &c.Prices, &c.Version, &c.UpdatedBy, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("config not found")
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

// Upsert with optimistic locking on version: 0 means insert-if-missing (or force set on create).
func (r *ConfigRepo) Upsert(ctx context.Context, c *domain.BotConfig, updatedBy *uuid.UUID) (*domain.BotConfig, error) {
	out := &domain.BotConfig{}
	// $2..$5 are json.RawMessage ([]byte): cast explicitly, otherwise pgx
	// sends them as bytea and jsonb stores them as a JSON string, which breaks
	// every downstream json.Unmarshal in the config merge.
	err := r.db.QueryRow(ctx, `
		INSERT INTO bot_configs (bot_id, texts, menus, feature_flags, prices, version, updated_by)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5::jsonb, 1, $6)
		ON CONFLICT (bot_id) DO UPDATE SET
			texts = EXCLUDED.texts, menus = EXCLUDED.menus,
			feature_flags = EXCLUDED.feature_flags, prices = EXCLUDED.prices,
			version = bot_configs.version + 1, updated_by = EXCLUDED.updated_by, updated_at = now()
		WHERE bot_configs.version = $7
		RETURNING `+configCols,
		c.BotID, c.Texts, c.Menus, c.FeatureFlags, c.Prices, updatedBy, c.Version,
	).Scan(&out.BotID, &out.Texts, &out.Menus, &out.FeatureFlags, &out.Prices, &out.Version, &out.UpdatedBy, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrVersionConflict{}
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}

// EnsureDefault creates an empty config row if missing (used after bot create).
func (r *ConfigRepo) EnsureDefault(ctx context.Context, botID uuid.UUID, texts, menus, flags, prices []byte) (*domain.BotConfig, error) {
	out := &domain.BotConfig{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO bot_configs (bot_id, texts, menus, feature_flags, prices)
		VALUES ($1, $2::jsonb, $3::jsonb, $4::jsonb, $5::jsonb)
		ON CONFLICT (bot_id) DO NOTHING
		RETURNING `+configCols,
		botID, texts, menus, flags, prices,
	).Scan(&out.BotID, &out.Texts, &out.Menus, &out.FeatureFlags, &out.Prices, &out.Version, &out.UpdatedBy, &out.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.Get(ctx, botID)
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
