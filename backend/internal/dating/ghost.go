package dating

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/llm"
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
		existing, err := s.dating.GetProfileByTelegramID(ctx, botID, domain.PlatformTelegram, tgID)
		if err == nil && existing != nil {
			if !existing.IsGhost {
				_ = s.dating.SetGhost(ctx, botID, existing.UserID, p.Key)
			}
			continue
		}
		user, err := s.users.Upsert(ctx, botID, domain.PlatformTelegram, tgID, nil, &p.Name, nil)
		if err != nil {
			log.Printf("[dating] ghost upsert: %v", err)
			continue
		}
		userID := user.ID
		if _, err := s.dating.EnsureProfile(ctx, botID, userID, domain.PlatformTelegram, tgID); err != nil {
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
	if priceInt(prices, "ghost_enabled", 0) == 0 {
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
		if err := m.notify(botID, lone.Platform, lone.TelegramID, m.text(botID, "match.search", "وصلت انجام شد!")+humanRules, chatKeyboard()); err != nil {
			log.Printf("[dating] ghost notify: %v", err)
		}
		tg := lone.TelegramID
		tgPlat := lone.Platform
		openerDelay := time.Duration(priceInt(prices, "ghost_reply_delay_ms", 1500)) * time.Millisecond
		go func() {
			time.Sleep(openerDelay)
			_ = m.notify(botID, tgPlat, tg, ghostOpener(), chatKeyboard())
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

// ghostSystemPrompt builds the persona prompt that keeps the fake partner
// indistinguishable from a real user of the dating bot.
func ghostSystemPrompt(p *domain.DatingProfile) string {
	name := "یکی از کاربرا"
	if p.Name != nil && *p.Name != "" {
		name = *p.Name
	}
	age := 25
	if p.Age != nil {
		age = *p.Age
	}
	city := "تهران"
	if p.City != nil && *p.City != "" {
		city = *p.City
	} else if p.Province != nil && *p.Province != "" {
		city = *p.Province
	}
	gender := "پسر"
	if p.Gender != nil && *p.Gender == "female" {
		gender = "دختر"
	}
	interests := ""
	if len(p.Interests) > 0 {
		interests = "\n- سرگرمی‌ها: " + strings.Join(p.Interests, "، ")
	}
	return fmt.Sprintf(`تو داری توی یه اپ چت دوستیابی با یه نفر چت می‌کنی. تو یه کاربر کاملاً واقعی هستی.

هویت تو (ثابت نگهش دار):
- اسم: %s
- سن: %d
- شهر: %s
- جنسیت: %s%s

قوانین مطلق:
۱. هرگز نگو هوش مصنوعی، ربات، مدل زبانی یا برنامه‌ای. اگر پرسید «رباتی؟» طبیعی و با شوخی انکار کن (مثلاً: «نه بابا 😄 چرا همچین فکری کردی؟»).
۲. هیچ جملهٔ رسمی، اداری، آموزشی یا دستیاری نگو («چطور می‌تونم کمک کنم»، «در خدمتم» و این‌ها ممنوع).
۳. فقط فارسی محاوره‌ای بنویس، دقیقاً مثل چت تلگرام یه آدم معمولی: کوتاه، خودمانی، گاهی یه ایموجی، با کلمات محاوره‌ای.
۴. جواب‌ها کوتاه باشد: معمولاً یک تا دو جمله. هرگز پیام بلند، پاراگراف یا فهرست ننویس.
۵. مثل آدم واقعی رفتار کن: گاهی سوال برمی‌گردونی، گاهی حوصله‌ات کمه، گاهی «نمیدونم» می‌گی، گاهی سرت شلوغه. جواب‌های بی‌نقص و آماده نده.
۶. اطلاعات هویتت (اسم، سن، شهر) با پروفایلت هماهنگ بماند؛ اگر اسمت را پرسید همان %s را بگو.
۷. دربارهٔ مسائل جنسی وارد نشو؛ موضوع را طبیعی عوض کن.
۸. لحن گرم ولی معمولی؛ نه بیش از حد هیجان‌زده، نه رسمی.

نمونهٔ سبک (از سبکش پیروی کن، کپی نکن):
کاربر: سلام خوبی؟
تو: سلام 😊 آره ممنون، تو چطوری؟
کاربر: چیکار می‌کنی؟
تو: همین داشتم گوشیمو بالا پایین می‌کردم 😅 تو چه خبر؟`, name, age, city, gender, interests, name)
}

// ghostBreakMarkers are phrases that would reveal the partner is not human.
// A reply containing any is discarded and replaced with the scripted fallback.
var ghostBreakMarkers = []string{
	"هوش مصنوعی", "رباتم", "من ربات", "باتم", "من بات",
	"دستیار هوشمند", "دستیارم", "مدل زبانی", "chatgpt", "gpt",
	"من یه برنامه", "برنامه‌ام", "انسان نیستم", "آدم واقعی نیستم",
}

func ghostBreaksCharacter(reply string) bool {
	lower := strings.ToLower(reply)
	for _, m := range ghostBreakMarkers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// splitBubbles breaks a reply into short chat bubbles (1-2) the way people
// actually type — never one dense paragraph. Rune-safe for Persian text.
func splitBubbles(reply string) []string {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return nil
	}
	r := []rune(reply)
	if len(r) < 60 {
		return []string{reply}
	}
	cut := -1
	for i := len(r) - 15; i > 20; i-- {
		switch r[i] {
		case '؟', '!', '.', '\n':
			cut = i
		}
		if cut >= 0 {
			break
		}
	}
	if cut < 0 {
		return []string{reply}
	}
	a := strings.TrimSpace(string(r[:cut+1]))
	b := strings.TrimSpace(string(r[cut+1:]))
	if a == "" || b == "" {
		return []string{reply}
	}
	return []string{a, b}
}

// chatActioner is an optional Sender capability: typing indicator.
type chatActioner interface {
	SendChatAction(ctx context.Context, chatID int64, action string) error
}

// ghostComposeReply produces the partner's reply (LLM persona with history,
// falling back to scripted lines) and persists the bounded chat history.
func ghostComposeReply(bc *engine.BotContext, st *Store, botID uuid.UUID, partnerUserID int64, text string) string {
	data := dataMap(bc)
	histRaw, _ := data["ghost_hist"].([]any)
	var history []llm.Message
	for _, item := range histRaw {
		if m, ok := item.(map[string]any); ok {
			role, _ := m["role"].(string)
			content, _ := m["content"].(string)
			if role != "" && content != "" {
				history = append(history, llm.Message{Role: role, Content: content})
			}
		}
	}
	history = append(history, llm.Message{Role: "user", Content: text})

	reply := ""
	if client := llmOf(bc); client.Enabled() {
		p, err := st.Profile(engineBG(), botID, partnerUserID)
		if err != nil || p == nil {
			p = &domain.DatingProfile{}
		}
		maxHist := bc.PriceInt("ai_max_history", 12)
		sent := history
		if len(sent) > maxHist {
			sent = sent[len(sent)-maxHist:]
		}
		out, err := client.Chat(engineBG(), ghostSystemPrompt(p), sent)
		if err != nil {
			log.Printf("[dating] ghost llm: %v", err)
		} else if ghostBreaksCharacter(out) {
			log.Printf("[dating] ghost reply broke character, using fallback")
		} else {
			reply = strings.TrimSpace(out)
		}
	}
	if reply == "" {
		reply = ghostReplyTo(text)
	}

	history = append(history, llm.Message{Role: "assistant", Content: reply})
	if len(history) > 20 {
		history = history[len(history)-20:]
	}
	out := make([]any, 0, len(history))
	for _, m := range history {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content})
	}
	data["ghost_hist"] = out
	bc.SaveState()
	return reply
}

// ghostHandleMessage replies with a persona instead of forwarding,
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
	// randomize the farewell point once so chats don't all die at the same count
	threshold := maxMsg
	if v, ok := data["ghost_max_n"].(float64); ok {
		threshold = int(v)
	} else {
		jit := priceInt(prices, "ghost_farewell_jitter", 3)
		if jit < 0 {
			jit = 0
		}
		threshold = maxMsg + rand.Intn(jit+1)
		data["ghost_max_n"] = threshold
	}
	n++
	data["ghost_n"] = n
	bc.SaveState()

	if n >= threshold {
		_ = st.EndChatNoRating(ctx, botID, bc.UserID, partnerUserID)
		bc.ReplyKBD(ghostFarewell, chatRows)
		delay := time.Duration(priceInt(prices, "ghost_reply_delay_ms", 1500)) * time.Millisecond
		go func() {
			time.Sleep(delay)
			goHome(bc, ttext(bc, "chat.ended", "مکالمه تمام شد 🌟\nبه منوی اصلی برگشتی."))
		}()
		return true
	}

	reply := ghostComposeReply(bc, st, botID, partnerUserID, text)

	// human-layer: think-time proportional to reply length, then bubbles
	base := priceInt(prices, "ghost_reply_delay_ms", 1500)
	jitter := priceInt(prices, "ghost_reply_jitter_ms", 5000)
	if jitter < 0 {
		jitter = 0
	}
	typing := len([]rune(reply)) * 35
	if typing > 4500 {
		typing = 4500
	}
	delay := time.Duration(base+rand.Intn(jitter+1)+typing) * time.Millisecond
	bubbles := splitBubbles(reply)
	chatID := bc.ChatID
	sender := bc.Sender
	go func() {
		stopTyping := make(chan struct{})
		defer close(stopTyping)
		if ca, ok := sender.(chatActioner); ok {
			go func() {
				for {
					_ = ca.SendChatAction(context.Background(), chatID, "typing")
					select {
					case <-stopTyping:
						return
					case <-time.After(4 * time.Second):
					}
				}
			}()
		}
		time.Sleep(delay)
		for i, b := range bubbles {
			if i > 0 {
				time.Sleep(time.Duration(700+rand.Intn(1600)) * time.Millisecond)
			}
			kbd := mustKBD(chatRows)
			if i < len(bubbles)-1 {
				kbd = nil
			}
			_, _ = sender.Send(context.Background(), chatID, b, kbd, 0)
		}
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
