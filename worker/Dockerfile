FROM python:3.14.5-slim

WORKDIR /app

RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .

RUN pip install --upgrade pip && pip install --no-cache-dir -r requirements.txt

COPY src/ ./src/

ENV OMP_NUM_THREADS=2
ENV PYTHONUNBUFFERED=1 

CMD ["python", "src/main.py"]