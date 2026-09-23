#!/usr/bin/env python3
"""Измеряет CPU и память процесса, пока существует файл-маркер.

Используется Go load test'ом для замера ресурсов sidecar во время нагрузки.

Запуск:
  python ner/measure.py <pid> <marker_file>
"""

import os
import statistics
import sys
import time

import psutil


def main():
    pid = int(sys.argv[1])
    marker = sys.argv[2]
    proc = psutil.Process(pid)
    cpu_samples = []
    mem_samples = []
    while os.path.exists(marker):
        try:
            cpu_samples.append(proc.cpu_percent(interval=0.2))
            mem_samples.append(proc.memory_info().rss)
        except psutil.NoSuchProcess:
            break
    cpu = statistics.mean(cpu_samples) if cpu_samples else 0.0
    mem = max(mem_samples) if mem_samples else 0
    print("%.1f %d" % (cpu, mem))


if __name__ == "__main__":
    main()