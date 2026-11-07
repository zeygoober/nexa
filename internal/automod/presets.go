package automod

// Ready-made word lists for the Commonly Flagged Words rule, in English, as
// a server's moderators would otherwise have to write them themselves. Each
// entry uses the same wildcards as a rule's own words: "word*" for words
// starting with it, "*word" for words ending with it. Disguised spellings
// (accents, look-alike letters, numbers for letters, stretched or spaced-out
// letters) are caught by matching, not by listing them. Every server can
// allow words back that a list catches.

// Preset list names.
const (
	PresetProfanity = "profanity"
	PresetSlurs     = "slurs"
	PresetSexual    = "sexual"
)

// Presets are the ready-made lists, by name.
var Presets = map[string][]string{
	// the more extreme swearing, leaving mild words (damn, hell, crap) alone
	PresetProfanity: {
		"fuck*", "*fuck", "*fucker*", "*fucking*", "fuk", "fuking", "fukin", "fuq", "fck", "fcking", "fcker", "fvck*", "phuck*", "fucc*",
		"motherfuck*", "muthafuck*", "mothafuck*", "cunt*", "cocksuck*", "cock sucker", "cumdump*", "shitfuck*", "fuckwit*",
		"twat*", "wank*", "dickhead*", "dick head", "assfuck*", "arsehole*", "shithead*", "shitface*", "dipshit*",
		"piece of shit", "son of a bitch", "sonofabitch*", "fucktard*", "fuckhead*", "fuckface*", "clusterfuck*",
	},
	// personal attacks and terms that may be considered hate speech
	PresetSlurs: {
		"nigger*", "nigga*", "niggah*", "negroid*", "coon", "coons", "jigaboo*", "porch monkey", "darkie*",
		"chink*", "gook*", "jap", "japs", "zipperhead*", "slant eye*", "slanteye*", "spic", "spics", "spick*", "wetback*", "beaner*",
		"kike*", "heeb*", "yid", "yids", "raghead*", "towelhead*", "sand nigger*", "paki", "pakis", "camel jockey*", "gyp", "gypo*",
		"redskin*", "injun*", "squaw*", "abo", "abos",
		"faggot*", "fag", "fags", "faggy", "fagot*", "dyke*", "tranny*", "trannie*", "shemale*", "he-she", "ladyboy*", "sodomite*",
		"retard*", "tard", "tards", "spaz", "spastic*", "mongoloid*",
		"whore*", "slut*", "skank*", "hoe", "hoes", "thot", "thots", "bitch*", "cunt*",
		"kys", "kill yourself", "kill urself", "kill ur self", "go kill yourself", "neck yourself", "hang yourself", "end yourself",
		"drink bleach", "you should die", "unalive yourself", "rope yourself",
		"subhuman*", "untermensch*", "white power", "heil hitler", "sieg heil", "1488", "gas the jews", "white trash",
	},
	// sexually explicit behaviour and activity
	PresetSexual: {
		"porn*", "*porn", "p0rn*", "pr0n", "xxx", "nsfw content", "hentai*", "rule34", "rule 34", "onlyfans", "only fans", "fansly",
		"nude*", "nudes", "send nudes", "nudity", "naked pic*", "dick pic*", "cock pic*", "titty*", "titties", "tits", "boobs", "boobies",
		"pussy*", "cunnilingus", "fellatio", "blowjob*", "blow job*", "handjob*", "hand job*", "rimjob*", "footjob*", "titjob*",
		"cum", "cums", "cumming", "cumshot*", "creampie*", "cum slut*", "jizz*", "semen", "sperm", "orgasm*", "orgy", "orgies",
		"gangbang*", "gang bang*", "threesome*", "anal sex", "anal", "dildo*", "vibrator*", "buttplug*", "butt plug*", "fleshlight*",
		"masturbat*", "jerk off", "jerking off", "jack off", "jacking off", "wank off", "fap", "fapping", "horny", "erection*",
		"boner*", "cock", "cocks", "dick", "dicks", "penis*", "vagina*", "clitoris", "clit", "clits", "bdsm", "bondage", "fetish*",
		"milf*", "dilf*", "camgirl*", "cam girl*", "sexting", "sext", "sexy pic*", "rape", "raped", "raping", "rapist*", "molest*",
		"incest*", "pedo", "pedos", "pedophile*", "paedo*", "loli", "lolicon", "shota*", "bestiality", "zoophil*", "deepthroat*",
		"doggy style", "doggystyle", "69ing", "sex chat", "sex tape*", "camsex", "cybersex", "escort service*", "hooker*", "prostitut*",
	},
}

