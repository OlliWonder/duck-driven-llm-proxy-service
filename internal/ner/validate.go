// Контекстная валидация кандидатов NER.
//
// Slovnet NER возвращает сырые кандидаты PER/LOC/ORG. Сами по себе они не
// являются персональными данными: "поэт Александр Пушкин" — публичное
// упоминание, а "имя держателя карты Пушкина" — ПДН. Решение о том, является
// ли кандидат ПДН и каким типом pii.Type его пометить, принимается на основе
// контекста вокруг span в исходном тексте.
//
// Правила обобщаемые и не содержат списков конкретных людей. Контекст
// определяется относительно КОНКРЕТНОГО кандидата: ищется ближайшая к нему
// подпись/роль в пределах части предложения (часть ограничена разделителями
// ",", ";", ".", "!", "?", переводом строки). Явная близкая подпись
// ("клиент", "имя держателя", "адрес клиента", "место рождения", "кем выдан")
// имеет приоритет над случайным словом дальше в предложении. Учитывается
// отрицание ("не" перед подписью инвертирует её знак).
package ner

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/pii"
)

// signature — подпись/роль, определяющая контекст кандидата.
type signature struct {
	text string
	// typ — тип ПДН, если подпись положительная; пустая строка означает
	// отрицательную подпись (кандидат не является ПДН).
	typ pii.Type
}

// Validator принимает решение о том, является ли кандидат NER персональными
// данными, и присваивает ему тип pii.Type.
type Validator struct{}

// NewValidator создаёт валидатор.
func NewValidator() *Validator { return &Validator{} }

// Validate принимает исходный текст и кандидатов NER, возвращает фрагменты
// ПДН с исходными байтовыми смещениями. Кандидаты, не являющиеся ПДН в данном
// контексте, отбрасываются.
func (v *Validator) Validate(text string, cands []Candidate) []detection.Fragment {
	frags := make([]detection.Fragment, 0, len(cands))
	for _, c := range cands {
		c = trimCandidateRolePrefix(text, c)
		if t, ok := v.decide(text, c); ok {
			frags = append(frags, detection.Fragment{Type: t, Start: c.Start, End: c.End})
		}
	}
	return frags
}

func trimCandidateRolePrefix(text string, candidate Candidate) Candidate {
	if candidate.Label != LabelPER || candidate.Start < 0 || candidate.End > len(text) {
		return candidate
	}
	value := text[candidate.Start:candidate.End]
	lower := strings.ToLower(value)
	for _, role := range []string{
		"клиент", "клиентка", "клиентку",
		"заявитель", "заявительница",
		"заёмщик", "заемщик", "заёмщица", "заемщица",
		"гражданин", "гражданка",
	} {
		if !strings.HasPrefix(lower, role) {
			continue
		}
		end := len(role)
		if end >= len(lower) {
			continue
		}
		next, _ := utf8.DecodeRuneInString(lower[end:])
		if !unicode.IsSpace(next) && next != ':' && next != ',' && next != '—' && next != '-' {
			continue
		}
		for end < len(value) {
			r, size := utf8.DecodeRuneInString(value[end:])
			if !unicode.IsSpace(r) && r != ':' && r != ',' && r != '—' && r != '-' {
				break
			}
			end += size
		}
		if end < len(value) {
			candidate.Start += end
			candidate.Text = text[candidate.Start:candidate.End]
		}
		return candidate
	}
	return candidate
}

