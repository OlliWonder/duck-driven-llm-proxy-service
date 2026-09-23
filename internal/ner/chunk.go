// Разбиение длинного текста на части (chunks) с overlap для NER.
//
// Slovnet NER обрабатывает текст ограниченной длины. Длинные тексты (до
// 100 000 токенов) разбиваются на части с небольшим overlap, чтобы сущности
// на границах не терялись. Границы выбираются безопасно для UTF-8 и по
// возможности по пробелам/пунктуации, чтобы не резать сущность посередине.
package ner

// chunkOptions — параметры разбиения длинного текста.
type chunkOptions struct {
	// chunkSize — целевой размер части в байтах.
	chunkSize int
	// overlap — размер перекрытия между соседними частями в байтах.
	overlap int
}

// defaultChunkOptions — значения по умолчанию.
//
// chunkSize=8192 байт (8 KB) — тексты больше 8 KB разбиваются на части.
// overlap=512 байт — достаточно, чтобы сущность, пересекающая границу, была
// полностью захвачена хотя бы в одной части.
var defaultChunkOptions = chunkOptions{
	chunkSize: 8192,
	overlap:   512,
}

// chunk — одна часть текста.
type chunk struct {
	// start — байтовое смещение начала части в исходном тексте.
	start int
	// end — байтовое смещение конца части в исходном тексте.
	end int
	// text — подстрока исходного текста.
	text string
}

// splitChunks разбивает текст на части с overlap. Короткие тексты (не больше
// chunkSize) возвращаются одной частью без разбиения.
func splitChunks(text string, opts chunkOptions) []chunk {
	if len(text) <= opts.chunkSize {
		return []chunk{{start: 0, end: len(text), text: text}}
	}
	var chunks []chunk
	start := 0
	for start < len(text) {
		end := start + opts.chunkSize
		if end >= len(text) {
			end = len(text)
		} else {
			end = adjustBoundary(text, end)
		}
		chunks = append(chunks, chunk{start: start, end: end, text: text[start:end]})
		if end >= len(text) {
			break
		}
		// Следующая часть начинается внутри overlap текущей.
		next := end - opts.overlap
		if next < 0 {
			next = 0
		}
		next = adjustForward(text, next)
		if next <= start || next >= end {
			next = end
		}
		start = next
	}
	return chunks
}

// adjustBoundary сдвигает позицию end назад до безопасной границы (пробел,
// пунктуация, перевод строки) и валидной UTF-8 границы. Ищет в окне до 100
// байт назад; если не находит — сдвигает до начала UTF-8 символа.
func adjustBoundary(text string, end int) int {
	if end > len(text) {
		end = len(text)
	}
	limit := end - 100
	if limit < 0 {
		limit = 0
	}
	for i := end - 1; i >= limit; i-- {
		c := text[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' ||
			c == ',' || c == ';' || c == '.' || c == '!' || c == '?' {
			return i + 1
		}
	}
	return toUTF8Boundary(text, end)
}

// adjustForward сдвигает позицию start вперёд до безопасной границы (пробел,
// пунктуация) и валидной UTF-8 границы. Ищет в окне до 100 байт вперёд.
func adjustForward(text string, start int) int {
	if start >= len(text) {
		return len(text)
	}
	start = toUTF8BoundaryForward(text, start)
	limit := start + 100
	if limit > len(text) {
		limit = len(text)
	}
	for i := start; i < limit; i++ {
		c := text[i]
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' ||
			c == ',' || c == ';' || c == '.' || c == '!' || c == '?' {
			return i + 1
		}
	}
	return start
}

// toUTF8Boundary сдвигает позицию назад до начала UTF-8 символа.
func toUTF8Boundary(text string, pos int) int {
	for pos > 0 && pos < len(text) {
		b := text[pos]
		if b >= 0x80 && b <= 0xBF {
			pos--
			continue
		}
		break
	}
	return pos
}

// toUTF8BoundaryForward сдвигает позицию вперёд до начала следующего UTF-8
// символа, если текущая позиция указывает на continuation byte.
func toUTF8BoundaryForward(text string, pos int) int {
	for pos < len(text) {
		b := text[pos]
		if b >= 0x80 && b <= 0xBF {
			pos++
			continue
		}
		break
	}
	return pos
}
