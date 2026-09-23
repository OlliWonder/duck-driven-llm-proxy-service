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
		case '.', ';', '!', '?', '\n', '\r':
			start = i + 1
			i = -1
		}
	}
	for start < end && text[start]&0xC0 == 0x80 {
		start++
	}
	return text[start:end]
}

func isNonPersonalEmailContext(text string, start int) bool {
	return nonPersonalContactContext(text, start, []string{
		"общий почтовый ящик", "общая почта", "корпоративная почта", "служебная почта",
		"почта организации", "почта компании", "email организации", "e-mail организации",
		"электронная почта организации", "электронная почта компании",
	})
}

func isNonPersonalPhoneContext(text string, start int) bool {
	return nonPersonalContactContext(text, start, []string{
		"горячая линия", "колл-центр", "контактный центр", "общий телефон", "общий номер",
		"телефон организации", "телефон компании", "телефон банка", "номер организации", "номер компании",
	})
}
