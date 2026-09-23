#!/usr/bin/env python3
"""Измеряет время Slovnet map и сериализации на batch 16 (без HTTP).

Показывает, сколько времени занимает Python-сторона обработки batch.
"""

import json
import statistics
import sys
import time

sys.path.insert(0, r"C:\Users\user\Documents\Alfa\eotm-sfacall-infra\sfacall\duck-driven-llm-proxy-service\ner")
import server

server.load_models()


def text_factory(idx):
    return "клиент Иван Петров %d, адрес клиента: Москва, ул Ленина %d" % (idx, idx)


def main():
    batch_size = 16
    n_batches = 200
    map_times = []
    ser_times = []
    total_times = []

    for b in range(n_batches):
        texts = [text_factory(b * batch_size + i) for i in range(batch_size)]
        t0 = time.perf_counter()
        markups = list(server._ner.map(texts))
        t1 = time.perf_counter()
        results = []
        for text, markup in zip(texts, markups):
            spans = []
            for span in markup.spans:
                if span.type not in ("PER", "LOC", "ORG"):
                    continue
                start = len(text[: span.start].encode("utf-8"))
                end = len(text[: span.stop].encode("utf-8"))
                spans.append({"start": start, "end": end, "type": span.type, "text": text[span.start: span.stop]})
            results.append({"spans": spans})
        t2 = time.perf_counter()
        data = json.dumps({"results": results}, ensure_ascii=False).encode("utf-8")
        t3 = time.perf_counter()

        map_times.append((t1 - t0) * 1000)
        ser_times.append((t3 - t2) * 1000)
        total_times.append((t3 - t0) * 1000)

    def pct(vals, p):
        s = sorted(vals)
        return s[int((len(s) - 1) * p / 100)]

    print("batch_size=%d n_batches=%d" % (batch_size, n_batches))
    print("Slovnet map:  mean=%.2fms p50=%.2fms p95=%.2fms" % (
        statistics.mean(map_times), pct(map_times, 50), pct(map_times, 95)))
    print("Serialize:    mean=%.2fms p50=%.2fms p95=%.2fms" % (
        statistics.mean(ser_times), pct(ser_times, 50), pct(ser_times, 95)))
    print("Total py:     mean=%.2fms p50=%.2fms p95=%.2fms" % (
        statistics.mean(total_times), pct(total_times, 50), pct(total_times, 95)))
    # RPS если бы только map (без HTTP/Go).
    rps_map = batch_size / (statistics.mean(map_times) / 1000)
    rps_total = batch_size / (statistics.mean(total_times) / 1000)
    print("RPS (map only): %.1f" % rps_map)
    print("RPS (map+ser):  %.1f" % rps_total)


if __name__ == "__main__":
    main()