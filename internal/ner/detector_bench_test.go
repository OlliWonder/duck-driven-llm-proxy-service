package ner

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// benchDetectorText — текст с несколькими сущностями для полного прохода.
const benchDetectorText = "клиент Александр Пушкин, адрес клиента: Москва, ул Ленина, 1, кем выдан: ОУФМС России"

// percentile возвращает значение перцентиля p (0..100) отсортированного слайса.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p / 100)
	return sorted[idx]
}

// reportLatency вычисляет и публикует p50/p95/p99 из слайса latency.
func reportLatency(b *testing.B, latencies []time.Duration) {
	sorted := make([]time.Duration, len(latencies))
	copy(sorted, latencies)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	b.ReportMetric(float64(percentile(sorted, 50).Microseconds()), "p50_us/op")
	b.ReportMetric(float64(percentile(sorted, 95).Microseconds()), "p95_us/op")
	b.ReportMetric(float64(percentile(sorted, 99).Microseconds()), "p99_us/op")
}

// BenchmarkDetectorSequential измеряет повторный warm cache-hit полного
// production-детектора.
func BenchmarkDetectorSequential(b *testing.B) {
	benchSkipIfUnavailable(b)
	client := NewClient(testBaseURL)
	defer client.Close()
	d := NewProductionDetector(client)
	ctx := context.Background()
	// Прогрев.
	_, _ = d.Detect(ctx, benchDetectorText)

	latencies := make([]time.Duration, 0, min(b.N, maxStatsSamples))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		_, _ = d.Detect(ctx, benchDetectorText)
		if len(latencies) < maxStatsSamples {
			latencies = append(latencies, time.Since(start))
		}
	}
	b.StopTimer()
	reportLatency(b, latencies)
}

// BenchmarkDetectorColdUnique измеряет cache miss полного production-
// детектора. Каждый текст уникален, поэтому запускаются NER и валидатор.
func BenchmarkDetectorColdUnique(b *testing.B) {
	benchSkipIfUnavailable(b)
	client := NewClient(testBaseURL)
	defer client.Close()
	d := NewProductionDetector(client)
	ctx := context.Background()
	latencies := make([]time.Duration, 0, min(b.N, maxStatsSamples))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		text := fmt.Sprintf("%s request-%d", benchDetectorText, i)
		start := time.Now()
		_, _ = d.Detect(ctx, text)
		if len(latencies) < maxStatsSamples {
			latencies = append(latencies, time.Since(start))
		}
	}
	b.StopTimer()
	reportLatency(b, latencies)
}

// BenchmarkDetectorParallel4 замеряет полный NER detection при 4 параллельных
// запросах.
func BenchmarkDetectorParallel4(b *testing.B) {
	benchmarkDetectorParallel(b, 4)
}

// BenchmarkDetectorParallel8 замеряет полный NER detection при 8 параллельных
// запросах.
func BenchmarkDetectorParallel8(b *testing.B) {
	benchmarkDetectorParallel(b, 8)
}

func benchmarkDetectorParallel(b *testing.B, workers int) {
	benchSkipIfUnavailable(b)
	client := NewClient(testBaseURL)
	defer client.Close()
	d := NewProductionDetector(client)
	ctx := context.Background()
	_, _ = d.Detect(ctx, benchDetectorText)

	var mu sync.Mutex
	var next atomic.Int64
	var wg sync.WaitGroup
	latencies := make([]time.Duration, 0, min(b.N, maxStatsSamples))
	b.ResetTimer()
	wg.Add(workers)
	for worker := 0; worker < workers; worker++ {
		go func() {
			defer wg.Done()
			for {
				if next.Add(1) > int64(b.N) {
					return
				}
				start := time.Now()
				_, _ = d.Detect(ctx, benchDetectorText)
				mu.Lock()
				if len(latencies) < maxStatsSamples {
					latencies = append(latencies, time.Since(start))
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	b.StopTimer()
	reportLatency(b, latencies)
}
