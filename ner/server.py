#!/usr/bin/env python3
"""NER sidecar на slovnet + navec + razdel.

Загружает модель и embeddings один раз при старте, принимает текст и
возвращает сущности PER/LOC/ORG с байтовыми позициями в исходной строке.

Протокол: HTTP POST /ner
  body: {"text": "..."}
  resp: {"spans": [{"start": int, "end": int, "type": "PER", "text": "..."}]}

start/end — байтовые смещения в UTF-8 исходной строки (start включительно,
end исключительно), чтобы text[start:end] в Go возвращал ровно фрагмент.
"""

import json
import os
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from navec import Navec
from slovnet import NER

# Пути к моделям (относительно этого файла).
BASE_DIR = os.path.dirname(os.path.abspath(__file__))
NAVEC_PATH = os.path.join(BASE_DIR, "models", "navec_news_v1_1B_250K_300d_100q.tar")
NER_PATH = os.path.join(BASE_DIR, "models", "slovnet_ner_news_v1.tar")

# Глобальные объекты модели — загружаются один раз при старте.
_navec = None
_ner = None
_ner_lock = threading.Lock()

MAX_BATCH = 16
MAX_TEXT_BYTES = 16 * 1024
MAX_BODY_BYTES = 2 * 1024 * 1024


def load_models():
    global _navec, _ner
    _navec = Navec.load(NAVEC_PATH)
    _ner = NER.load(NER_PATH)
    _ner.navec(_navec)


def ner_spans(text):
    """Возвращает список сущностей с байтовыми позициями."""
    with _ner_lock:
        markup = _ner(text)
    spans = []
    for span in markup.spans:
        if span.type not in ("PER", "LOC", "ORG"):
            continue
        # span.start/stop — позиции в символах. Переводим в байты UTF-8.
        start = len(text[: span.start].encode("utf-8"))
        end = len(text[: span.stop].encode("utf-8"))
        spans.append({
            "start": start,
            "end": end,
            "type": span.type,
            "text": text[span.start: span.stop],
        })
    return spans


def ner_batch(texts):
    """Обрабатывает массив текстов одним вызовом Slovnet map.

    Порядок результатов совпадает с порядком входов.
    """
    t0 = time.perf_counter()
    with _ner_lock:
        markups = list(_ner.map(texts))
    map_ms = (time.perf_counter() - t0) * 1000
    results = []
    for text, markup in zip(texts, markups):
        spans = []
        for span in markup.spans:
            if span.type not in ("PER", "LOC", "ORG"):
                continue
            start = len(text[: span.start].encode("utf-8"))
            end = len(text[: span.stop].encode("utf-8"))
            spans.append({
                "start": start,
                "end": end,
                "type": span.type,
                "text": text[span.start: span.stop],
            })
        results.append({"spans": spans})
    return results, map_ms


class Handler(BaseHTTPRequestHandler):
    # HTTP/1.1: соединение по умолчанию переиспользуется (keep-alive), а не
    # закрывается после каждого ответа, как в HTTP/1.0.
    protocol_version = "HTTP/1.1"

    def do_POST(self):
        try:
            length = int(self.headers.get("Content-Length", 0))
        except (TypeError, ValueError):
            self._json(400, {"error": "invalid content length"})
            return
        if length <= 0 or length > MAX_BODY_BYTES:
            self._json(413, {"error": "request body is empty or too large"})
            return
        body = self.rfile.read(length)
        try:
            req = json.loads(body)
        except (json.JSONDecodeError, AttributeError):
            self._json(400, {"error": "invalid json"})
            return

        if not isinstance(req, dict):
            self._json(400, {"error": "request body must be a JSON object"})
            return

        if self.path == "/ner":
            text = req.get("text", "")
            if not isinstance(text, str) or len(text.encode("utf-8")) > MAX_TEXT_BYTES:
                self._json(400, {"error": "text must be a string within the size limit"})
                return
            try:
                spans = ner_spans(text)
            except Exception as exc:  # noqa: BLE001
                self._json(500, {"error": str(exc)})
                return
            self._json(200, {"spans": spans})
            return

        if self.path == "/ner/batch":
            texts = req.get("texts")
            if not isinstance(texts, list):
                self._json(400, {"error": "texts must be a list"})
                return
            if not texts or len(texts) > MAX_BATCH:
                self._json(400, {"error": "batch size must be between 1 and %d" % MAX_BATCH})
                return
            if any(not isinstance(text, str) or len(text.encode("utf-8")) > MAX_TEXT_BYTES for text in texts):
                self._json(400, {"error": "every text must be a string within the size limit"})
                return
            t_start = time.perf_counter()
            try:
                results, map_ms = ner_batch(texts)
            except Exception as exc:  # noqa: BLE001
                self._json(500, {"error": str(exc)})
                return
            t_after_map = time.perf_counter()
            ser_ms = (time.perf_counter() - t_after_map) * 1000
            total_ms = (time.perf_counter() - t_start) * 1000
            payload = {
                "results": results,
                "timing": {
                    "map_ms": round(map_ms, 3),
                    "ser_ms": round(ser_ms, 3),
                    "total_ms": round(total_ms, 3),
                },
            }
            data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            sys.stderr.write(
                "BATCH n=%d map=%.2fms ser=%.2fms total=%.2fms\n"
                % (len(texts), map_ms, ser_ms, total_ms)
            )
            sys.stderr.flush()
            self._json_bytes(200, data)
            return

        self.send_error(404)

    def do_GET(self):
        if self.path == "/healthz":
            self._json(200, {"status": "ok"})
            return
        self.send_error(404)

    def _json(self, code, payload):
        data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        self._json_bytes(code, data)

    def _json_bytes(self, code, data):
        self.send_response(code)
        self.send_header("Content-Type", "application/json; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Connection", "keep-alive")
        self.end_headers()
        self.wfile.write(data)

    def log_message(self, fmt, *args):  # noqa: A003
        sys.stderr.write("%s\n" % (fmt % args))


def main():
    host = os.environ.get("NER_HOST", "127.0.0.1")
    port = int(os.environ.get("NER_PORT", "8090"))
    load_models()
    server = ThreadingHTTPServer((host, port), Handler)
    sys.stderr.write("NER sidecar listening on %s:%d\n" % (host, port))
    sys.stderr.flush()
    server.serve_forever()


if __name__ == "__main__":
    main()