// decide возвращает тип ПДН для кандидата или false, если кандидат не является
// ПДН в данном контексте.
func (v *Validator) decide(text string, c Candidate) (pii.Type, bool) {
	if candidateInsideEmail(text, c.Start, c.End) {
		return "", false
	}
	if c.Label == LabelPER && isPublicAuthorJSONValue(text, c.Start) {
		return "", false
	}
	if c.Label == LabelLOC && hasPublicBirthplaceLinkedToClient(text, c.Start) {
		return "", false
	}
	if c.Label == LabelPER && hasPublicAppositiveAfter(text, c.End) && !hasPersonalRoleBefore(text, c.Start) {
		return "", false
	}
	sigs := signaturesFor(c.Label)
	if len(sigs) == 0 {
		return "", false
	}

	// Границы части предложения вокруг кандидата.
	partStart := clauseStart(text, c.Start)
	partEnd := clauseEnd(text, c.End)

	before := strings.ToLower(text[partStart:c.Start])
	after := strings.ToLower(text[c.End:partEnd])
	if c.Label == LabelPER && containsPublicRole(before) {
		return "", false
	}

	// Ближайшая подпись слева и справа от кандидата.
	leftSig, leftDist, leftOK := nearestBefore(before, sigs)
	rightSig, rightDist, rightOK := nearestAfter(after, sigs)
	// A comma may introduce an appositive person name: "Заёмщик, Алексей
	// Морозов". Commas normally isolate unrelated clauses (important for
	// public-person filtering), so bridge only an immediately preceding phrase
	// that ends with a known signature.
	if !leftOK && c.Label == LabelPER {
		if appositive := appositiveSignatureContext(text, c.Start); appositive != "" {
			if appSig, appDist, ok := nearestBefore(appositive, sigs); ok && appDist == 0 {
				before = appositive
				leftSig, leftDist, leftOK = appSig, appDist, true
			}
		}
	}

	// Выбираем ближайшую; при равенстве — слева.
	var sig signature
	switch {
	case leftOK && rightOK:
		if leftDist <= rightDist {
			sig = leftSig
		} else {
			sig = rightSig
		}
	case leftOK:
		sig = leftSig
	case rightOK:
		sig = rightSig
	default:
		return "", false
	}
	if sig.typ == "" && c.Label == LabelPER &&
		(hasPublicAppositiveAfter(text, c.End) || containsPublicRole(strings.ToLower(text[c.Start:c.End]))) {
		return "", false
	}

	selectedLeft := leftOK && sig == leftSig && (!rightOK || leftDist <= rightDist)
	if signatureNegated(before, after, sig, leftDist, rightDist, selectedLeft) {
		// Инвертируем: отрицательная подпись с "не" становится положительной
		// (неоднозначно, поэтому отклоняем), положительная с "не" — отклоняем.
		return "", false
	}

	if sig.typ == "" {
		return "", false
	}
	if c.Label == LabelLOC && (containsPublicRole(strings.ToLower(text[partStart:partEnd])) ||
		(sig.typ == pii.TypeBirthPlace && historicalBirthContext(text, c.Start))) {
		return "", false
	}
	return sig.typ, true
}

// isPublicAuthorJSONValue оставляет явно публичные метаданные JSON открытыми.
// Соседнее поле client_name проверяется отдельно по обычным правилам поиска
// персональных данных.
func isPublicAuthorJSONValue(text string, start int) bool {
	if start < 0 || start > len(text) {
		return false
	}
	prefix := strings.ToLower(detectionWindowBefore(text, start, 128))
	key := strings.LastIndex(prefix, `"public_author"`)
	if key < 0 || strings.ContainsAny(prefix[key+len(`"public_author"`):], "{}") {
		return false
	}
	between := strings.TrimSpace(prefix[key+len(`"public_author"`):])
	return strings.HasPrefix(between, ":")
}

func hasPublicBirthplaceLinkedToClient(text string, start int) bool {
	if start < 0 || start > len(text) {
		return false
	}
	before := strings.ToLower(detectionWindowBefore(text, start, 220))
	birth := strings.LastIndex(before, "родился")
	if birth < 0 {
		birth = strings.LastIndex(before, "родилась")
	}
	if birth < 0 {
		return false
	}
	subject := strings.Fields(before[:birth])
	if len(subject) < 2 {
		return false
	}
	first, surname := subject[len(subject)-2], subject[len(subject)-1]
	for _, suffix := range []string{",", ".", ":", "—", "-", "«", "»", `"`} {
		first = strings.Trim(first, suffix)
		surname = strings.Trim(surname, suffix)
	}
	if len(first) < 3 || len(surname) < 3 {
		return false
	}
	afterEnd := start + 512
	if afterEnd > len(text) {
		afterEnd = len(text)
	}
	after := strings.ToLower(text[start:afterEnd])
	field := strings.Index(after, "фио клиента")
	if field < 0 {
		field = strings.Index(after, "имя клиента")
	}
	if field < 0 {
		return false
	}
	end := field + 180
	if end > len(after) {
		end = len(after)
	}
	client := after[field:end]
	return strings.Contains(client, first) && strings.Contains(client, surname)
}

func detectionWindowBefore(text string, end, maxBytes int) string {
	start := end - maxBytes
	if start < 0 {
		start = 0
	}
	for start < end && text[start]&0xC0 == 0x80 {
		start++
	}
	return text[start:end]
}

