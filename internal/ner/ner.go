// Пакет ner предоставляет клиент для локального NER sidecar (slovnet + navec).
//
// Sidecar возвращает сырые кандидаты с метками PER/LOC/ORG и байтовыми
// позициями в исходной строке. Эти кандидаты НЕ являются готовыми типами PII:
// решение о том, какой pii.Type присвоить (или отбросить) принимает следующий
// слой по контексту. Поэтому результаты NER не подключаются напрямую в общий
// detection.Detector.
//
// Клиент использует bounded microbatcher: конкурентные Detect ставят запросы в
// ограниченную очередь, воркер накапливает их в batch (до maxBatch или maxWait)
// и отправляет одним вызовом /ner/batch. Порядок результатов совпадает с
// порядком входов. Учитывается context cancellation: отменённые запросы не
// зависают и не блокируют воркер.
package ner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Параметры microbatcher по умолчанию.
const (
	DefaultMaxBatch  = 16
	DefaultMaxWait   = 8 * time.Millisecond
	DefaultQueueSize = 256
	maxStatsSamples  = 10_000
)

var ErrClientClosed = errors.New("ner: client is closed")

// Label — тип именованной сущности, возвращаемой NER.
type Label string

// Стандартные метки NER.
const (
	LabelPER Label = "PER" // имя человека
	LabelLOC Label = "LOC" // локация
	LabelORG Label = "ORG" // организация
)

// Candidate — сырой кандидат именованной сущности.
//
// Start/End — байтовые смещения в ИСХОДНОЙ строке UTF-8 (start включительно,
// end исключительно), так что text[start:end] возвращает ровно фрагмент.
type Candidate struct {
	Label Label
	Start int
	End   int
	Text  string
}

// Options — параметры microbatcher.
type Options struct {
	MaxBatch  int
	MaxWait   time.Duration
	QueueSize int
}

// batchItem — один запрос в очереди microbatcher.
type batchItem struct {
	text       string
	ctx        context.Context
	ch         chan batchResult
	enqueuedAt time.Time
}

// batchResult — результат обработки одного элемента batch.
type batchResult struct {
	cands []Candidate
	err   error
}

// Client — HTTP-клиент для NER sidecar с microbatcher.
type Client struct {
	baseURL   string
	http      *http.Client
	transport *http.Transport

	maxBatch int
	maxWait  time.Duration
	queue    chan *batchItem
	done     chan struct{}

	wg        sync.WaitGroup
	closeOnce sync.Once

	// Статистика для диагностики (не влияет на логику).
	statsMu    sync.Mutex
	batchSizes []int
	queueWaits []time.Duration
	httpRTTs   []time.Duration
	sidecarMap []time.Duration
	sidecarTot []time.Duration

	// conns — число новых TCP-соединений, открытых к sidecar. Считается в
	// DialContext. Если keep-alive работает, conns должно быть малым по
	// сравнению с числом batch-запросов.
	conns atomic.Int64
}

// BatchTiming — разбивка времени обработки одного batch.
type BatchTiming struct {
	// QueueWait — время ожидания первого элемента batch в Go-очереди.
	QueueWait time.Duration
	// Size — фактический размер batch (число текстов).
	Size int
	// HTTPRTT — полное время Go HTTP round trip до sidecar и обратно.
	HTTPRTT time.Duration
	// SidecarMap — время Slovnet map внутри Python (из служебных данных ответа).
	SidecarMap time.Duration
	// SidecarTotal — общее время обработки batch внутри Python (map + сериализация).
	SidecarTotal time.Duration
}

// Stats — сводка измерений microbatcher.
type Stats struct {
	BatchSizes []int
	QueueWaits []time.Duration
	HTTPRTTs   []time.Duration
	// SidecarMap — время Slovnet map на batch (из служебных данных sidecar).
	SidecarMap []time.Duration
	// SidecarTotal — общее время обработки batch внутри Python.
	SidecarTotal []time.Duration
}

// Stats возвращает копию собранной статистики.
func (c *Client) Stats() Stats {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	return Stats{
		BatchSizes:   append([]int(nil), c.batchSizes...),
		QueueWaits:   append([]time.Duration(nil), c.queueWaits...),
		HTTPRTTs:     append([]time.Duration(nil), c.httpRTTs...),
		SidecarMap:   append([]time.Duration(nil), c.sidecarMap...),
		SidecarTotal: append([]time.Duration(nil), c.sidecarTot...),
	}
}

