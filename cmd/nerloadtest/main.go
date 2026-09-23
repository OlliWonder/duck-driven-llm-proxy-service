// Команда nerloadtest замеряет производительность NER microbatcher под
// длительной нагрузкой.
//
// Использует ner.Detector (microbatcher + валидатор) с уникальными текстами,
// гоняет запросы заданное время с заданной concurrency и выводит RPS,
// p50/p95/p99 полного Detect, а также разбивку по этапам: batch size,
// queue wait, HTTP RTT, Slovnet map, Python total, validator time.
//
// Запуск:
//   go run ./cmd/nerloadtest [--duration 20s] [--concurrency 16,32,64,128]
package main

import (
	"context"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/duck-driven-llm-proxy-service/internal/ner"
)

const baseURL = "http://127.0.0.1:8090"

func main() {
	duration := flag.Duration("duration", 20*time.Second, "длительность каждого режима")
	concFlag := flag.String("concurrency", "16,32,64,128", "список concurrency")
	mode := flag.String("mode", "unique", "режим текстов: unique | repeat")
	poolSize := flag.Int("pool", 100, "размер пула текстов для режима repeat")
	workers := flag.Int("workers", 7, "число параллельных NER batch workers")
	flag.Parse()

	client := ner.NewClientWithWorkers(baseURL, *workers)
	defer client.Close()
	det := ner.NewDetector(client)

	// Прогрев.
	ctx := context.Background()
	_, _ = det.Detect(ctx, "клиент Иван Петров прогрев")

	fmt.Println("=== NER microbatcher load test (duration-based) ===")
	fmt.Printf("duration=%s mode=%s pool=%d workers=%d\n", *duration, *mode, *poolSize, *workers)
	fmt.Println()

	for _, conc := range parseConcurrency(*concFlag) {
		runMode(det, client, conc, *duration, *mode, *poolSize)
	}
}

type modeResult struct {
	concurrency int
	completed   int
	errors      int
	rps         float64
	detectP50   float64
	detectP95   float64
	detectP99   float64

	batchSizeAvg float64
	batchSizeP95 float64
	queueWaitAvg float64
	queueWaitP95 float64
	httpRTTAvg   float64
	httpRTTP95   float64
	mapAvg       float64
	mapP95       float64
	pyTotalAvg   float64
	pyTotalP95   float64
	valAvg       float64
	valP95       float64
	conns        int64
	batchCount   int
	cacheHits    int64
	cacheMisses  int64
}

func runMode(det *ner.Detector, client *ner.Client, concurrency int, duration time.Duration, mode string, poolSize int) {
	// Сбрасываем диагностическую статистику перед режимом.
	client.ResetStats()
	det.ResetValidatorStats()
	det.Cache().ResetStats()

	ctx := context.Background()
	latencies := make([]time.Duration, 0, 4096)
	var mu sync.Mutex
	var errors int
	counter := 0
	var counterMu sync.Mutex

	stop := make(chan struct{})
	var wg sync.WaitGroup
	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				counterMu.Lock()
				idx := counter
				counter++
				counterMu.Unlock()

				text := makeText(idx, mode, poolSize)
				t0 := time.Now()
				_, err := det.Detect(ctx, text)
				lat := time.Since(t0)
				mu.Lock()
				if err != nil {
					errors++
				} else {
					latencies = append(latencies, lat)
				}
				mu.Unlock()
			}
		}()
	}

	// Останавливаем ровно через duration.
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	elapsed := time.Since(start)

	completed := len(latencies)
	rps := float64(completed) / elapsed.Seconds()
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	stats := client.Stats()
	valDurs := det.ValidatorStats()
	cacheStats := det.Cache().Stats()

	res := modeResult{
		concurrency: concurrency,
		completed:   completed,
		errors:      errors,
		rps:         rps,
		detectP50:   percentileMs(sorted, 50),
		detectP95:   percentileMs(sorted, 95),
		detectP99:   percentileMs(sorted, 99),

		batchSizeAvg: avgInt(stats.BatchSizes),
		batchSizeP95: p95Int(stats.BatchSizes),
		queueWaitAvg: avgDurMs(stats.QueueWaits),
		queueWaitP95: p95DurMs(stats.QueueWaits),
		httpRTTAvg:   avgDurMs(stats.HTTPRTTs),
		httpRTTP95:   p95DurMs(stats.HTTPRTTs),
		mapAvg:       avgDurMs(stats.SidecarMap),
		mapP95:       p95DurMs(stats.SidecarMap),
		pyTotalAvg:   avgDurMs(stats.SidecarTotal),
		pyTotalP95:   p95DurMs(stats.SidecarTotal),
		valAvg:       avgDurMs(valDurs),
		valP95:       p95DurMs(valDurs),
		conns:        client.Connections(),
		batchCount:   len(stats.BatchSizes),
		cacheHits:    cacheStats.Hits,
		cacheMisses:  cacheStats.Misses,
	}

	printResult(res)
}

