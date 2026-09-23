package metrics

import (
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var requestDurationBounds = [...]time.Duration{5 * time.Millisecond, 10 * time.Millisecond, 25 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond, 250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2500 * time.Millisecond, 5 * time.Second, 10 * time.Second, 30 * time.Second}

type Metrics struct {
	requestsTotal, errorsTotal, rateLimited, overloaded, tokensProcessed atomic.Int64
	inFlight, maxInFlight                                                atomic.Int64
	responses                                                            [6]atomic.Int64
	latencySum, latencyCount                                             atomic.Int64
	latencyBuckets                                                       [len(requestDurationBounds) + 1]atomic.Int64
	start                                                                time.Time
}

func New() *Metrics { return &Metrics{start: time.Now()} }
func (m *Metrics) RequestStarted() {
	m.requestsTotal.Add(1)
	current := m.inFlight.Add(1)
	for {
		maximum := m.maxInFlight.Load()
		if current <= maximum || m.maxInFlight.CompareAndSwap(maximum, current) {
			return
		}
	}
}
func (m *Metrics) RequestFinished(status int, elapsed time.Duration) {
	m.inFlight.Add(-1)
	class := status / 100
	if class >= 1 && class <= 5 {
		m.responses[class].Add(1)
	}
	m.ObserveLatency(elapsed.Nanoseconds())
}
func (m *Metrics) IncErrors()        { m.errorsTotal.Add(1) }
func (m *Metrics) IncRateLimited()   { m.rateLimited.Add(1) }
func (m *Metrics) IncOverloaded()    { m.overloaded.Add(1) }
func (m *Metrics) AddTokens(n int64) { m.tokensProcessed.Add(n) }
func (m *Metrics) ObserveLatency(ns int64) {
	if ns < 0 {
		ns = 0
	}
	m.latencySum.Add(ns)
	m.latencyCount.Add(1)
	bucket := len(requestDurationBounds)
	for i, upper := range requestDurationBounds {
		if time.Duration(ns) <= upper {
			bucket = i
			break
		}
	}
	m.latencyBuckets[bucket].Add(1)
}

type Snapshot struct {
	UptimeSeconds                                            float64
	RequestsTotal, ErrorsTotal, RateLimited, TokensProcessed int64
	RPS, LatencyAvgMs                                        float64
	LatencyCount, InFlight, MaxInFlight                      int64
}

func (m *Metrics) Snapshot() Snapshot {
	up := time.Since(m.start).Seconds()
	req := m.requestsTotal.Load()
	return Snapshot{up, req, m.errorsTotal.Load(), m.rateLimited.Load(), m.tokensProcessed.Load(), float64(req) / up, avgMs(m.latencySum.Load(), m.latencyCount.Load()), m.latencyCount.Load(), m.inFlight.Load(), m.maxInFlight.Load()}
}
func avgMs(sum, count int64) float64 {
	if count == 0 {
		return 0
	}
	return float64(sum) / float64(count) / 1e6
}

func (m *Metrics) Prometheus() string {
	s := m.Snapshot()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	var b strings.Builder
	wi(&b, "pii_requests_total", s.RequestsTotal)
	wi(&b, "pii_errors_total", s.ErrorsTotal)
	wi(&b, "pii_rate_limited_total", s.RateLimited)
	wi(&b, "pii_overloaded_total", m.overloaded.Load())
	wi(&b, "pii_tokens_processed_total", s.TokensProcessed)
	wi(&b, "pii_output_bytes_total", s.TokensProcessed)
	wf(&b, "pii_rps", s.RPS)
	wf(&b, "pii_latency_avg_ms", s.LatencyAvgMs)
	wf(&b, "pii_uptime_seconds", s.UptimeSeconds)
	wi(&b, "pii_in_flight_requests", s.InFlight)
	wi(&b, "pii_in_flight_requests_max", s.MaxInFlight)
	for class := 1; class <= 5; class++ {
		b.WriteString("pii_responses_total{class=\"")
		b.WriteString(strconv.Itoa(class))
		b.WriteString("xx\"} ")
		b.WriteString(strconv.FormatInt(m.responses[class].Load(), 10))
		b.WriteByte('\n')
	}
	cumulative := int64(0)
	for i, upper := range requestDurationBounds {
		cumulative += m.latencyBuckets[i].Load()
		b.WriteString("pii_request_duration_seconds_bucket{le=\"")
		b.WriteString(strconv.FormatFloat(upper.Seconds(), 'f', 3, 64))
		b.WriteString("\"} ")
		b.WriteString(strconv.FormatInt(cumulative, 10))
		b.WriteByte('\n')
	}
	cumulative += m.latencyBuckets[len(requestDurationBounds)].Load()
	b.WriteString("pii_request_duration_seconds_bucket{le=\"+Inf\"} ")
	b.WriteString(strconv.FormatInt(cumulative, 10))
	b.WriteByte('\n')
	wf(&b, "pii_request_duration_seconds_sum", float64(m.latencySum.Load())/float64(time.Second))
	wi(&b, "pii_request_duration_seconds_count", m.latencyCount.Load())
	wi(&b, "pii_runtime_goroutines", int64(runtime.NumGoroutine()))
	wu(&b, "pii_runtime_heap_alloc_bytes", mem.HeapAlloc)
	wu(&b, "pii_runtime_heap_inuse_bytes", mem.HeapInuse)
	wu(&b, "pii_runtime_sys_bytes", mem.Sys)
	wu(&b, "pii_runtime_gc_cycles_total", uint64(mem.NumGC))
	return b.String()
}
func wi(b *strings.Builder, n string, v int64) {
	b.WriteString(n)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(v, 10))
	b.WriteByte('\n')
}
func wu(b *strings.Builder, n string, v uint64) {
	b.WriteString(n)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatUint(v, 10))
	b.WriteByte('\n')
}
func wf(b *strings.Builder, n string, v float64) {
	b.WriteString(n)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatFloat(v, 'f', 6, 64))
	b.WriteByte('\n')
}
