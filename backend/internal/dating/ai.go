package dating

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"sadbot/internal/engine"
	"sadbot/internal/llm"
)

// AI companion scene (phase 3). Honest by design: the bot always discloses
// it is a smart assistant — never pretends to be a human (no identity
// deception, unlike kpkchat's original personas).

const aiDisclosure = "🤖 من یک دستیار هوشمندم (هوش مصنوعی)، نه کاربر واقعی."

var aiIntro = aiDisclosure + "\n\n" +
	"اینجا می‌تونی حرف بزنی، سوال بپرسی یا نظرت رو بگی.\n" +
	"🎁 پیام‌های اول رایگانه؛ بعدش ۱ سکه (اشتراک پریموم = نامحدود).\n\n" +
	"هر وقت خواستی: 🏠 منوی اصلی"

var aiRows = [][]string{
	{"🏠 منوی اصلی"},
}

// aiSystemPrompt: warm Persian companion, explicitly AI, short answers.
const aiSystemPrompt = `تو «آریا» هستی؛ دستیار هوشمند، صمیمی و مهربان این بات دوستیابی.
قوانین سخت:
- تو هوش مصنوعی هستی؛ هرگز ادعای انسان بودن نکن و خاطره یا تجربهٔ دروغین نساز.
- پاسخ‌ها کوتاه باشد (۱ تا ۳ جمله)، به فارسی روان و خودمانی.
- اگر سوال شخصی/حساس (سلامت، حقوقی، بحران) پرسیده شد، صادقانه بگو تخصص نداری و به متخصص مراجعه کن.
- دربارهٔ تن/محتوای جنسی صحبت نکن.
- کاربر را به استفادهٔ بیشتر از سکه یا خرید تشویق نکن.`

// aiHint appends simple human-layer context (question / mood detection).
func aiHint(userText string) string {
	lower := strings.ToLower(userText)
	var hints []string
	if strings.Contains(userText, "?") || strings.Contains(userText, "؟") ||
		strings.HasPrefix(lower, "چه") || strings.HasPrefix(lower, "چرا") ||
		strings.HasPrefix(lower, "کی") || strings.HasPrefix(lower, "کجا") || strings.HasPrefix(lower, "چطور") {
		hints = append(hints, "کاربر سوال پرسیده؛ پاسخ مستقیم و کوتاه بده.")
	}
	if strings.Contains(lower, "خوب") || strings.Contains(lower, "عالی") || strings.Contains(lower, "ممنون") {
		hints = append(hints, "حال کاربر خوب است؛ شاد باش.")
	}
	if strings.Contains(lower, "خسته") || strings.Contains(lower, "حالم خوب نیست") || strings.Contains(lower, "ناراحت") || strings.Contains(lower, "غم") {
		hints = append(hints, "کاربر حال خوبی ندارد؛ همدلی کن.")
	}
	if len(hints) == 0 {
		return ""
	}
	return "\n" + strings.Join(hints, "\n")
}

// aiFallbackReplies keep the scene usable without an LLM key.
var aiFallbackReplies = []string{
	"چه حرف جالبی! بیشتر بگو 🙂",
	"متوجه شدم. از نظرت چه چیزی مهمه؟",
	"آره، حق با توعه. چیز دیگه‌ای هست که بخوای بگی؟",
	"جالبه! می‌خوای بیشتر توضیح بدی؟",
	"باشه، شنیدم 😊",
}

func llmOf(bc *engine.BotContext) *llm.Client {
	c, _ := bc.Data["llm"].(*llm.Client)
	return c
}

// aiEnabled reports the panel's ai_enabled switch (prices key, default on).
func aiEnabled(bc *engine.BotContext) bool { return bc.PriceInt("ai_enabled", 1) != 0 }

// startAI enters the companion scene.
func startAI(bc *engine.BotContext) bool {
	if !aiEnabled(bc) {
		bc.Reply("🤖 دستیار هوشمند فعلاً غیرفعال است. بعداً دوباره امتحان کن.")
		return true
	}
	bc.State["scene"] = "ai"
	bc.State["step"] = ""
	bc.SaveState()
	bc.ReplyKBD(ttext(bc, "ai.intro", aiIntro), aiRows)
	return true
}

// handleAIText processes one user message in the AI scene.
func handleAIText(bc *engine.BotContext, st *Store, text string) bool {
	// home-menu labels are routed by the central guard in HandleMessage
	if !aiEnabled(bc) {
		goHome(bc, "🤖 دستیار هوشمند فعلاً غیرفعال است.")
		return true
	}
	ctx := engineBG()
	botID := botUUID(bc)

	// paywall: free quota then 1 coin (subscribers unlimited)
	if !hasSubscription(bc, botID) {
		p, err := st.Profile(ctx, botID, bc.UserID)
		if err != nil {
			bc.ReplyKBD("خطا. /start بزن.", aiRows)
			return true
		}
		free := bc.PriceInt("ai_free_messages", 20)
		if p.AIFreeUsed >= free {
			cost := bc.PriceInt("ai_message_cost", 1)
			ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
			if err != nil || !ok {
				bc.ReplyKBDStyled(fmt.Sprintf(
					"پیام‌های رایگانت تموم شد. هر پیام %d سکه‌ست یا با اشتراک پریموم نامحدوده.\nاز «💰 سکه» سکه بخر یا برگرد.",
					cost), homeKB)
				return true
			}
		}
		if _, err := st.Repo().IncAIFree(ctx, botID, bc.UserID); err != nil {
			log.Printf("[dating] ai counter: %v", err)
		}
	} else {
		_, _ = st.Repo().IncAIFree(ctx, botID, bc.UserID)
	}

	// history in state (bounded)
	data := dataMap(bc)
	histRaw, _ := data["ai_hist"].([]any)
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
	client := llmOf(bc)
	if client.Enabled() {
		maxHist := bc.PriceInt("ai_max_history", 12)
		sent := history
		if len(sent) > maxHist {
			sent = sent[len(sent)-maxHist:]
		}
		out, err := client.Chat(ctx, aiSystemPrompt+aiHint(text), sent)
		if err != nil {
			log.Printf("[dating] llm: %v", err)
		}
		reply = out
	}
	if reply == "" {
		reply = aiFallbackReply()
	}

	// persist bounded history
	history = append(history, llm.Message{Role: "assistant", Content: reply})
	if len(history) > 24 {
		history = history[len(history)-24:]
	}
	out := make([]any, 0, len(history))
	for _, m := range history {
		out = append(out, map[string]any{"role": m.Role, "content": m.Content})
	}
	data["ai_hist"] = out
	bc.SaveState()

	// human-layer typing delay (non-blocking)
	delay := time.Duration(700+rand.Intn(1800)) * time.Millisecond
	go func() {
		time.Sleep(delay)
		_, _ = bc.Sender.Send(context.Background(), bc.ChatID, reply, nil, 0)
	}()
	return true
}

func aiFallbackReply() string {
	return aiFallbackReplies[rand.Intn(len(aiFallbackReplies))]
}