// PresetNames lists the ready-made lists in the order they are offered.
var PresetNames = []string{PresetProfanity, PresetSlurs, PresetSexual}

// SpamPhrases are what scams and unwanted advertising say, for the Suspected
// Spam rule.
var SpamPhrases = []string{
	"free nitro", "nitro for free", "free discord nitro", "nitro giveaway", "discord nitro free", "steam gift card", "free steam gift",
	"free robux", "robux generator", "free vbucks", "free v-bucks", "vbucks generator", "claim your reward", "claim your prize",
	"claim your gift", "you have won", "you've won", "you won a", "congratulations you won", "airdrop is live", "free airdrop",
	"crypto giveaway", "bitcoin giveaway", "eth giveaway", "double your bitcoin", "double your crypto", "send me bitcoin",
	"investment opportunity", "guaranteed profit", "passive income", "make money fast", "earn $", "earn money from home",
	"work from home and earn", "dm me to earn", "dm me for profit", "100% legit", "limited time offer", "click this link",
	"click the link below", "first 100 people", "first 50 people", "scan this qr", "verify your account", "your account will be banned",
	"account will be suspended", "login to claim", "connect your wallet", "free giftcard", "free gift card", "cheap followers",
	"buy followers", "get followers", "onlyfans leaks", "leaked nudes", "18+ server", "join my server", "check my bio",
	"check out my profile", "i'm selling", "selling accounts", "boosting service", "cashapp me", "cash app me",
}

// Shorteners are link shorteners, which hide where a link goes.
var Shorteners = []string{
	"bit.ly", "tinyurl.com", "t.co", "goo.gl", "ow.ly", "is.gd", "buff.ly", "cutt.ly", "rebrand.ly", "shorturl.at", "rb.gy",
	"tiny.cc", "t.ly", "bl.ink", "short.io", "s.id", "v.gd", "lnkd.in", "clck.ru", "qr.ae", "adf.ly", "shorte.st", "linktr.ee",
	"grabify.link", "iplogger.org", "iplogger.com", "2no.co", "yip.su", "blasze.com", "ps3cfw.com",
}

// Impersonated are the names scam sites put in their own addresses to pass
// for the real ones, with the addresses that really are them.
var Impersonated = map[string][]string{
	"discord":   {"discord.com", "discord.gg", "discordapp.com", "discord.media", "discordapp.net", "discord.new", "discord.gift", "discordstatus.com", "dis.gd"},
	"nitro":     {"discord.com"},
	"steam":     {"steampowered.com", "steamcommunity.com", "steamstatic.com", "steamgames.com", "steamchina.com"},
	"nexa":      {"nexa.pm"},
	"paypal":    {"paypal.com", "paypal.me"},
	"roblox":    {"roblox.com", "rbxcdn.com"},
	"epicgames": {"epicgames.com"},
	"twitch":    {"twitch.tv"},
	"metamask":  {"metamask.io"},
	"opensea":   {"opensea.io"},
}

// RiskyExtensions are file endings that run as programs when opened, for the
// file rule to start from.
var RiskyExtensions = []string{"exe", "scr", "bat", "cmd", "com", "pif", "msi", "msix", "vbs", "vbe", "js", "jse", "wsf", "wsh", "ps1", "psm1", "hta", "lnk", "jar", "apk", "dll", "reg", "cpl", "iso", "img", "dmg", "app", "sh", "run", "appimage", "deb", "rpm"}
