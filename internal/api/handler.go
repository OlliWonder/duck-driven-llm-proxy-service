package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/detection"
	"github.com/duck-driven-llm-proxy-service/internal/metrics"
)

// ConsumerHeader — HTTP-заголовок, используемый для идентификации вызывающей системы.
const ConsumerHeader = "X-Consumer"

// DefaultConsumer используется, когда заголовок потребителя отсутствует.
const DefaultConsumer = "default"

// maxProcessBodyBytes ограничивает память на один запрос, сохраняя большой
// запас для long-text detection (включая проверенные тексты на 100 000 токенов).
const maxProcessBodyBytes int64 = 8 << 20

// ProcessRequest — тело запроса по контракту.
type ProcessRequest struct {
	Payload   string `json:"payload"`
	PayloadID string `json:"payload_id"`
}

// ProcessResponse — тело ответа по контракту.
type ProcessResponse struct {
	Result string `json:"result"`
}

// Handler обслуживает POST /process.
type Handler struct {
	svc     *Service
	metrics *metrics.Metrics
	log     *slog.Logger
	// rateLimiter возвращает false, когда запрос следует отклонить с кодом 429.
	rateLimiter func() bool
}

// NewHandler создаёт HTTP-обработчик для /process.
func NewHandler(svc *Service, m *metrics.Metrics, log *slog.Logger, rateLimiter func() bool) *Handler {
	return &Handler{svc: svc, metrics: m, log: log, rateLimiter: rateLimiter}
}

// ServeHTTP реализует http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	status := http.StatusOK
	h.metrics.RequestStarted()
	defer func() { h.metrics.RequestFinished(status, time.Since(start)) }()
	if r.Method != http.MethodPost {
		status = http.StatusMethodNotAllowed
		writeError(w, http.StatusMethodNotAllowed, "метод не поддерживается")
		return
	}

	if h.rateLimiter != nil && !h.rateLimiter() {
		status = http.StatusTooManyRequests
		h.metrics.IncRateLimited()
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "слишком много запросов")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxProcessBodyBytes)
	decoder := json.NewDecoder(r.Body)
	var req ProcessRequest
	if err := decoder.Decode(&req); err != nil {
		h.metrics.IncErrors()
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
			writeError(w, http.StatusRequestEntityTooLarge, "слишком большой запрос")
			return
		}
		status = http.StatusBadRequest
		writeError(w, http.StatusBadRequest, "некорректный json")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		h.metrics.IncErrors()
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
			writeError(w, http.StatusRequestEntityTooLarge, "слишком большой запрос")
			return
		}
		status = http.StatusBadRequest
		writeError(w, http.StatusBadRequest, "некорректный json")
		return
	}

	consumer := r.Header.Get(ConsumerHeader)
	if consumer == "" {
		consumer = DefaultConsumer
	}

	result, err := h.svc.Process(r.Context(), req.Payload, req.PayloadID, consumer)
	if err != nil {
		h.metrics.IncErrors()
		switch {
		case errors.Is(err, ErrInvalid):
			status = http.StatusBadRequest
			writeError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, ErrForbidden):
			status = http.StatusForbidden
			writeError(w, http.StatusForbidden, err.Error())
		case errors.Is(err, detection.ErrOverloaded):
			status = http.StatusServiceUnavailable
			h.metrics.IncOverloaded()
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusServiceUnavailable, "сервис перегружен, повторите запрос")
		default:
			status = http.StatusInternalServerError
			h.log.Error("process failed", "error", err)
			writeError(w, http.StatusInternalServerError, "внутренняя ошибка")
		}
		return
	}

	h.metrics.AddTokens(int64(len(result)))
	writeJSON(w, http.StatusOK, ProcessResponse{Result: result})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
