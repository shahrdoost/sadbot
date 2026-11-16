package dating

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"sadbot/internal/domain"
	"sadbot/internal/engine"
	"sadbot/internal/repository/postgres"
)

// Anonymous confessions (admin-approved before channel publish) and the
// support ticket inbox — bot-side scenes for phase 3 moderation.

var confessConfirmRows = [][]string{
	{"✅ ثبت اعتراف", "❌ انصراف"},
}

var supportRows = [][]string{
	{"🏠 منوی اصلی"},
}

func moderationRepoOf(bc *engine.BotContext) *postgres.ModerationRepo {
	r, _ := bc.Data["moderationRepo"].(*postgres.ModerationRepo)
	return r
}

// --- confessions ---

func startConfess(bc *engine.BotContext) bool {
	bc.State["scene"] = "confess"
	bc.State["step"] = "confess_text"
	bc.SaveState()
	cost := bc.PriceInt("confess_cost", 2)
	bc.ReplyKBD(fmt.Sprintf(
		"💌 اعتراف ناشناس\n\nمتن اعتراف رو بنویس و بفرست.\n(هزینه: %d سکه — بعد از تایید ادمین در کانال منتشر می‌شود)\n\nبرای خروج: 🏠 منوی اصلی",
		cost), [][]string{{"🏠 منوی اصلی"}})
	return true
}

func handleConfessText(bc *engine.BotContext, st *Store, text string) bool {
	// home-menu labels are routed by the central guard in HandleMessage
	repo := moderationRepoOf(bc)
	if repo == nil {
		goHome(bc, "ثبت اعتراف فعلاً در دسترس نیست.")
		return true
	}
	ctx := engineBG()
	botID := botUUID(bc)

	if step(bc) == "confess_confirm" {
		switch text {
		case "✅ ثبت اعتراف":
			cost := bc.PriceInt("confess_cost", 2)
			if cost > 0 {
				ok, err := st.Repo().SpendCoins(ctx, botID, bc.UserID, cost)
				if err != nil || !ok {
					clearConfessState(bc)
					goHome(bc, fmt.Sprintf("💰 سکه کافی نیست! (هزینهٔ اعتراف: %d سکه)", cost))
					return true
				}
			}
			data := dataMap(bc)
			confessText, _ := data["confess"].(string)
			if _, err := repo.InsertConfession(ctx, botID, bc.UserID, confessText); err != nil {
				log.Printf("[dating] confession insert: %v", err)
				if cost > 0 {
					_, _ = st.Repo().AddCoins(ctx, botID, bc.UserID, cost) // rollback
				}
				clearConfessState(bc)
				goHome(bc, "خطا در ثبت. دوباره تلاش کن.")
				return true
			}
			clearConfessState(bc)
			goHome(bc, "✅ اعتراف ثبت شد!\nپس از تایید ادمین، در کانال منتشر می‌شود.")
			return true
		case "❌ انصراف":
			clearConfessState(bc)
			goHome(bc, "انصراف دادی. از اول شروع کن اگر خواستی.")
			return true
		default:
			bc.ReplyKBD("یکی از دکمه‌ها رو بزن.", confessConfirmRows)
			return true
		}
	}

	// step confess_text → preview + confirm
	body := strings.TrimSpace(text)
	if len([]rune(body)) < 5 || strings.HasPrefix(body, "/") {
		bc.ReplyKBD("متن اعتراف باید حداقل ۵ حرف باشه. دوباره بنویس.", [][]string{{"🏠 منوی اصلی"}})
		return true
	}
	data := dataMap(bc)
	data["confess"] = body
	bc.State["step"] = "confess_confirm"
	bc.SaveState()
	cost := bc.PriceInt("confess_cost", 2)
	bc.ReplyKBD(
		"پیش‌نمایش اعتراف:\n\n«"+body+"»\n\nثبت شود؟ (هزینه: "+fmt.Sprint(cost)+" سکه)",
		confessConfirmRows)
	return true
}

func clearConfessState(bc *engine.BotContext) {
	data := dataMap(bc)
	delete(data, "confess")
	bc.State["step"] = ""
	bc.SaveState()
}

// --- support tickets ---

const (
	supportActionContinue = "act:support:continue:"
	supportActionClose    = "act:support:close:"
	supportActionNew      = "act:support:new"
)

// startSupport opens the support scene on the user's open ticket when there is
// one, so a follow-up lands in the same conversation instead of a new ticket.
func startSupport(bc *engine.BotContext) bool {
	bc.State["scene"] = "support"
	bc.State["step"] = "support_msg"
	bc.SaveState()
	open := openTicket(bc)
	if open == nil {
		bc.ReplyKBD(
			"📩 پشتیبانی\n\nپیامت رو بنویس و بفرست؛ ادمین‌ها می‌بینن و جواب میدن.\n\nبرای خروج: 🏠 منوی اصلی",
			supportRows)
		return true
	}
	// one open ticket at a time: default to continuing it, with a way out to a
	// fresh ticket for a different topic
	bc.ReplyInline(
		fmt.Sprintf("📩 پشتیبانی\n\nیک تیکت باز داری؛ هرچی بنویسی به همان اضافه می‌شود.\nبرای موضوع جدید، «تیکت جدید» را بزن."),
		[][]engine.InlineButton{{
			{Label: "📝 تیکت جدید", Callback: supportActionNew},
			{Label: "🏠 منوی اصلی", Callback: "act:support:home"},
		}})
	return true
}