func historicalBirthContext(text string, start int) bool {
	from := clauseStart(text, start)
	before := strings.ToLower(text[from:start])
	if strings.Contains(before, "историческ") || strings.Contains(before, "биограф") {
		return true
	}
	for i := from; i+4 <= start; i++ {
		if text[i] < '0' || text[i] > '9' || i > from && text[i-1] >= '0' && text[i-1] <= '9' {
			continue
		}
		if i+4 < len(text) && text[i+4] >= '0' && text[i+4] <= '9' {
			continue
		}
		year, err := strconv.Atoi(text[i : i+4])
		if err == nil && year >= 1000 && year < 1900 {
			return true
		}
	}
	return false
}

func hasPublicAppositiveAfter(text string, end int) bool {
	if end >= len(text) || text[end] != ',' {
		return false
	}
	limit := end + 96
	if limit > len(text) {
		limit = len(text)
	}
	for i := end + 1; i < limit; i++ {
		if text[i] == ';' || text[i] == '.' || text[i] == '!' || text[i] == '?' || text[i] == '\n' {
			limit = i
			break
		}
	}
	clause := strings.ToLower(text[end+1 : limit])
	for _, role := range []string{"автор", "автором", "писатель", "поэт", "режиссёр", "режиссер", "художник", "учёный", "ученый"} {
		if firstSignatureIndex(clause, role) >= 0 {
			return true
		}
	}
	return false
}

func hasPersonalRoleBefore(text string, start int) bool {
	from := clauseStart(text, start)
	if colon := strings.LastIndexAny(text[from:start], ":;.!?\n"); colon >= 0 {
		from += colon + 1
	}
	before := strings.ToLower(text[from:start])
	for _, role := range []string{"клиент", "заявитель", "заёмщик", "заемщик", "пользователь", "сотрудник", "истец", "ответчик"} {
		if strings.Contains(before, role) {
			return true
		}
	}
	return false
}

// Slovnet может принять часть адреса электронной почты (например, «press» в
// press@example.org) за место. Не даём NER разделить адрес: детектор почты сам
// решает, является ли весь адрес персональными данными.
func candidateInsideEmail(text string, start, end int) bool {
	left := start
	for left > 0 && isEmailByte(text[left-1]) {
		left--
	}
	right := end
	for right < len(text) && isEmailByte(text[right]) {
		right++
	}
	if left >= right {
		return false
	}
	value := text[left:right]
	return strings.Contains(value, "@") && strings.Contains(value, ".")
}

func isEmailByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' ||
		strings.ContainsRune("._%+-@", rune(b))
}

func appositiveSignatureContext(text string, candidateStart int) string {
	i := candidateStart
	for i > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:i])
		if !unicode.IsSpace(r) {
			break
		}
		i -= size
	}
	if i == 0 || text[i-1] != ',' {
		return ""
	}
	end := i - 1
	start := end
	for start > 0 {
		r, size := utf8.DecodeLastRuneInString(text[:start])
		if r == ',' || r == ';' || r == '.' || r == '!' || r == '?' || r == '\n' || r == '\r' {
			break
		}
		start -= size
	}
	return strings.ToLower(strings.TrimSpace(text[start:end]))
}

// clauseStart возвращает начало части предложения, содержащей позицию pos.
func clauseStart(text string, pos int) int {
	i := pos
	for i > 0 {
		c := text[i-1]
		if c == ',' || c == ';' || c == '.' || c == '!' || c == '?' || c == '\n' || c == '\r' {
			break
		}
		i--
	}
	return i
}

// clauseEnd возвращает конец части предложения, содержащей позицию pos.
func clauseEnd(text string, pos int) int {
	i := pos
	for i < len(text) {
		c := text[i]
		if c == ',' || c == ';' || c == '.' || c == '!' || c == '?' || c == '\n' || c == '\r' {
			break
		}
		i++
	}
	return i
}

// nearestBefore находит подпись, ближайшую к концу part (к кандидату слева).
// Возвращает подпись, расстояние до кандидата и признак успеха.
func nearestBefore(part string, sigs []signature) (signature, int, bool) {
	best := -1
	var bestSig signature
	for _, s := range sigs {
		idx := lastSignatureIndex(part, s.text)
		if idx < 0 {
			continue
		}
		end := idx + len(s.text)
		dist := len(part) - end
		if best == -1 || dist < best {
			best = dist
			bestSig = s
		}
	}
	return bestSig, best, best != -1
}