// ResetStats очищает собранную диагностическую статистику. Используется между
// режимами нагрузки, чтобы результаты не смешивались.
func (c *Client) ResetStats() {
	c.statsMu.Lock()
	defer c.statsMu.Unlock()
	c.batchSizes = c.batchSizes[:0]
	c.queueWaits = c.queueWaits[:0]
	c.httpRTTs = c.httpRTTs[:0]
	c.sidecarMap = c.sidecarMap[:0]
	c.sidecarTot = c.sidecarTot[:0]
}

func (c *Client) recordBatch(size int) {
	c.statsMu.Lock()
	c.batchSizes = appendStat(c.batchSizes, size)
	c.statsMu.Unlock()
}

func (c *Client) recordQueueWait(d time.Duration) {
	c.statsMu.Lock()
	c.queueWaits = appendStat(c.queueWaits, d)
	c.statsMu.Unlock()
}

func (c *Client) recordHTTPRTT(d time.Duration) {
	c.statsMu.Lock()
	c.httpRTTs = appendStat(c.httpRTTs, d)
	c.statsMu.Unlock()
}

func (c *Client) recordSidecarTiming(mapMs, totalMs time.Duration) {
	c.statsMu.Lock()
	c.sidecarMap = appendStat(c.sidecarMap, mapMs)
	c.sidecarTot = appendStat(c.sidecarTot, totalMs)
	c.statsMu.Unlock()
}

func appendStat[T any](samples []T, value T) []T {
	if len(samples) >= maxStatsSamples {
		keep := maxStatsSamples / 2
		copy(samples[:keep], samples[len(samples)-keep:])
		samples = samples[:keep]
	}
	return append(samples, value)
}

// NewClient создаёт клиент для sidecar по адресу baseURL (например,
// "http://127.0.0.1:8090") с параметрами microbatcher по умолчанию.
func NewClient(baseURL string) *Client {
	return NewClientWithOptions(baseURL, Options{
		MaxBatch:  DefaultMaxBatch,
		MaxWait:   DefaultMaxWait,
		QueueSize: DefaultQueueSize,
	})
}

// NewClientWithOptions создаёт клиент с заданными параметрами microbatcher.
func NewClientWithOptions(baseURL string, opts Options) *Client {
	if opts.MaxBatch <= 0 {
		opts.MaxBatch = DefaultMaxBatch
	} else if opts.MaxBatch > DefaultMaxBatch {
		opts.MaxBatch = DefaultMaxBatch
	}
	if opts.MaxWait <= 0 {
		opts.MaxWait = DefaultMaxWait
	}
	if opts.QueueSize <= 0 {
		opts.QueueSize = DefaultQueueSize
	}
	c := &Client{
		baseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		maxBatch: opts.MaxBatch,
		maxWait:  opts.MaxWait,
		queue:    make(chan *batchItem, opts.QueueSize),
		done:     make(chan struct{}),
	}
	// Явный Transport с keep-alive (DisableKeepAlives=false) и подсчётом новых
	// TCP-соединений. Один переиспользуемый http.Client на весь клиент.
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	tr := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			c.conns.Add(1)
			return dialer.DialContext(ctx, network, addr)
		},
		DisableKeepAlives:   false,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	c.http = &http.Client{Transport: tr, Timeout: 30 * time.Second}
	c.transport = tr
	c.wg.Add(1)
	go c.worker()
	return c
}

// Connections возвращает число новых TCP-соединений, открытых к sidecar.
// Используется для проверки, что keep-alive реально работает: при
// переиспользовании соединений это число должно быть малым по сравнению с
// числом batch-запросов.
func (c *Client) Connections() int64 {
	return c.conns.Load()
}

// CloseIdleConnections закрывает простаивающие keep-alive соединения
// Transport. Используется для освобождения ресурсов (и их goroutine) после
// завершения работы клиента.
func (c *Client) CloseIdleConnections() {
	c.transport.CloseIdleConnections()
}

// Close останавливает воркер microbatcher и закрывает простаивающие HTTP
// keep-alive соединения. После Close клиент нельзя использовать. Close
// безопасно вызывать несколько раз.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		c.wg.Wait()
		c.transport.CloseIdleConnections()
	})
}

