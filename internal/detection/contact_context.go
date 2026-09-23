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

func surroundingContext(text string, start, end, maxBytes int) string {
	left := start - maxBytes
	if left < 0 {
		left = 0
	}
	right := end + maxBytes
	if right > len(text) {
		right = len(text)
	}
	for left < start && text[left]&0xC0 == 0x80 {
		left++
	}
	return strings.ToLower(text[left:right])
}

func isExplicitExampleContext(text string, start, end int) bool {
	context := surroundingContext(text, start, end, 192)
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
	})
}

func isNonPersonalPhoneContext(text string, start int) bool {
	return nonPersonalContactContext(text, start, []string{
		"горячая линия", "колл-центр", "контактный центр", "общий телефон", "общий номер",
		"телефон организации", "телефон компании", "телефон банка", "номер организации", "номер компании",
		"телефон магазина", "телефон ресторана",
	})
}
