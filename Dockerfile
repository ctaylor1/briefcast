FROM node:24.12.0-alpine@sha256:c921b97d4b74f51744057454b306b418cf693865e73b8100559189605f6955b8 AS frontend-builder
WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM golang:1.26.0@sha256:fb612b7831d53a89cbc0aaa7855b69ad7b0caf603715860cf538df854d047b84 AS builder

WORKDIR /api

COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .
ARG APP_VERSION=dev
RUN CGO_ENABLED=1 go build -trimpath -ldflags "-X main.appVersion=${APP_VERSION}" -o ./app ./main.go

FROM python:3.12.13-slim-trixie@sha256:229a2c5bfa27522db7815ea81f9bed70af17ccb9de9fc7ad142b1877b5830d36

ENV CONFIG=/config
ENV DATA=/assets
ENV GIN_MODE=release
VOLUME ["/config", "/assets"]
ENV PYTHONDONTWRITEBYTECODE=1
COPY docker/app/requirements.lock /opt/briefcast/requirements.lock
COPY docker/build-requirements.lock /opt/briefcast/build-requirements.lock
RUN python -m venv /opt/venv && \
    /opt/venv/bin/pip install --no-cache-dir --require-hashes --no-deps -r /opt/briefcast/build-requirements.lock && \
    /opt/venv/bin/pip install --no-cache-dir --no-build-isolation --require-hashes --no-deps -r /opt/briefcast/requirements.lock && \
    /opt/venv/bin/pip uninstall -y setuptools wheel && \
    /opt/venv/bin/pip check
RUN mkdir -p /config; \
    mkdir -p /assets; \
    mkdir -p /api

RUN chmod 777 /config; \
    chmod 777 /assets

WORKDIR /api
ENV PATH="/opt/venv/bin:${PATH}"
COPY --from=builder /api/app .
COPY webassets ./webassets
COPY scripts/feedparser_parse.py scripts/mutagen_id3_extract.py ./scripts/
COPY src/briefcast_tools ./src/briefcast_tools
COPY --from=frontend-builder /frontend/dist ./frontend/dist
COPY docker/runtime_manifest.py /opt/briefcast/runtime_manifest.py
ARG APP_VERSION=dev
ARG SOURCE_REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/ctaylor1/briefcast" \
      org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.version="${APP_VERSION}"
ENV BRIEFCAST_VERSION=${APP_VERSION}
RUN python /opt/briefcast/runtime_manifest.py app "${APP_VERSION}" "${SOURCE_REVISION}" > /opt/briefcast/runtime.json

EXPOSE 8080

ENTRYPOINT ["./app"]

