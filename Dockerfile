ARG GO_VERSION=1.26.0
ARG NODE_VERSION=24.12.0
ARG TORCH_INDEX_URL=https://download.pytorch.org/whl/cpu

FROM node:${NODE_VERSION}-alpine AS frontend-builder
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM golang:${GO_VERSION} AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /api
WORKDIR /api

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build -o ./app ./main.go

FROM python:3.12-slim
ARG TORCH_INDEX_URL=https://download.pytorch.org/whl/cpu
ARG TORCH_VERSION=2.8.0
ARG TORCH_BUILD=+cpu

LABEL org.opencontainers.image.source="https://github.com/ctaylor1/briefcast"

ENV CONFIG=/config
ENV DATA=/assets
ENV UID=998
ENV PID=100
ENV GIN_MODE=release
VOLUME ["/config", "/assets"]
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*
RUN python -m venv /opt/venv && \
    /opt/venv/bin/pip install --no-cache-dir --upgrade pip && \
    /opt/venv/bin/pip install --no-cache-dir feedparser mutagen && \
    echo "torch==${TORCH_VERSION}${TORCH_BUILD}" > /tmp/torch-constraints.txt && \
    echo "torchaudio==${TORCH_VERSION}${TORCH_BUILD}" >> /tmp/torch-constraints.txt && \
    /opt/venv/bin/pip install --no-cache-dir --index-url ${TORCH_INDEX_URL} -c /tmp/torch-constraints.txt torch torchaudio && \
    /opt/venv/bin/pip install --no-cache-dir --index-url ${TORCH_INDEX_URL} --extra-index-url https://pypi.org/simple -c /tmp/torch-constraints.txt whisperx
RUN mkdir -p /config; \
    mkdir -p /assets; \
    mkdir -p /api

RUN chmod 777 /config; \
    chmod 777 /assets

WORKDIR /api
ENV PATH="/opt/venv/bin:${PATH}"
COPY --from=builder /api/app .
COPY webassets ./webassets
COPY scripts ./scripts
COPY --from=frontend-builder /frontend/dist ./frontend/dist

EXPOSE 8080

ENTRYPOINT ["./app"]

