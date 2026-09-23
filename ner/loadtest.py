#!/usr/bin/env python3
"""Load test HTTP NER sidecar.

Измеряет реальный completed RPS, p50/p95/p99, ошибки, CPU и память процесса
sidecar при разной concurrency. Использует уникальные тексты, чтобы кеш (если
бы он был) не влиял на результаты.

Также проверяет batch-запросы через Slovnet map (потенциал, без переделки
архитектуры).

Запуск:
  python ner/loadtest.py [--host 127.0.0.1] [--port 8090] [--requests N]
"""

import argparse
import concurrent.futures
import json
import statistics
import sys
import threading
import time
import urllib.request

import psutil


def find_sidecar_pid(port):
    """Находит PID процесса, слушающего заданный порт."""
    for conn in psutil.net_connections(kind="tcp"):
        if conn.laddr and conn.laddr.port == port and conn.status == "LISTEN":
            return conn.pid
    return None


class ProcessMonitor:
    """Периодически считывает CPU и память процесса."""

    def __init__(self, pid):
        self.proc = psutil.Process(pid)
        self.cpu_samples = []
        self.mem_samples = []
        self._stop = False
        self._thread = None

    def start(self):
        self._thread = threading.Thread(target=self._run, daemon=True)
        self._thread.start()

    def _run(self):
        while not self._stop:
            try:
                self.cpu_samples.append(self.proc.cpu_percent(interval=0.3))
                self.mem_samples.append(self.proc.memory_info().rss)
            except psutil.NoSuchProcess:
                break

    def stop(self):
        self._stop = True
        if self._thread:
            self._thread.join(timeout=2)

    def summary(self):
        cpu = statistics.mean(self.cpu_samples) if self.cpu_samples else 0.0
        mem = max(self.mem_samples) if self.mem_samples else 0
        return cpu, mem


def percentile(sorted_vals, p):
    if not sorted_vals:
        return 0.0
    idx = int((len(sorted_vals) - 1) * p / 100)
    return sorted_vals[idx]


def http_ner(host, port, text):
    """Отправляет один запрос /ner, возвращает latency (сек) или бросает."""
    url = "http://%s:%d/ner" % (host, port)
    body = json.dumps({"text": text}).encode("utf-8")
    req = urllib.request.Request(url, data=body, headers={"Content-Type": "application/json"})
    start = time.perf_counter()
    with urllib.request.urlopen(req, timeout=60) as resp:
        resp.read()
    return time.perf_counter() - start


def run_http_load(host, port, concurrency, total_requests, text_factory):
    """Запускает total_requests запросов с заданной concurrency."""
    latencies = []
    lock = threading.Lock()
    counter = {"done": 0}

    def worker(_):
        while True:
            with lock:
                if counter["done"] >= total_requests:
                    return
                counter["done"] += 1
                idx = counter["done"]
            text = text_factory(idx)
            try:
                lat = http_ner(host, port, text)
                with lock:
                    latencies.append(lat)
            except Exception:
                with lock:
                    nonlocal_errors[0] += 1

    nonlocal_errors = [0]
    start = time.perf_counter()
    with concurrent.futures.ThreadPoolExecutor(max_workers=concurrency) as ex:
        list(ex.map(worker, range(concurrency)))
    elapsed = time.perf_counter() - start

    completed = len(latencies)
    rps = completed / elapsed if elapsed > 0 else 0
    sorted_lat = sorted(latencies)
    return {
        "concurrency": concurrency,
        "completed": completed,
        "errors": nonlocal_errors[0],
        "rps": rps,
        "p50_ms": percentile(sorted_lat, 50) * 1000,
        "p95_ms": percentile(sorted_lat, 95) * 1000,
        "p99_ms": percentile(sorted_lat, 99) * 1000,
        "elapsed_s": elapsed,
    }


def run_batch_test(batch_size, total_texts, text_factory):
    """Проверяет batch через Slovnet map (потенциал)."""
    import server

    server.load_models()
    texts = [text_factory(i) for i in range(total_texts)]
    # Прогрев.
    list(server._ner.map(texts[:batch_size]))

    batches = [texts[i:i + batch_size] for i in range(0, len(texts), batch_size)]
    start = time.perf_counter()
    for b in batches:
        list(server._ner.map(b))
    elapsed = time.perf_counter() - start

    rps = total_texts / elapsed if elapsed > 0 else 0
    return {
        "batch_size": batch_size,
        "texts": total_texts,
        "rps": rps,
        "elapsed_s": elapsed,
    }


def text_factory(idx):
    """Генерирует уникальный текст с сущностями."""
    return ("клиент Иван Петров %d, адрес клиента: Москва, ул Ленина %d, "
            "кем выдан: ОУФМС России %d" % (idx, idx, idx))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=8090)
    parser.add_argument("--requests", type=int, default=200)
    parser.add_argument("--concurrency", default="1,4,8,16,32")
    parser.add_argument("--batch", default="4,8,16")
    args = parser.parse_args()

    pid = find_sidecar_pid(args.port)
    if pid is None:
        print("ERROR: sidecar not found on port %d" % args.port)
        sys.exit(1)
    print("sidecar PID: %d" % pid)

    print("\n=== HTTP load test ===")
    print("concurrency | completed | errors | RPS | p50(ms) | p95(ms) | p99(ms) | CPU%% | mem(MB)")
    for conc in [int(x) for x in args.concurrency.split(",")]:
        monitor = ProcessMonitor(pid)
        monitor.start()
        res = run_http_load(args.host, args.port, conc, args.requests, text_factory)
        monitor.stop()
        cpu, mem = monitor.summary()
        print("%11d | %9d | %6d | %5.1f | %7.2f | %7.2f | %7.2f | %4.1f | %7.1f" % (
            res["concurrency"], res["completed"], res["errors"], res["rps"],
            res["p50_ms"], res["p95_ms"], res["p99_ms"], cpu, mem / 1024 / 1024))

    print("\n=== Batch test (Slovnet map, потенциал) ===")
    print("batch_size | texts | RPS")
    for bs in [int(x) for x in args.batch.split(",")]:
        res = run_batch_test(bs, args.requests, text_factory)
        print("%10d | %5d | %5.1f" % (res["batch_size"], res["texts"], res["rps"]))


if __name__ == "__main__":
    main()