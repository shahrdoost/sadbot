package content

import (
	"regexp"
	"strconv"
	"strings"
)

// LLM prompts — verbatim from sadcontent/main.py (the behavioral reference).
// Overridable per-bot via bot_configs.texts (clean_prompt / card_prompt).

const (
	// CleanSystem strips links/branding/ads and outputs clean Persian, or
	// exactly NO_CONTENT when the post is an advertisement.
	CleanSystem = `You are a meticulous news editor for a crypto/finance Telegram channel.
Clean the following post:
- Remove all URLs, links, t.me references, @usernames, phone numbers, invites
- Remove promotional phrases (join us, subscribe, follow, download app, contact admin, etc.)
- Remove channel branding, sign-offs, and ad hashtags
- Remove forwarded-from artifacts and quotes of other channels
- IMPORTANT: reject ENTIRELY (output NO_CONTENT) if the post is an advertisement —
  examples: trading-broker bonuses (بونوس، پاداش، کش‌بک), deposit offers,
  "تا سقف X دلار", promotional benefit lists (مزایای...), referral/invite links,
  platform promos, limited-time offers, customer-service boilerplate
  (لطفاً صبور باشید، پشتیبانی، پاسخ تیم ما), "بعد از ارسال" notices,
  private-channel memberships (عضو کانال خصوصی، تمایل دارند عضو شوند,
  کی بخریم/کی بفروشیم signal groups), "به آیدی زیر مراجعه کنید" invites
- Keep the actual news, analysis, numbers and facts — preserve meaning
- Write natural, tidy Persian
- Output ONLY the cleaned text. If nothing meaningful remains, output exactly: NO_CONTENT`

	// CardSystem summarizes text for the rendered card (1-2 complete sentences).
	CardSystem = `یک ویرایشگر شبکه‌های اجتماعی حرفه‌ای هستی. متن زیر را برای قرارگیری داخل کارت اینستاگرام خلاصه کن:
- دقیقاً ۱ یا ۲ جمله‌ی کامل و تمام‌شده — هرگز وسط جمله تمام نشود
- مهم‌ترین حقایق و اعداد را نگه دار
- کوتاه و گویا باش، حدود ۱۵۰ کاراکتر
- فقط خلاصه را خروجی بده، هیچ توضیح یا علامت اضافه نگذار`
)

// conversationalMarkers — outputs containing these are the model answering
// conversationally instead of cleaning (ported guard from sadcontent).
var conversationalMarkers = []string{"لطفاً", "لطفا", "ارسال کنید", "متنی که", "بفرمایید"}

// cardConversationalMarkers — the card-summary guard set (same minus بفرمایید).
var cardConversationalMarkers = []string{"لطفاً", "لطفا", "ارسال کنید", "متنی که"}

// sourceCreditMarkers — attribution lines the model leaves behind that should
// never ship ("مرجع این خبر: @x", "منبع: ...").
var sourceCreditMarkers = []string{"مرجع این خبر", "منبع این خبر", "منبع:", "منبع :"}

// promoMarkers — engagement-bait / paid-service sales copy. A post carrying any
// of these is not free value (giveaway contests, VIP updates, price lists),
// even when it name-drops "کانفیگ".
var promoMarkers = []string{
	"VIP", "وی‌آی‌پی", "وی آی پی",
	"اشتراک", "تمدید اشتراک",
	"برای خرید", "جهت خرید", "پیام بدید", "در ارتباط باشید", "تماس با فروش",
	"فروشنده", "قیمت:", "پلن یک‌ماهه", "پلن ۱ ماهه", "پلن یک ماهه",
	"پیش‌بینی", "هدیه بگیر", "قرعه‌کشی", "نحوه شرکت", "یک شانس داری",
	"تکمیل ظرفیت", "ظرفیت توزیع", "فقط تا ساعت",
	"برای کاربران VIP", "کاربران VIP", "بروزرسانی مهم برای",
	"بری بت", "بونوس", "تبلیغات پذیرفته", "فروش فیلترشکن",
}

func containsAny(s string, subs []string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// foldZWNJ removes zero-width non-joiners so "پیش‌بینی" and "پیش‌بینی" match
// the same marker (the LLM and source channels mix both spellings).
func foldZWNJ(s string) string {
	return strings.ReplaceAll(s, "‌", "")
}

// stripSourceCredits drops attribution lines the model sometimes keeps.
func stripSourceCredits(s string) string {
	out := make([]string, 0, strings.Count(s, "\n")+1)
	for _, ln := range strings.Split(s, "\n") {
		if containsAny(foldZWNJ(ln), sourceCreditMarkers) {
			continue
		}
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// looksLikePromo is the deterministic guard behind the LLM: contests, VIP
// notices and price lists must not reach the channel.
func looksLikePromo(s string) bool {
	folded := foldZWNJ(s)
	for _, m := range promoMarkers {
		if strings.Contains(folded, foldZWNJ(m)) {
			return true
		}
	}
	return false
}

// hasConfigURI reports whether the text carries a real subscription link.
func hasConfigURI(s string) bool {
	for _, scheme := range []string{"vless://", "vmess://", "ss://", "ssr://", "trojan://", "tuic://", "hysteria://", "hysteria2://"} {
		if strings.Contains(strings.ToLower(s), scheme) {
			return true
		}
	}
	return false
}

// URI / mention shapes for stripForeignMentions. Config URIs must survive
// intact (they contain @host) while every other @handle goes away.
var (
	configURIRe = regexp.MustCompile(`(?i)(?:vless|vmess|ss|ssr|trojan|tuic|hysteria2?|wireguard|wg|socks)://\S+`)
	mentionRe   = regexp.MustCompile(`@[A-Za-z][A-Za-z0-9_]{3,}`)
	tmeLinkRe   = regexp.MustCompile(`(?:https?://)?t\.me/\S+`)
	blankRunRe  = regexp.MustCompile(`\n{3,}`)
)

// stripForeignMentions removes source-channel @handles and t.me links, keeping
// only the brand handle. Config URIs are placeholdered so `vless://id@host`
// is not damaged.
func stripForeignMentions(s, brandHandle string) string {
	if s == "" {
		return s
	}
	var kept []string
	protected := configURIRe.ReplaceAllStringFunc(s, func(m string) string {
		kept = append(kept, m)
		return "\x00u" + strconv.Itoa(len(kept)-1) + "\x00"
	})
	protected = tmeLinkRe.ReplaceAllString(protected, "")
	protected = mentionRe.ReplaceAllStringFunc(protected, func(m string) string {
		if brandHandle != "" && strings.EqualFold(m, brandHandle) {
			return m
		}
		return ""
	})
	for i, u := range kept {
		protected = strings.Replace(protected, "\x00u"+strconv.Itoa(i)+"\x00", u, 1)
	}
	// drop lines left empty by the strips (and squeeze leftover double blanks)
	lines := strings.Split(protected, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			if len(out) > 0 && out[len(out)-1] == "" {
				continue
			}
			out = append(out, "")
			continue
		}
		out = append(out, strings.TrimRight(ln, " \t"))
	}
	return strings.TrimSpace(blankRunRe.ReplaceAllString(strings.Join(out, "\n"), "\n\n"))
}
