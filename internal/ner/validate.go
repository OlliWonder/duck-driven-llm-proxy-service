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
	"strings"

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
		if t, ok := v.decide(text, c); ok {
			frags = append(frags, detection.Fragment{Type: t, Start: c.Start, End: c.End})
		}
	}
	return frags
}

// decide возвращает тип ПДН для кандидата или false, если кандидат не является
// ПДН в данном контексте.
func (v *Validator) decide(text string, c Candidate) (pii.Type, bool) {
	sigs := signaturesFor(c.Label)
	if len(sigs) == 0 {
		return "", false
	}

	// Границы части предложения вокруг кандидата.
	partStart := clauseStart(text, c.Start)
	partEnd := clauseEnd(text, c.End)

	before := strings.ToLower(text[partStart:c.Start])
	after := strings.ToLower(text[c.End:partEnd])

	// Ближайшая подпись слева и справа от кандидата.
	leftSig, leftDist, leftOK := nearestBefore(before, sigs)
	rightSig, rightDist, rightOK := nearestAfter(after, sigs)

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

	selectedLeft := leftOK && sig == leftSig && (!rightOK || leftDist <= rightDist)
	if signatureNegated(before, after, sig, leftDist, rightDist, selectedLeft) {
		// Инвертируем: отрицательная подпись с "не" становится положительной
		// (неоднозначно, поэтому отклоняем), положительная с "не" — отклоняем.
		return "", false
	}

	if sig.typ == "" {
		return "", false
	}
	if c.Label == LabelLOC && containsPublicRole(strings.ToLower(text[partStart:partEnd])) {
		return "", false
	}
	return sig.typ, true
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
	isWord := func(b byte) bool {
		return b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 0x80
	}
	if start > 0 && isWord(signature[0]) && isWord(text[start-1]) {
		return false
	}
	end := start + len(signature)
	return !isWord(signature[len(signature)-1]) || end == len(text) || !isWord(text[end])
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
	// Положительные → card_holder.
	{text: "держатель карты", typ: pii.TypeCardHolder},
	{text: "имя держателя", typ: pii.TypeCardHolder},
	// Положительные → full_name.
	{text: "клиента", typ: pii.TypeFullName},
	{text: "клиенту", typ: pii.TypeFullName},
	{text: "клиентом", typ: pii.TypeFullName},
	{text: "клиент", typ: pii.TypeFullName},
	{text: "владелец", typ: pii.TypeFullName},
	{text: "пациент", typ: pii.TypeFullName},
	{text: "сотрудник", typ: pii.TypeFullName},
	{text: "пользователь", typ: pii.TypeFullName},
	{text: "заявитель", typ: pii.TypeFullName},
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
	// Положительные → birth_place.
	{text: "место рождения", typ: pii.TypeBirthPlace},
	{text: "родился", typ: pii.TypeBirthPlace},
	{text: "родилась", typ: pii.TypeBirthPlace},
	// Положительные → address.
	{text: "адрес клиента", typ: pii.TypeAddress},
	{text: "адрес регистрации", typ: pii.TypeAddress},
	{text: "адрес проживания", typ: pii.TypeAddress},
	{text: "проживает", typ: pii.TypeAddress},
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