// Detect отправляет текст в sidecar и возвращает сырые кандидаты NER.
// Запрос ставится в очередь microbatcher; результат возвращается после
// обработки batch. Учитывается отмена контекста.
func (c *Client) Detect(ctx context.Context, text string) ([]Candidate, error) {
	item := &batchItem{text: text, ctx: ctx, ch: make(chan batchResult, 1), enqueuedAt: time.Now()}
	select {
	case c.queue <- item:
	case <-c.done:
		return nil, ErrClientClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case res := <-item.ch:
		return res.cands, res.err
	case <-c.done:
		return nil, ErrClientClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// worker накапливает запросы в batch и отправляет их в sidecar.
func (c *Client) worker() {
	defer c.wg.Done()
	for {
		items := c.collectBatch()
		if len(items) == 0 {
			return
		}
		c.processBatch(items)
	}
}

// collectBatch накапливает batch из очереди. Ждёт первый элемент (блокирующе,
// до закрытия очереди), затем накапливает до maxBatch или maxWait. Отменённые
// элементы пропускаются и получают ошибку сразу.
func (c *Client) collectBatch() []*batchItem {
	// Читаем первый элемент (блокирующе, до закрытия очереди).
	var first *batchItem
	for first == nil {
		select {
		case <-c.done:
			return nil
		case item := <-c.queue:
			if item.ctx.Err() != nil {
				item.ch <- batchResult{err: item.ctx.Err()}
				continue
			}
			first = item
		}
	}

	items := []*batchItem{first}
	c.recordQueueWait(time.Since(first.enqueuedAt))
	timer := time.NewTimer(c.maxWait)
	defer timer.Stop()
	for len(items) < c.maxBatch {
		select {
		case <-c.done:
			c.recordBatch(len(items))
			return items
		case item := <-c.queue:
			if item.ctx.Err() != nil {
				item.ch <- batchResult{err: item.ctx.Err()}
				continue
			}
			c.recordQueueWait(time.Since(item.enqueuedAt))
			items = append(items, item)
		case <-timer.C:
			c.recordBatch(len(items))
			return items
		}
	}
	c.recordBatch(len(items))
	return items
}

// processBatch отправляет batch в sidecar и распределяет результаты по
// элементам. Отменённые элементы не блокируют воркер.
func (c *Client) processBatch(items []*batchItem) {
	texts := make([]string, len(items))
	for i, it := range items {
		texts[i] = it.text
	}
	start := time.Now()
	results, timing, err := c.sendBatch(texts)
	c.recordHTTPRTT(time.Since(start))
	if timing != nil {
		c.recordSidecarTiming(timing.mapMs, timing.totalMs)
	}
	for i, it := range items {
		var res batchResult
		if err != nil {
			res = batchResult{err: err}
		} else {
			res = batchResult{cands: results[i]}
		}
		select {
		case it.ch <- res:
		case <-it.ctx.Done():
			// Контекст отменён — не отправляем результат, воркер не блокируется.
		}
	}
}

// sidecarTiming — служебные timing-данные, возвращаемые sidecar в ответе.
type sidecarTiming struct {
	mapMs   time.Duration
	totalMs time.Duration
}

// sendBatch отправляет массив текстов в /ner/batch и возвращает кандидатов
// для каждого текста в том же порядке, а также служебные timing-данные.
func (c *Client) sendBatch(texts []string) ([][]Candidate, *sidecarTiming, error) {
	body, err := json.Marshal(map[string][]string{"texts": texts})
	if err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/ner/batch", bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("ner: batch request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("ner: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("ner: sidecar returned %d: %s", resp.StatusCode, string(data))
	}

	var out struct {
		Results []struct {
			Spans []struct {
				Start int    `json:"start"`
				End   int    `json:"end"`
				Type  string `json:"type"`
				Text  string `json:"text"`
			} `json:"spans"`
		} `json:"results"`
		Timing *struct {
			MapMs   float64 `json:"map_ms"`
			SerMs   float64 `json:"ser_ms"`
			TotalMs float64 `json:"total_ms"`
		} `json:"timing"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, nil, fmt.Errorf("ner: decode response: %w", err)
	}
	if len(out.Results) != len(texts) {
		return nil, nil, fmt.Errorf("ner: batch response count mismatch: got %d want %d", len(out.Results), len(texts))
	}

	results := make([][]Candidate, len(out.Results))
	for i, r := range out.Results {
		cands := make([]Candidate, 0, len(r.Spans))
		for _, s := range r.Spans {
			if s.Start < 0 || s.End <= s.Start || s.End > len(texts[i]) || texts[i][s.Start:s.End] != s.Text {
				return nil, nil, fmt.Errorf("ner: invalid span for result %d: %d:%d %q", i, s.Start, s.End, s.Text)
			}
			cands = append(cands, Candidate{
				Label: Label(s.Type),
				Start: s.Start,
				End:   s.End,
				Text:  s.Text,
			})
		}
		results[i] = cands
	}

	var timing *sidecarTiming
	if out.Timing != nil {
		timing = &sidecarTiming{
			mapMs:   time.Duration(out.Timing.MapMs * float64(time.Millisecond)),
			totalMs: time.Duration(out.Timing.TotalMs * float64(time.Millisecond)),
		}
	}
	return results, timing, nil
}