// nearestAfter находит подпись, ближайшую к началу part (к кандидату справа).
func nearestAfter(part string, sigs []signature) (signature, int, bool) {
	best := -1
	var bestSig signature
	for _, s := range sigs {
		idx := firstSignatureIndex(part, s.text)
		if idx < 0 {
			continue
		}
		if best == -1 || idx < best {
			best = idx
			bestSig = s
		}
	}
	return bestSig, best, best != -1
}

// hasNegation проверяет, есть ли "не" непосредственно перед подписью.
func signatureNegated(before, after string, sig signature, leftDist, rightDist int, selectedLeft bool) bool {
	if selectedLeft {
		gapStart := len(before) - leftDist
		if gapStart >= 0 && containsNegation(before[gapStart:]) {
			return true
		}
		sigStart := gapStart - len(sig.text)
		from := sigStart - 16
		if from < 0 {
			from = 0
		}
		return sigStart >= 0 && containsNegation(before[from:sigStart])
	}
	if rightDist >= 0 && rightDist <= len(after) {
		return containsNegation(after[:rightDist])
	}
	return false
}

func containsNegation(text string) bool {
	for _, word := range strings.Fields(text) {
		if word == "не" {
			return true
		}
	}
	return false
}

func firstSignatureIndex(text, signature string) int {
	from := 0
	for from <= len(text) {
		idx := strings.Index(text[from:], signature)
		if idx < 0 {
			return -1
		}
		idx += from
		if signatureBoundaries(text, idx, signature) {
			return idx
		}
		from = idx + 1
	}
	return -1
}

func lastSignatureIndex(text, signature string) int {
	last := -1
	from := 0
	for {
		idx := firstSignatureIndex(text[from:], signature)
		if idx < 0 {
			return last
		}
		last = from + idx
		from = last + 1
	}
}

func signatureBoundaries(text string, start int, signature string) bool {
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }
	first, _ := utf8.DecodeRuneInString(signature)
	if start > 0 && isWord(first) {
		previous, _ := utf8.DecodeLastRuneInString(text[:start])
		if isWord(previous) {
			return false
		}
	}
	end := start + len(signature)
	last, _ := utf8.DecodeLastRuneInString(signature)
	if !isWord(last) || end == len(text) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(text[end:])
	return !isWord(next)
}

func containsPublicRole(clause string) bool {
	for _, role := range publicPersonSignatures {
		if firstSignatureIndex(clause, role) >= 0 {
			return true
		}
	}
	return false
}

// signaturesFor возвращает подписи, релевантные для метки кандидата.
func signaturesFor(label Label) []signature {
	switch label {
	case LabelPER:
		return perSignatures
	case LabelLOC:
		return locSignatures
	case LabelORG:
		return orgSignatures
	default:
		return nil
	}
}

