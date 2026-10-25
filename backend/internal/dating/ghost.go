package dating

import (
	"context"
	"log"
	"math/rand"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"

	"github.com/google/uuid"
)

// Ghost cold-start (phase 3, simplified kpkchat parity): scripted Persian
// personas that give lone searchers a partner when the network is empty.
// Intensity adapts to the number of recently-active real users.

type ghostPersona struct {
	Key      string
	Name     string
	Gender   string
	Age      int
	Province string
	City     string
}

var ghostPersonas = []ghostPersona{
	{"mina", "مینا", "female", 24, "تهران", "تهران"},
	{"sara", "سارا", "female", 22, "اصفهان", "اصفهان"},
	{"negin", "نگین", "female", 27, "فارس", "شیراز"},
	{"parisa", "پریسا", "female", 21, "خراسان رضوی", "مشهد"},
	{"reza", "رضا", "male", 26, "تهران", "تهران"},
	{"kian", "کیان", "male", 24, "اصفهان", "اصفهان"},
	{"bagher", "باقر", "male", 28, "فارس", "شیراز"},
	{"navid", "نوید", "male", 23, "خراسان رضوی", "مشهد"},
	{"omid", "امید", "male", 27, "البرز", "کرج"},
	{"delara", "دلارا", "female", 25, "آذربایجان شرقی", "تبریز"},
}

// ghostLevel maps active real users to injection intensity (kpkchat tiers).
func ghostLevel(activeReal int) int {
	switch {
	case activeReal >= 50:
		return 0 // network healthy — ghosts off
	case activeReal >= 10:
		return 1 // browse-only tier (no search injection here)
	case activeReal >= 1:
		return 2 // 50% inject chance per tick
	default:
		return 3 // empty network — inject every tick
	}
}

// EnsureGhosts seeds the per-bot ghost pool up to n and returns idle ghosts.
func (s *Store) EnsureGhosts(ctx context.Context, botID uuid.UUID, n int) ([]domain.DatingProfile, error) {
	if n > len(ghostPersonas) {
		n = len(ghostPersonas)
	}
	count, err := s.dating.CountGhosts(ctx, botID)
	if err != nil {
		return nil, err
	}
	for i := count; i < n; i++ {
		p := ghostPersonas[i]
		tgID := int64(-10001 - i)
		existing, err := s.dating.GetProfileByTelegramID(ctx, botID, tgID)
		if err == nil && existing != nil {
			if !existing.IsGhost {
				_ = s.dating.SetGhost(ctx, botID, existing.UserID, p.Key)
			}
			continue
		}
		user, err := s.users.Upsert(ctx, botID, tgID, nil, &p.Name, nil)
		if err != nil {
			log.Printf("[dating] ghost upsert: %v", err)
			continue
		}
		userID := user.ID
		if _, err := s.dating.EnsureProfile(ctx, botID, userID, tgID); err != nil {
			continue
		}
		name, gender, age, province, city := p.Name, p.Gender, p.Age, p.Province, p.City
		if _, err := s.dating.UpdateProfile(ctx, botID, userID, postgres.ProfileUpdate{
			Name: &name, Gender: &gender, Age: &age, Province: &province, City: &city,
		}); err != nil {
			continue
		}
		_ = s.dating.SetGhost(ctx, botID, userID, p.Key)
	}
	return s.dating.FreeGhosts(ctx, botID, n)
}

// priceInt reads an int from a merged prices map with a fallback.
func priceInt(prices map[string]any, key string, def int) int {
	if prices == nil {
		return def
	}
	switch v := prices[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	}
	return def
}

