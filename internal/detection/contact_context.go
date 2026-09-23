package detection

import "strings"

func nonPersonalContactContext(text string, start int, markers []string) bool {
	clause := strings.ToLower(contactClauseBefore(text, start, 192))
	for _, marker := range markers {
		if strings.Contains(clause, marker) {
			return true
		}
	}
	return false
}

func contactClauseBefore(text string, end, maxBytes int) string {
	start := end - maxBytes
	if start < 0 {
		start = 0
	}
	for i := end - 1; i >= start; i-- {
		switch text[i] {
		case '.':
			if i > 0 && i+1 < end && isASCIILetter(text[i-1]) && isASCIILetter(text[i+1]) {
				continue // dot inside an email/host name, not a sentence boundary
			}
			if (i > 0 && i+1 < end && isDigit(text[i-1]) && isDigit(text[i+1])) || isAddressAbbreviationDot(text, i) {
				continue
			}
			start = i + 1
			i = -1
		case ';', '!', '?', '\n', '\r':
			start = i + 1
			i = -1
		}
	}
	for start < end && text[start]&0xC0 == 0x80 {
		start++
	}
	return text[start:end]
}

func isASCIILetter(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func isExplicitExampleContext(text string, start, end int) bool {
	context := contactClauseBefore(text, start, 192)
	if end >= 0 && end < len(text) {
		right := end + 96
		if right > len(text) {
			right = len(text)
		}
		after := text[end:right]
		if cut := strings.IndexAny(after, ".,;!?\n\r"); cut >= 0 {
			after = after[:cut]
		}
		context += " " + strings.ToLower(after)
	}
	return strings.Contains(context, "исключительно как пример") ||
		(strings.Contains(context, "привед") && strings.Contains(context, "тестов")) ||
		(strings.Contains(context, "пример") &&
			(strings.Contains(context, "инструкц") || strings.Contains(context, "документац")))
}

func isNonPersonalEmailContext(text string, start int) bool {
	return nonPersonalContactContext(text, start, []string{
		"общий почтовый ящик", "общая почта", "корпоративная почта", "служебная почта",
		"почта организации", "почта компании", "email организации", "e-mail организации",
		"электронная почта организации", "электронная почта компании",
		"почта отдела", "почта пресс-службы", "email пресс-службы", "e-mail пресс-службы",
		"общий адрес", "адрес поддержки", "опубликованы", "публичный рабочий адрес",
	})
}

func isNonPersonalPhoneContext(text string, start int) bool {
	return isExplicitExampleContext(text, start, start+12) || nonPersonalContactContext(text, start, []string{
		"горячая линия", "колл-центр", "контактный центр", "общий телефон", "общий номер",
		"телефон организации", "телефон компании", "телефон банка", "номер организации", "номер компании",
		"телефон магазина", "телефон ресторана", "телефон приёмной", "телефон приемной",
		"приёмная", "приемная", "телефон справочной", "справочная", "опубликованы", "общий телефон",
	})
}
