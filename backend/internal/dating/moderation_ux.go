package dating

import (
	"fmt"
	"log"
	"strings"

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

func startSupport(bc *engine.BotContext) bool {
	bc.State["scene"] = "support"
	bc.State["step"] = "support_msg"
	bc.SaveState()
	bc.ReplyKBD(
		"📩 پشتیبانی\n\nپیامت رو بنویس و بفرست؛ ادمین‌ها می‌بینن و جواب میدن.\n\nبرای خروج: 🏠 منوی اصلی",
		supportRows)
	return true
}

func handleSupportText(bc *engine.BotContext, st *Store, text string) bool {
	// home-menu labels are routed by the central guard in HandleMessage
	repo := moderationRepoOf(bc)
	if repo == nil {
		goHome(bc, "پشتیبانی فعلاً در دسترس نیست.")
		return true
	}
	body := strings.TrimSpace(text)
	if len([]rune(body)) < 3 || strings.HasPrefix(body, "/") {
		bc.ReplyKBD("متن پیام باید حداقل ۳ حرف باشه. دوباره بنویس.", supportRows)
		return true
	}
	if _, err := repo.InsertTicket(engineBG(), botUUID(bc), bc.UserID, body); err != nil {
		log.Printf("[dating] ticket insert: %v", err)
		bc.ReplyKBD("خطا در ثبت پیام. دوباره تلاش کن.", supportRows)
		return true
	}
	goHome(bc, "✅ پیامت ثبت شد؛ ادمین‌ها به‌زودی جواب میدن.")
	return true
}