// injectGhosts pairs lone searchers (waiting past the configured delay) with
// free ghost personas. Intensity adapts to the active real-user count.
func (m *Matcher) injectGhosts(ctx context.Context, botID uuid.UUID) {
	prices := m.prices(botID)
	if priceInt(prices, "ghost_enabled", 1) == 0 {
		return
	}
	if priceInt(prices, "ghost_force", 0) == 0 {
		active, err := m.store.Repo().CountRealActive(ctx, botID, 10*time.Minute)
		if err != nil {
			log.Printf("[dating] ghost count-active: %v", err)
			return
		}
		level := ghostLevel(active)
		if level <= 1 {
			log.Printf("[dating] ghost skip bot=%s active=%d level=%d", botID, active, level)
			return
		}
		if level == 2 && rand.Float64() > 0.5 {
			return
		}
	}
	delay := time.Duration(priceInt(prices, "ghost_delay_seconds", 15)) * time.Second
	poolSize := priceInt(prices, "ghost_pool_size", 5)

	searchers, err := m.store.LoadSearchers(ctx, botID)
	if err != nil {
		log.Printf("[dating] ghost load-searchers: %v", err)
		return
	}
	var lones []Searcher
	for _, s := range searchers {
		if time.Since(s.CreatedAt) >= delay {
			lones = append(lones, s)
		}
	}
	if len(lones) == 0 {
		return
	}
	ghosts, err := m.store.EnsureGhosts(ctx, botID, poolSize)
	if err != nil {
		log.Printf("[dating] ghost ensure-pool: %v", err)
		return
	}
	if len(ghosts) == 0 {
		log.Printf("[dating] ghost empty-pool bot=%s searchers=%d lones=%d", botID, len(searchers), len(lones))
		return
	}
	log.Printf("[dating] ghost inject bot=%s lones=%d ghosts=%d", botID, len(lones), len(ghosts))
	for _, lone := range lones {
		if len(ghosts) == 0 {
			return
		}
		// prefer a gender-compatible ghost when the searcher filtered
		chosen := 0
		if lone.Filter != "" && lone.Filter != "any" {
			found := -1
			for j, g := range ghosts {
				if derefStr(g.Gender) == lone.Filter {
					found = j
					break
				}
			}
			if found < 0 {
				continue
			}
			chosen = found
		}
		g := ghosts[chosen]
		ghosts = append(ghosts[:chosen], ghosts[chosen+1:]...)

		ok, err := m.store.ConnectUsers(ctx, botID, lone.UserID, g.UserID)
		if err != nil || !ok {
			continue
		}
		_ = m.store.SetScene(ctx, botID.String(), lone.UserID, "chat")
		if err := m.notify(botID, lone.TelegramID, m.text(botID, "match.search", "وصلت انجام شد!")+humanRules, chatKeyboard()); err != nil {
			log.Printf("[dating] ghost notify: %v", err)
		}
		tg := lone.TelegramID
		openerDelay := time.Duration(priceInt(prices, "ghost_reply_delay_ms", 1500)) * time.Millisecond
		go func() {
			time.Sleep(openerDelay)
			_ = m.notify(botID, tg, ghostOpener(), chatKeyboard())
		}()
	}
}

// --- scripted replies ---

func ghostOpener() string {
	ops := []string{
		"سلام! خوش اومدی 😊 حالت چطوره؟",
		"سلام! بالاخره یه نفر پیدا شد 🙂 چه خبرا؟",
		"هی! خوش گذشت روزت؟",
	}
	return ops[rand.Intn(len(ops))]
}

var ghostGeneric = []string{"چه خبرا؟", "بگو ببینم 🙂", "جالبه، ادامه بده", "چیکار می‌کنی؟", "خب بعدش چی شد؟"}
var ghostQuestion = []string{"فکر کنم آره، تو چه فکری هستی؟", "سؤال خوبیه 🙂 نظر من اینه که بستگی داره", "نه مطمئن نیستم… تو چی میگی؟"}
var ghostPositive = []string{"چه عالی! خوشحالم 😊", "آفرین، همین انرژی رو نگه دار 🙂"}
var ghostNegative = []string{"اوه، متأسفم… چی شده؟", "امیدوارم زودتر بهتر بشی 🌸"}
var ghostBye = []string{"باشه، وقتی خواستی برگرد 🙂", "خوش گذشت، مراقب خودت باش 👋"}
var ghostFarewell = "من باید برم، کسی منتظرمه. قدمت گرم بود! 👋"

