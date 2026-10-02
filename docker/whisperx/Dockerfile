FROM golang:1.26.0@sha256:fb612b7831d53a89cbc0aaa7855b69ad7b0caf603715860cf538df854d047b84 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/whisperx-worker ./cmd/whisperx-worker
COPY internal/transcription ./internal/transcription
COPY internal/whisperxworker ./internal/whisperxworker
ARG WORKER_VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-X main.workerVersion=${WORKER_VERSION}" -o /whisperx-worker ./cmd/whisperx-worker

FROM python:3.12.13-slim-trixie@sha256:229a2c5bfa27522db7815ea81f9bed70af17ccb9de9fc7ad142b1877b5830d36
ARG TARGETARCH
ARG RUNTIME=cpu
ENV PATH="/opt/venv/bin:${PATH}" PYTHONDONTWRITEBYTECODE=1 \
    HOME=/cache/home HF_HOME=/cache/huggingface TORCH_HOME=/cache/torch NLTK_DATA=/cache/nltk \
    HF_HUB_DISABLE_TELEMETRY=1 \
    LD_LIBRARY_PATH="/opt/venv/lib/python3.12/site-packages/nvidia/cublas/lib:/opt/venv/lib/python3.12/site-packages/nvidia/cudnn/lib"
# Fail rather than silently upgrade an unavailable system version.
COPY docker/whisperx/system-packages.lock /opt/briefcast/system-packages.lock
RUN test "${TARGETARCH}" = amd64 && \
    apt-get update && xargs -a /opt/briefcast/system-packages.lock \
        apt-get install -y --no-install-recommends -- && \
    rm -rf /var/lib/apt/lists/*
COPY docker/whisperx/requirements-*.lock /opt/briefcast/
COPY docker/build-requirements.lock /opt/briefcast/build-requirements.lock
RUN --mount=type=cache,target=/cache/home/.cache/pip,sharing=locked \
    case "${RUNTIME}" in cpu|cu128) ;; *) exit 1 ;; esac && \
    python -m venv /opt/venv && \
    /opt/venv/bin/pip install --require-hashes --no-deps -r /opt/briefcast/build-requirements.lock && \
    /opt/venv/bin/pip install --no-build-isolation --require-hashes --no-deps \
        --index-url https://pypi.org/simple \
        --extra-index-url "https://download.pytorch.org/whl/${RUNTIME}" \
        -r "/opt/briefcast/requirements-${RUNTIME}.lock" && \
    /opt/venv/bin/pip uninstall -y wheel && /opt/venv/bin/pip check && \
    mkdir -p /state /cache /media /worker && chown 1000:1000 /state /cache
WORKDIR /worker
COPY --from=builder /whisperx-worker /usr/local/bin/whisperx-worker
COPY scripts/whisperx_transcribe.py ./scripts/
COPY src/briefcast_tools ./src/briefcast_tools
COPY docker/runtime_manifest.py /opt/briefcast/runtime_manifest.py
COPY docker/whisperx/compatibility.json /opt/briefcast/compatibility.json
ARG WORKER_VERSION=dev
ARG SOURCE_REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/ctaylor1/briefcast" \
      org.opencontainers.image.revision="${SOURCE_REVISION}" \
      org.opencontainers.image.version="${WORKER_VERSION}" \
      io.briefcast.whisperx.runtime="${RUNTIME}"
RUN python /opt/briefcast/runtime_manifest.py "worker-${RUNTIME}" "${WORKER_VERSION}" "${SOURCE_REVISION}" > /opt/briefcast/runtime.json
USER 1000:1000
EXPOSE 8090
ENTRYPOINT ["whisperx-worker"]