// openTicket returns the user's non-closed ticket, or nil.
func openTicket(bc *engine.BotContext) *domain.SupportTicket {
	repo := moderationRepoOf(bc)
	if repo == nil {
		return nil
	}
	t, err := repo.OpenTicketForUser(engineBG(), botUUID(bc), bc.UserID)
	if err != nil {
		log.Printf("[dating] open ticket lookup: %v", err)
		return nil
	}
	return t
}

func handleSupportText(bc *engine.BotContext, st *Store, text string) bool {
	// home-menu labels are routed by the central guard in HandleMessage
	repo := moderationRepoOf(bc)
	if repo == nil {
		goHome(bc, "پشتیبانی فعلاً در دسترس نیست.")
		return true
	}
	// A button press is never a ticket body. On Kat every reply-keyboard press
	// arrives as a callback whose data IS the label; an old screen's button
	// (or a leftover chat button) pressed here used to be stored verbatim as a
	// support ticket, so the admin's inbox filled with «📞 پشتیبانی»-style
	// entries instead of what the user actually wrote.
	if bc.IsCallback {
		bc.ReplyKBD("لطفاً متن پیامت رو تایپ کن و بفرست (دکمه‌ها ثبت نمی‌شن).", supportRows)
		return true
	}
	body := strings.TrimSpace(text)
	if len([]rune(body)) < 3 || strings.HasPrefix(body, "/") {
		bc.ReplyKBD("متن پیام باید حداقل ۳ حرف باشه. دوباره بنویس.", supportRows)
		return true
	}
	// an open ticket absorbs the message; a fresh one only starts when the
	// user asked for it or has nothing open
	if open := openTicket(bc); open != nil && dataMap(bc)["ticket_fresh"] != true {
		if _, err := repo.AddUserMessage(engineBG(), botUUID(bc), open.ID, body); err != nil {
			log.Printf("[dating] ticket message: %v", err)
			bc.ReplyKBD("خطا در ثبت پیام. دوباره تلاش کن.", supportRows)
			return true
		}
		delete(dataMap(bc), "ticket_fresh")
		bc.SaveState()
		goHome(bc, "✅ پیامت به تیکت اضافه شد؛ ادمین‌ها جواب میدن.")
		return true
	}
	if _, err := repo.InsertTicket(engineBG(), botUUID(bc), bc.UserID, body); err != nil {
		log.Printf("[dating] ticket insert: %v", err)
		bc.ReplyKBD("خطا در ثبت پیام. دوباره تلاش کن.", supportRows)
		return true
	}
	delete(dataMap(bc), "ticket_fresh")
	bc.SaveState()
	goHome(bc, "✅ پیامت ثبت شد؛ ادمین‌ها به‌زودی جواب میدن.")
	return true
}

// handleSupportAction routes the buttons attached to a support reply:
// continue the same ticket, close it, or start a fresh one.
func handleSupportAction(bc *engine.BotContext, action string) bool {
	repo := moderationRepoOf(bc)
	if repo == nil {
		goHome(bc, "پشتیبانی فعلاً در دسترس نیست.")
		return true
	}
	switch {
	case action == "support:home":
		goHome(bc, "🏠 منوی اصلی")
		return true

	case action == "support:new":
		bc.State["scene"] = "support"
		bc.State["step"] = "support_msg"
		dataMap(bc)["ticket_fresh"] = true
		bc.SaveState()
		bc.ReplyKBD(
			"📝 تیکت جدید\n\nپیامت رو بنویس و بفرست.\n\nبرای خروج: 🏠 منوی اصلی",
			supportRows)
		return true

	case strings.HasPrefix(action, "support:continue:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(action, "support:continue:"), 10, 64)
		if err != nil {
			return fallbackAction(bc)
		}
		t, err := repo.TicketByID(engineBG(), botUUID(bc), id)
		if err != nil || t == nil {
			goHome(bc, "این تیکت پیدا نشد.")
			return true
		}
		if t.Status == domain.TicketClosed {
			bc.ReplyInline("🔒 این تیکت بسته شده.\nبرای پیام جدید، تیکت تازه باز کن.",
				[][]engine.InlineButton{{
					{Label: "📝 تیکت جدید", Callback: supportActionNew},
					{Label: "🏠 منوی اصلی", Callback: "act:support:home"},
				}})
			return true
		}
		bc.State["scene"] = "support"
		bc.State["step"] = "support_msg"
		delete(dataMap(bc), "ticket_fresh")
		bc.SaveState()
		bc.ReplyKBD(
			"✍️ ادامهٔ گفتگو\n\nپاسخ خودت رو بنویس و بفرست؛ به همین تیکت اضافه می‌شود.\n\nبرای خروج: 🏠 منوی اصلی",
			supportRows)
		return true

	case strings.HasPrefix(action, "support:close:"):
		id, err := strconv.ParseInt(strings.TrimPrefix(action, "support:close:"), 10, 64)
		if err != nil {
			return fallbackAction(bc)
		}
		ok, err := repo.CloseTicket(engineBG(), botUUID(bc), id)
		if err != nil || !ok {
			bc.ReplyInline("این تیکت دیگر باز نیست.",
				[][]engine.InlineButton{{
					{Label: "📝 تیکت جدید", Callback: supportActionNew},
					{Label: "🏠 منوی اصلی", Callback: "act:support:home"},
				}})
			return true
		}
		bc.State["scene"] = "home"
		bc.State["step"] = ""
		bc.SaveState()
		bc.ReplyInline("🔒 تیکت بسته شد. اگر موضوع جدیدی داری، تیکت تازه باز کن.",
			[][]engine.InlineButton{{
				{Label: "📝 تیکت جدید", Callback: supportActionNew},
				{Label: "🏠 منوی اصلی", Callback: "act:support:home"},
			}})
		return true
	}
	return fallbackAction(bc)
}
