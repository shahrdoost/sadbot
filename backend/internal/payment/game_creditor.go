package payment

import (
	"context"
	"log"

	"sadbot/internal/domain"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// GameCreditor routes payment fulfillment to the correct game store based on
// the bot's template_key. Implements CoinCreditor so payment.Service can use
// it without changes.
type GameCreditor struct {
	db        *pgxpool.Pool
	dating    CoinCreditor // fallback for dating bots
	bombland  *postgres.BombLandRepo
	duel      *postgres.DuelRepo
}

func NewGameCreditor(db *pgxpool.Pool, dating CoinCreditor) *GameCreditor {
	return &GameCreditor{
		db:       db,
		dating:   dating,
		bombland: postgres.NewBombLandRepo(db),
		duel:     postgres.NewDuelRepo(db),
	}
}

// AddCoins credits game currency based on the bot's template.
// For dating bots it falls through to the dating CoinCreditor.
func (g *GameCreditor) AddCoins(ctx context.Context, botID uuid.UUID, userID int64, n int) (int, error) {
	var templateKey string
	err := g.db.QueryRow(ctx, `SELECT template_key FROM bots WHERE id=$1`, botID).Scan(&templateKey)
	if err != nil {
		log.Printf("[payment] game creditor: unknown bot %s: %v", botID, err)
		return 0, err
	}

	switch templateKey {
	case domain.TemplateBombLand:
		// n = money amount to credit
		if _, err := g.bombland.EnsurePlayer(ctx, botID, userID); err != nil {
			return 0, err
		}
		// Add money via direct SQL (repo doesn't have AddMoney method)
		_, err = g.db.Exec(ctx, `UPDATE bombland_players SET money = money + $3 WHERE bot_id=$1 AND user_id=$2`, botID, userID, n)
		return n, err

	case domain.TemplateDuel:
		// n = energy amount to credit
		if _, err := g.duel.EnsurePlayer(ctx, botID, userID); err != nil {
			return 0, err
		}
		err = g.duel.AddEnergy(ctx, botID, userID, n)
		return n, err

	default:
		// dating / welcome_menu / content — use dating creditor
		if g.dating != nil {
			return g.dating.AddCoins(ctx, botID, userID, n)
		}
		return 0, nil
	}
}