func ghostReplyTo(text string) string {
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "خداحافظ") || strings.Contains(lower, "فعلا") || strings.Contains(lower, "بای"):
		return ghostBye[rand.Intn(len(ghostBye))]
	case strings.Contains(text, "?") || strings.Contains(text, "؟") ||
		strings.HasPrefix(lower, "چه") || strings.HasPrefix(lower, "چرا") || strings.HasPrefix(lower, "چطور"):
		return ghostQuestion[rand.Intn(len(ghostQuestion))]
	case strings.Contains(lower, "خوب") || strings.Contains(lower, "عالی") || strings.Contains(lower, "ممنون"):
		return ghostPositive[rand.Intn(len(ghostPositive))]
	case strings.Contains(lower, "خسته") || strings.Contains(lower, "ناراحت") || strings.Contains(lower, "غم") || strings.Contains(lower, "حالم خوب نیست"):
		return ghostNegative[rand.Intn(len(ghostNegative))]
	default:
		return ghostGeneric[rand.Intn(len(ghostGeneric))]
	}
}

// partnerIsGhost reports whether the chat partner is a ghost persona.
func partnerIsGhost(ctx context.Context, st *Store, botID uuid.UUID, partnerUserID int64) bool {
	p, err := st.Profile(ctx, botID, partnerUserID)
	if err != nil {
		return false
	}
	return p.IsGhost
}

// ghostHandleMessage replies with a scripted persona instead of forwarding,
// ending the chat after the configured message budget (no rating).
func ghostHandleMessage(bc *engine.BotContext, st *Store, botID uuid.UUID, partnerUserID int64, text string) bool {
	ctx := engineBG()
	var prices map[string]any
	if pricesFn, ok := bc.Data["pricesof"].(func(string) map[string]any); ok {
		prices = pricesFn(botID.String())
	}
	maxMsg := priceInt(prices, "ghost_max_messages", 12)

	data := dataMap(bc)
	n := 0
	switch v := data["ghost_n"].(type) {
	case float64:
		n = int(v)
	case int:
		n = v
	}
	n++
	data["ghost_n"] = n
	bc.SaveState()

	if n >= maxMsg {
		_ = st.EndChatNoRating(ctx, botID, bc.UserID, partnerUserID)
		bc.ReplyKBD(ghostFarewell, chatRows)
		delay := time.Duration(priceInt(prices, "ghost_reply_delay_ms", 1500)) * time.Millisecond
		go func() {
			time.Sleep(delay)
			goHome(bc, ttext(bc, "chat.ended", "مکالمه تمام شد 🌟\nبه منوی اصلی برگشتی."))
		}()
		return true
	}

	reply := ghostReplyTo(text)
	base := priceInt(prices, "ghost_reply_delay_ms", 1500)
	jitter := priceInt(prices, "ghost_reply_jitter_ms", 5000)
	if jitter < 0 {
		jitter = 0
	}
	delay := time.Duration(base+rand.Intn(jitter+1)) * time.Millisecond
	chatID := bc.ChatID
	sender := bc.Sender
	go func() {
		time.Sleep(delay)
		_, _ = sender.Send(context.Background(), chatID, reply, mustKBD(chatRows), 0)
	}()
	return true
}

// EndChatNoRating ends both sides of a ghost chat without pending ratings.
func (s *Store) EndChatNoRating(ctx context.Context, botID uuid.UUID, a, b int64) error {
	_ = s.dating.ClearPendingRate(ctx, botID, a)
	_ = s.dating.ClearPendingRate(ctx, botID, b)
	if _, err := s.dating.EndChat(ctx, botID, a); err != nil {
		return err
	}
	_, err := s.dating.EndChat(ctx, botID, b)
	return err
}

// endGhostChatFromProfile ends the human's current ghost chat (partner resolved
// from the profile). Used by force-end paths.
func endGhostChatFromProfile(bc *engine.BotContext, st *Store, botID uuid.UUID, humanID, partnerID int64) {
	ctx := engineBG()
	_ = st.EndChatNoRating(ctx, botID, humanID, partnerID)
	goHome(bc, ttext(bc, "chat.ended", "مکالمه تمام شد 🌟\nبه منوی اصلی برگشتی."))
}
