package postgres

import (
	"context"
	"errors"

	"sadbot/internal/domain"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ModerationRepo struct {
	db *pgxpool.Pool
}

func NewModerationRepo(db *pgxpool.Pool) *ModerationRepo { return &ModerationRepo{db: db} }

const confessionCols = `id, bot_id, user_id, text, status, created_at, decided_at, decided_by, published_msg_id`

func scanConfession(row pgx.Row) (*domain.Confession, error) {
	c := &domain.Confession{}
	err := row.Scan(&c.ID, &c.BotID, &c.UserID, &c.Text, &c.Status, &c.CreatedAt, &c.DecidedAt, &c.DecidedBy, &c.PublishedMsgID)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ModerationRepo) InsertConfession(ctx context.Context, botID uuid.UUID, userID int64, text string) (*domain.Confession, error) {
	return scanConfession(r.db.QueryRow(ctx, `
		INSERT INTO confessions (bot_id, user_id, text)
		VALUES ($1, $2, $3)
		RETURNING `+confessionCols,
		botID, userID, text))
}

// ListConfessions returns one page plus the total count for the filter.
func (r *ModerationRepo) ListConfessions(ctx context.Context, botID uuid.UUID, status string, page, perPage int) ([]domain.Confession, int, error) {
	if status == "" {
		status = domain.ConfessionPending
	}
	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM confessions WHERE bot_id = $1 AND status = $2`,
		botID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT c.id, c.bot_id, c.user_id, u.username, u.first_name, c.text, c.status, c.created_at, c.decided_at, c.decided_by, c.published_msg_id
		FROM confessions c
		LEFT JOIN bot_end_users u ON u.id = c.user_id AND u.bot_id = c.bot_id
		WHERE c.bot_id = $1 AND c.status = $2
		ORDER BY c.created_at DESC
		LIMIT $3 OFFSET $4`,
		botID, status, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.Confession
	for rows.Next() {
		c := &domain.Confession{}
		if err := rows.Scan(&c.ID, &c.BotID, &c.UserID, &c.Username, &c.FirstName, &c.Text, &c.Status, &c.CreatedAt, &c.DecidedAt, &c.DecidedBy, &c.PublishedMsgID); err != nil {
			return nil, 0, err
		}
		out = append(out, *c)
	}
	return out, total, rows.Err()
}

// DecideConfession CASes pending → approved/rejected and records the admin.
func (r *ModerationRepo) DecideConfession(ctx context.Context, botID uuid.UUID, id int64, adminID uuid.UUID, approve bool) (*domain.Confession, error) {
	to := domain.ConfessionRejected
	if approve {
		to = domain.ConfessionApproved
	}
	return scanConfession(r.db.QueryRow(ctx, `
		UPDATE confessions SET status = $3, decided_at = now(), decided_by = $4
		WHERE id = $1 AND bot_id = $2 AND status = 'pending'
		RETURNING `+confessionCols,
		id, botID, to, adminID))
}

// MarkConfessionPublished stores the channel message id after approval post.
func (r *ModerationRepo) MarkConfessionPublished(ctx context.Context, id int64, msgID int64) error {
	_, err := r.db.Exec(ctx, `UPDATE confessions SET published_msg_id = $2 WHERE id = $1`, id, msgID)
	return err
}

// --- support tickets ---

const ticketCols = `id, bot_id, user_id, message, admin_reply, status, created_at, replied_at`

func scanTicket(row pgx.Row) (*domain.SupportTicket, error) {
	t := &domain.SupportTicket{}
	err := row.Scan(&t.ID, &t.BotID, &t.UserID, &t.Message, &t.AdminReply, &t.Status, &t.CreatedAt, &t.RepliedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func (r *ModerationRepo) InsertTicket(ctx context.Context, botID uuid.UUID, userID int64, message string) (*domain.SupportTicket, error) {
	return scanTicket(r.db.QueryRow(ctx, `
		INSERT INTO support_tickets (bot_id, user_id, message)
		VALUES ($1, $2, $3)
		RETURNING `+ticketCols,
		botID, userID, message))
}

func (r *ModerationRepo) ListTickets(ctx context.Context, botID uuid.UUID, status string, page, perPage int) ([]domain.SupportTicket, int, error) {
	if status == "" {
		status = domain.TicketOpen
	}
	var total int
	if err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM support_tickets WHERE bot_id = $1 AND status = $2`,
		botID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT t.id, t.bot_id, t.user_id, u.username, u.first_name, t.message, t.admin_reply, t.status, t.created_at, t.replied_at
		FROM support_tickets t
		LEFT JOIN bot_end_users u ON u.id = t.user_id AND u.bot_id = t.bot_id
		WHERE t.bot_id = $1 AND t.status = $2
		ORDER BY t.created_at DESC
		LIMIT $3 OFFSET $4`,
		botID, status, perPage, (page-1)*perPage)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []domain.SupportTicket
	for rows.Next() {
		t := &domain.SupportTicket{}
		if err := rows.Scan(&t.ID, &t.BotID, &t.UserID, &t.Username, &t.FirstName, &t.Message, &t.AdminReply, &t.Status, &t.CreatedAt, &t.RepliedAt); err != nil {
			return nil, 0, err
		}
		t.Messages = []domain.TicketMessage{}
		out = append(out, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	// one extra round trip for the page's threads instead of N per ticket
	if len(out) > 0 {
		ids := make([]int64, len(out))
		for i := range out {
			ids[i] = out[i].ID
		}
		threads, err := r.ticketThreads(ctx, ids)
		if err != nil {
			return nil, 0, err
		}
		for i := range out {
			if m := threads[out[i].ID]; m != nil {
				out[i].Messages = m
			}
		}
	}
	return out, total, nil
}

// ticketThreads loads the threads of several tickets at once, keyed by ticket id.
func (r *ModerationRepo) ticketThreads(ctx context.Context, ids []int64) (map[int64][]domain.TicketMessage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ticket_id, id, sender, body, created_at FROM support_ticket_messages
		WHERE ticket_id = ANY($1) ORDER BY ticket_id, created_at, id`,
		ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[int64][]domain.TicketMessage, len(ids))
	for rows.Next() {
		var ticketID int64
		var m domain.TicketMessage
		if err := rows.Scan(&ticketID, &m.ID, &m.Sender, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out[ticketID] = append(out[ticketID], m)
	}
	return out, rows.Err()
}

// ReplyTicket records the admin reply (CAS open|answered → answered) and
// appends it to the thread in the same transaction, so the panel and the bot
// never disagree about what was said.
func (r *ModerationRepo) ReplyTicket(ctx context.Context, botID uuid.UUID, id int64, reply string) (*domain.SupportTicket, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	t, err := scanTicket(tx.QueryRow(ctx, `
		UPDATE support_tickets SET admin_reply = $3, status = 'answered', replied_at = now()
		WHERE id = $1 AND bot_id = $2 AND status IN ('open','answered')
		RETURNING `+ticketCols,
		id, botID, reply))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO support_ticket_messages (ticket_id, sender, body) VALUES ($1, 'admin', $2)`,
		id, reply); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	t.Messages = []domain.TicketMessage{}
	return t, nil
}

// CloseTicket closes a ticket (any non-closed state).
func (r *ModerationRepo) CloseTicket(ctx context.Context, botID uuid.UUID, id int64) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE support_tickets SET status = 'closed'
		WHERE id = $1 AND bot_id = $2 AND status <> 'closed'`,
		id, botID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// TicketByID loads one ticket regardless of status.
func (r *ModerationRepo) TicketByID(ctx context.Context, botID uuid.UUID, id int64) (*domain.SupportTicket, error) {
	return scanTicket(r.db.QueryRow(ctx, `
		SELECT `+ticketCols+` FROM support_tickets WHERE id = $1 AND bot_id = $2`,
		id, botID))
}

// OpenTicketForUser returns the user's most recent ticket that is not closed,
// or nil when they have none. The bot uses it to keep «پشتیبانی» in one thread
// instead of opening a second ticket on every visit.
func (r *ModerationRepo) OpenTicketForUser(ctx context.Context, botID uuid.UUID, userID int64) (*domain.SupportTicket, error) {
	t, err := scanTicket(r.db.QueryRow(ctx, `
		SELECT `+ticketCols+` FROM support_tickets
		WHERE bot_id = $1 AND user_id = $2 AND status <> 'closed'
		ORDER BY created_at DESC
		LIMIT 1`,
		botID, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// TicketMessages is the thread of one ticket, oldest first.
func (r *ModerationRepo) TicketMessages(ctx context.Context, ticketID int64) ([]domain.TicketMessage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, sender, body, created_at FROM support_ticket_messages
		WHERE ticket_id = $1 ORDER BY created_at, id`,
		ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TicketMessage{}
	for rows.Next() {
		var m domain.TicketMessage
		if err := rows.Scan(&m.ID, &m.Sender, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AddUserMessage appends a user turn to the thread and puts the ticket back in
// the admins' queue (answered → open). Returns false when the ticket is closed.
func (r *ModerationRepo) AddUserMessage(ctx context.Context, botID uuid.UUID, id int64, body string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE support_tickets SET status = 'open'
		WHERE id = $1 AND bot_id = $2 AND status <> 'closed'`,
		id, botID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO support_ticket_messages (ticket_id, sender, body) VALUES ($1, 'user', $2)`,
		id, body); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}
