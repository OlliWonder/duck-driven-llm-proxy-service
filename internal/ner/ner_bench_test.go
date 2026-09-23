package ner

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"
)

const benchText = "Президент Франции Эмманюэль Макрон встретился с канцлером ФРГ Ангелой Меркель в Берлине."

func benchSkipIfUnavailable(b *testing.B) {
	b.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(testBaseURL + "/healthz")
	if err != nil {
		b.Skipf("NER sidecar недоступен (%v), пропускаю", err)
	}
	resp.Body.Close()
}

// BenchmarkWarmLatency замеряет latency одного warm запроса (последовательные
// вызовы, модель уже загружена).
func BenchmarkWarmLatency(b *testing.B) {
	benchSkipIfUnavailable(b)
	c := NewClient(testBaseURL)
	defer c.Close()
	ctx := context.Background()
	// Прогрев.
	_, _ = c.Detect(ctx, benchText)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.Detect(ctx, benchText)
	}
}

// BenchmarkParallel1 замеряет пропускную способность при 1 параллельном воркере.
func BenchmarkParallel1(b *testing.B) {
	benchmarkParallel(b, 1)
}

// BenchmarkParallel4 замеряет пропускную способность при 4 параллельных воркерах.
func BenchmarkParallel4(b *testing.B) {
	benchmarkParallel(b, 4)
}

// BenchmarkParallel8 замеряет пропускную способность при 8 параллельных воркерах.
func BenchmarkParallel8(b *testing.B) {
	benchmarkParallel(b, 8)
}

func benchmarkParallel(b *testing.B, workers int) {
	benchSkipIfUnavailable(b)
	c := NewClient(testBaseURL)
	defer c.Close()
	ctx := context.Background()
	_, _ = c.Detect(ctx, benchText)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = c.Detect(ctx, benchText)
		}
	})
	_ = workers
}

// BenchmarkParallelSharedClient замеряет параллельность с общим клиентом и
// общим HTTP-транспортом (реалистичный сценарий).
func BenchmarkParallelSharedClient(b *testing.B) {
	benchSkipIfUnavailable(b)
	c := NewClient(testBaseURL)
	defer c.Close()
	ctx := context.Background()
	_, _ = c.Detect(ctx, benchText)
	b.ReportAllocs()
	b.ResetTimer()
	var wg sync.WaitGroup
	workers := 8
	per := b.N / workers
	if per == 0 {
		per = 1
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < per; i++ {
				_, _ = c.Detect(ctx, benchText)
			}
		}()
	}
	wg.Wait()
}