// makeText генерирует текст. В режиме unique каждый idx уникален; в режиме
// repeat idx циклируется по пулу, чтобы попадать в кеш.
func makeText(idx int, mode string, poolSize int) string {
	if mode == "repeat" && poolSize > 0 {
		idx = idx % poolSize
	}
	return fmt.Sprintf("клиент Иван Петров %d, адрес клиента: Москва, ул Ленина %d, кем выдан: ОУФМС России %d", idx, idx, idx)
}

func printResult(r modeResult) {
	fmt.Printf("=== concurrency %d ===\n", r.concurrency)
	fmt.Printf("  completed=%d errors=%d RPS=%.1f\n", r.completed, r.errors, r.rps)
	fmt.Printf("  Detect p50=%.2fms p95=%.2fms p99=%.2fms\n", r.detectP50, r.detectP95, r.detectP99)
	fmt.Printf("  batch size   avg=%.1f p95=%.1f\n", r.batchSizeAvg, r.batchSizeP95)
	fmt.Printf("  queue wait   avg=%.2fms p95=%.2fms\n", r.queueWaitAvg, r.queueWaitP95)
	fmt.Printf("  HTTP RTT     avg=%.2fms p95=%.2fms\n", r.httpRTTAvg, r.httpRTTP95)
	fmt.Printf("  Slovnet map  avg=%.2fms p95=%.2fms\n", r.mapAvg, r.mapP95)
	fmt.Printf("  Python total avg=%.2fms p95=%.2fms\n", r.pyTotalAvg, r.pyTotalP95)
	fmt.Printf("  validator    avg=%.2fms p95=%.2fms\n", r.valAvg, r.valP95)
	fmt.Printf("  TCP conns=%d (keep-alive: %s)\n", r.conns, keepAliveVerdict(r))
	fmt.Printf("  cache hits=%d misses=%d hit_rate=%.1f%%\n", r.cacheHits, r.cacheMisses, hitRate(r))
	fmt.Println()
}

func hitRate(r modeResult) float64 {
	total := r.cacheHits + r.cacheMisses
	if total == 0 {
		return 0
	}
	return float64(r.cacheHits) / float64(total) * 100
}

// keepAliveVerdict оценивает, работает ли keep-alive: если число новых TCP
// соединений много меньше числа batch-запросов, соединения переиспользуются.
func keepAliveVerdict(r modeResult) string {
	batches := int64(r.batchCount)
	if batches == 0 {
		return "n/a"
	}
	if r.conns <= 1 {
		return "yes (single conn)"
	}
	if r.conns*10 < batches {
		return fmt.Sprintf("yes (%.1f req/conn)", float64(batches)/float64(r.conns))
	}
	return "NO (conn per batch)"
}

func percentileMs(sorted []time.Duration, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p / 100)
	return float64(sorted[idx].Microseconds()) / 1000.0
}

func avgDurMs(ds []time.Duration) float64 {
	if len(ds) == 0 {
		return 0
	}
	var sum int64
	for _, d := range ds {
		sum += int64(d)
	}
	return float64(sum) / float64(len(ds)) / 1e6
}

func p95DurMs(ds []time.Duration) float64 {
	if len(ds) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(ds))
	copy(sorted, ds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	idx := int(float64(len(sorted)-1) * 95 / 100)
	return float64(sorted[idx].Microseconds()) / 1000.0
}

func avgInt(vs []int) float64 {
	if len(vs) == 0 {
		return 0
	}
	var sum int
	for _, v := range vs {
		sum += v
	}
	return float64(sum) / float64(len(vs))
}

func p95Int(vs []int) float64 {
	if len(vs) == 0 {
		return 0
	}
	sorted := make([]int, len(vs))
	copy(sorted, vs)
	sort.Ints(sorted)
	idx := int(float64(len(sorted)-1) * 95 / 100)
	return float64(sorted[idx])
}

func parseConcurrency(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ",") {
		if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			out = append(out, n)
		}
	}
	return out
}
