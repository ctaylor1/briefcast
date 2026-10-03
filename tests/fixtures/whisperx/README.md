# Synthetic speech fixture

`speech.flac` contains locally synthesized English speech (Windows SAPI), created for this repository's isolated transcription tests. It contains no production podcast audio or personal data. Text: “This is an isolated transcription test. Briefcast keeps durable worker results until the application confirms receipt. The quick brown fox jumps over the lazy dog.”

The fixture exercises the actual ASR/VAD/alignment engine in bounded release tests. It does not satisfy the representative podcast/two-speaker quality gate. Stage G's local long test repeats the same speech to exceed 240 seconds; that also remains synthetic.