// Подписи PER: положительные (личные данные) и отрицательные (публичная фигура).
var perSignatures = []signature{
	{text: "public_author"},
	// Положительные → card_holder.
	{text: "держатель карты", typ: pii.TypeCardHolder},
	{text: "имя держателя", typ: pii.TypeCardHolder},
	// Положительные → full_name.
	{text: "клиента", typ: pii.TypeFullName},
	{text: "клиентки", typ: pii.TypeFullName},
	{text: "клиентку", typ: pii.TypeFullName},
	{text: "клиентка", typ: pii.TypeFullName},
	{text: "клиенту", typ: pii.TypeFullName},
	{text: "клиентом", typ: pii.TypeFullName},
	{text: "клиент", typ: pii.TypeFullName},
	{text: "контактное лицо", typ: pii.TypeFullName},
	{text: "истец", typ: pii.TypeFullName},
	{text: "ответчик", typ: pii.TypeFullName},
	{text: "директор", typ: pii.TypeFullName},
	{text: "client_name", typ: pii.TypeFullName},
	{text: "владелец", typ: pii.TypeFullName},
	{text: "пациент", typ: pii.TypeFullName},
	{text: "сотрудник", typ: pii.TypeFullName},
	{text: "пользователь", typ: pii.TypeFullName},
	{text: "заявитель", typ: pii.TypeFullName},
	{text: "заявительница", typ: pii.TypeFullName},
	{text: "заявителя", typ: pii.TypeFullName},
	{text: "заявительницы", typ: pii.TypeFullName},
	{text: "абонент", typ: pii.TypeFullName},
	{text: "покупатель", typ: pii.TypeFullName},
	{text: "меня зовут", typ: pii.TypeFullName},
	{text: "получатель платежа", typ: pii.TypeFullName},
	{text: "получатель", typ: pii.TypeFullName},
	{text: "имя", typ: pii.TypeFullName},
	{text: "фамилия", typ: pii.TypeFullName},
	{text: "отчество", typ: pii.TypeFullName},
	{text: "физлицо", typ: pii.TypeFullName},
	{text: "гражданин", typ: pii.TypeFullName},
	{text: "гражданка", typ: pii.TypeFullName},
	{text: "гражданина", typ: pii.TypeFullName},
	{text: "заёмщика", typ: pii.TypeFullName},
	{text: "заемщика", typ: pii.TypeFullName},
	{text: "заёмщик", typ: pii.TypeFullName},
	{text: "заемщик", typ: pii.TypeFullName},
	{text: "заёмщица", typ: pii.TypeFullName},
	{text: "заемщица", typ: pii.TypeFullName},
	// Отрицательные → не ПДН.
	{text: "поэт"},
	{text: "писатель"},
	{text: "художник"},
	{text: "композитор"},
	{text: "учёный"},
	{text: "ученый"},
	{text: "президент"},
	{text: "актёр"},
	{text: "актер"},
	{text: "певец"},
	{text: "режиссёр"},
	{text: "режиссер"},
	{text: "историк"},
	{text: "философ"},
	{text: "политик"},
	{text: "министр"},
	{text: "генерал"},
	{text: "спортсмен"},
	{text: "автор"},
	{text: "автором"},
	{text: "классик"},
	{text: "деятель"},
	{text: "премьер"},
	{text: "канцлер"},
	{text: "король"},
	{text: "королева"},
	{text: "царь"},
	{text: "император"},
	{text: "полководец"},
	{text: "космонавт"},
}

var publicPersonSignatures = []string{
	"поэт", "писатель", "художник", "композитор", "учёный", "ученый", "президент",
	"актёр", "актер", "певец", "режиссёр", "режиссер", "историк", "философ",
	"политик", "министр", "генерал", "спортсмен", "автор", "классик", "космонавт",
}

// Подписи LOC: положительные (адрес/место человека) и отрицательные (адрес
// организации).
var locSignatures = []signature{
	// Адрес регистрации организации — общедоступные сведения о компании.
	{text: "компания зарегистрирована"},
	{text: "организация зарегистрирована"},
	{text: "адрес редакции"},
	// Положительные → birth_place.
	{text: "место рождения", typ: pii.TypeBirthPlace},
	{text: "родился", typ: pii.TypeBirthPlace},
	{text: "родилась", typ: pii.TypeBirthPlace},
	// Положительные → address.
	{text: "адрес клиента", typ: pii.TypeAddress},
	{text: "адрес регистрации", typ: pii.TypeAddress},
	{text: "адрес проживания", typ: pii.TypeAddress},
	{text: "проживает", typ: pii.TypeAddress},
	{text: "живёт", typ: pii.TypeAddress},
	{text: "живет", typ: pii.TypeAddress},
	{text: "зарегистрирован", typ: pii.TypeAddress},
	{text: "зарегистрирована", typ: pii.TypeAddress},
	{text: "прописан", typ: pii.TypeAddress},
	{text: "прописана", typ: pii.TypeAddress},
	// Отрицательные → не ПДН.
	{text: "адрес отделения банка"},
	{text: "адрес отделения"},
	{text: "отделение банка"},
	{text: "отделения банка"},
	{text: "адрес офиса"},
	{text: "офис"},
	{text: "адрес компании"},
	{text: "адрес организации"},
	{text: "адрес банка"},
	{text: "филиал"},
	{text: "адрес магазина"},
	{text: "адрес фирмы"},
}

// Подписи ORG: положительные (орган, выдавший документ).
var orgSignatures = []signature{
	{text: "орган выдачи", typ: pii.TypePassportIssuer},
	{text: "кем выдан", typ: pii.TypePassportIssuer},
	{text: "выдавший", typ: pii.TypePassportIssuer},
	{text: "выдал", typ: pii.TypePassportIssuer},
}
