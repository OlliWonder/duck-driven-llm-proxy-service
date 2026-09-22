// Пакет metrics собирает эксплуатационные метрики: задержку, скорость
// запросов (RPS) и пропускную способность токенов (TPS). Он предоставляет
// текстовый эндпоинт в стиле Prometheus и внутрипроцессные счётчики.
package metrics

import (
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Metrics агрегирует счётчики и датчики для сервиса.
type Metrics struct {
	mu sync.Mutex

	requestsTotal   atomic.Int64
	errorsTotal     atomic.Int64
	rateLimited     atomic.Int64
	tokensProcessed atomic.Int64

	// окно задержки
	latencySum   atomic.Int64 // наносекунды
	latencyCount atomic.Int64

	start time.Time
}

// New возвращает коллектор метрик.
func New() *Metrics {
	return &Metrics{start: time.Now()}
}

// IncRequests увеличивает общий счётчик запросов.
func (m *Metrics) IncRequests() { m.requestsTotal.Add(1) }

// IncErrors увеличивает счётчик ошибок.
func (m *Metrics) IncErrors() { m.errorsTotal.Add(1) }

// IncRateLimited увеличивает счётчик ответов 429.
func (m *Metrics) IncRateLimited() { m.rateLimited.Add(1) }

// AddTokens добавляет n обработанных токенов.
func (m *Metrics) AddTokens(n int64) { m.tokensProcessed.Add(n) }

// ObserveLatency фиксирует задержку запроса в наносекундах.
func (m *Metrics) ObserveLatency(ns int64) {
	m.latencySum.Add(ns)
	m.latencyCount.Add(1)
}

// Snapshot возвращает мгновенный срез счётчиков.
func (m *Metrics) Snapshot() Snapshot {
	up := time.Since(m.start).Seconds()
	req := m.requestsTotal.Load()
	return Snapshot{
		UptimeSeconds:   up,
		RequestsTotal:   req,
		ErrorsTotal:     m.errorsTotal.Load(),
		RateLimited:     m.rateLimited.Load(),
		TokensProcessed: m.tokensProcessed.Load(),
		RPS:             float64(req) / up,
		LatencyAvgMs:    avgMs(m.latencySum.Load(), m.latencyCount.Load()),
		LatencyCount:    m.latencyCount.Load(),
	}
}

// Snapshot — мгновенный срез метрик.
type Snapshot struct {
	UptimeSeconds   float64
	RequestsTotal   int64
	ErrorsTotal     int64
	RateLimited     int64
	TokensProcessed int64
	RPS             float64
	LatencyAvgMs    float64
	LatencyCount    int64
}

func avgMs(sumNs, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(sumNs) / float64(count) / 1e6
}

// Prometheus отображает метрики в текстовом формате Prometheus.
func (m *Metrics) Prometheus() string {
	s := m.Snapshot()
	return "pii_requests_total " + itoa(s.RequestsTotal) + "\n" +
		"pii_errors_total " + itoa(s.ErrorsTotal) + "\n" +
		"pii_rate_limited_total " + itoa(s.RateLimited) + "\n" +
		"pii_tokens_processed_total " + itoa(s.TokensProcessed) + "\n" +
		"pii_rps " + ftoa(s.RPS) + "\n" +
		"pii_latency_avg_ms " + ftoa(s.LatencyAvgMs) + "\n" +
		"pii_uptime_seconds " + ftoa(s.UptimeSeconds) + "\n"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func ftoa(f float64) string {
	return strconv.FormatFloat(f, 'f', 3, 64)
}