package mask

import "strings"

// Card masks a PAN — shows first 6 and last 4 digits.
// "4111111111111111" → "411111******1111"
func Card(pan string) string {
	pan = strings.ReplaceAll(pan, " ", "")
	if len(pan) < 10 {
		return "**masked**"
	}
	return pan[:6] + strings.Repeat("*", len(pan)-10) + pan[len(pan)-4:]
}

// Email masks the local part of an email address.
// "user@example.com" → "u***@example.com"
func Email(email string) string {
	at := strings.Index(email, "@")
	if at <= 0 {
		return "***@***"
	}
	local := email[:at]
	domain := email[at:]
	if len(local) <= 1 {
		return local + "***" + domain
	}
	return string([]rune(local)[0]) + strings.Repeat("*", len([]rune(local))-1) + domain
}

// Phone masks the middle digits of a phone number.
// "081234567890" → "0812****7890"
func Phone(phone string) string {
	digits := strings.Map(func(r rune) rune {
		if (r >= '0' && r <= '9') || r == '+' {
			return r
		}
		return -1
	}, phone)
	if len(digits) < 8 {
		return "****"
	}
	return digits[:4] + strings.Repeat("*", len(digits)-8) + digits[len(digits)-4:]
}

// Name masks all but the first character of each word.
// "John Doe" → "J*** D**"
func Name(name string) string {
	words := strings.Fields(name)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) <= 1 {
			continue
		}
		words[i] = string(runes[0]) + strings.Repeat("*", len(runes)-1)
	}
	return strings.Join(words, " ")
}

// Secret always returns [REDACTED] — use for API keys, tokens, and secrets.
func Secret(_ string) string {
	return "[REDACTED]"
}
