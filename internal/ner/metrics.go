package ner

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Prometheus возвращает bounded-диагностику NER. Квантили считаются только
// при scrape по последним не более чем maxStatsSamples значениям.
func (c *Client) Prometheus() string {
	s := c.Stats()
	var b strings.Builder
	metricInt(&b, "pii_ner_batches_total", s.BatchesTotal)
	metricInt(&b, "pii_ner_texts_total", s.TextsTotal)
	metricInt(&b, "pii_ner_batch_errors_total", s.BatchErrors)
	metricInt(&b, "pii_ner_queue_depth", int64(s.QueueDepth))
	metricInt(&b, "pii_ner_queue_depth_max", s.MaxQueueDepth)
	metricInt(&b, "pii_ner_queue_capacity", int64(s.QueueCapacity))
	metricInt(&b, "pii_ner_connections_total", s.Connections)
	if s.BatchesTotal > 0 {
		metricFloat(&b, "pii_ner_batch_size_avg", float64(s.TextsTotal)/float64(s.BatchesTotal))
	}
	writeDurationQuantiles(&b, "pii_ner_queue_wait", s.QueueWaits)
	writeDurationQuantiles(&b, "pii_ner_http_rtt", s.HTTPRTTs)
	writeDurationQuantiles(&b, "pii_ner_sidecar_map", s.SidecarMap)
	writeDurationQuantiles(&b, "pii_ner_sidecar_total", s.SidecarTotal)
	return b.String()
}

func writeDurationQuantiles(b *strings.Builder, name string, values []time.Duration) {
	if len(values) == 0 {
		return
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	for _, q := range []struct {
		name string
		p    int
	}{{"p50", 50}, {"p95", 95}, {"p99", 99}} {
		index := (len(values) - 1) * q.p / 100
		metricFloat(b, name+"_"+q.name+"_ms", float64(values[index])/float64(time.Millisecond))
	}
}

func metricInt(b *strings.Builder, name string, value int64) {
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatInt(value, 10))
	b.WriteByte('\n')
}

func metricFloat(b *strings.Builder, name string, value float64) {
	b.WriteString(name)
	b.WriteByte(' ')
	b.WriteString(strconv.FormatFloat(value, 'f', 6, 64))
	b.WriteByte('\n')
}
