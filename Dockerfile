ARG GO_VERSION=1.26.0
ARG NODE_VERSION=24.12.0

FROM node:${NODE_VERSION}-alpine AS frontend-builder
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM golang:${GO_VERSION}-alpine AS builder

RUN apk update && apk add alpine-sdk git && rm -rf /var/cache/apk/*

RUN mkdir -p /api
WORKDIR /api

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
RUN go build -o ./app ./main.go

FROM alpine:latest

LABEL org.opencontainers.image.source="https://github.com/ctaylor1/briefcast"

ENV CONFIG=/config
ENV DATA=/assets
ENV UID=998
ENV PID=100
ENV GIN_MODE=release
VOLUME ["/config", "/assets"]
RUN apk update && apk add ca-certificates python3 py3-pip && \
    python3 -m venv /opt/venv && \
    /opt/venv/bin/pip install --no-cache-dir --upgrade pip && \
    /opt/venv/bin/pip install --no-cache-dir feedparser mutagen && \
    rm -rf /var/cache/apk/*
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

