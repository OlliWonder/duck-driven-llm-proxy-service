FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    OMP_NUM_THREADS=1 \
    OPENBLAS_NUM_THREADS=1 \
    MKL_NUM_THREADS=1 \
    NUMEXPR_NUM_THREADS=1

WORKDIR /app/ner

COPY ner/requirements.txt ./requirements.txt
RUN pip install --no-cache-dir -r requirements.txt

RUN useradd --system --uid 10001 --create-home app

COPY --chown=app:app ner/server.py ./server.py
COPY --chown=app:app ner/models ./models

USER app

EXPOSE 8090

CMD ["python", "server.py"]
