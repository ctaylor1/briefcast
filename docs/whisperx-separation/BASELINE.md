# Stage A baseline manifest

Observed 2026-10-02. This is a redacted source/runtime inventory, not a recovery package or proof that source HEAD built the running image.

| Item | Observation |
| --- | --- |
| Source before document commit | `62ecec0703d4ad1c2c12a81984831e861cebdf18` |
| Document commit seen before feature branch switch | `bdf3168439a518046fecfe91b7220d1df047f6ef` (`Pre Whisper Separation`); it committed both handoff documents on `main` while Stage A inspection was underway |
| Feature branch base | `bdf3168439a518046fecfe91b7220d1df047f6ef` |
| Source-baseline tag | `baseline/whisperx-split-20261002-source` points to `62ecec0`; `baseline/whisperx-split-20261002` points to the intervening document commit. Neither identifies the image source |
| Running container image ID | `sha256:a7542b94500a5ac7e00271451ef2a51db233bd5e2bec92c615b240302b06d052` |
| Running image size / architecture | 13,271,848,020 bytes / linux/amd64, from `docker image inspect` |
| Runtime package metadata | WhisperX 3.8.6; Torch and torchaudio 2.8.0+cu128; CTranslate2 4.8.1; faster-whisper 1.2.1; pyannote.audio 4.0.7 |
| Current code defaults | `service/whisperx.go` loads `medium.en`, English, 120-second chunks, alignment on; Compose currently forces diarization off, and the GPU override forces CUDA/float16/batch 16/concurrency 1 |
| Isolation risk | `main.go` starts scheduling and unlocks jobs; do not start a second app against the live database during tests |

The package query used `importlib.metadata` inside the running container and printed names and versions only. Image ID came from `docker inspect --format '{{.Image}}' briefcast`; size and architecture came from `docker image inspect` by immutable ID. No environment array, connection URL or secret was captured here. The configuration values in the handoff must be rechecked privately immediately before rollout. The exact source revision used to build the running image remains unverified.

Still required before production-affecting work: verified image archive and checksum; private configuration and complete `docker diff` inventory; PostgreSQL dump and isolated restore; consistent config/media protection; baseline isolation smoke test. No backup or restore is claimed by this manifest.
