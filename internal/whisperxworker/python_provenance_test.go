package whisperxworker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

func TestPipelineFingerprintIncludesProvenanceHelper(t *testing.T) {
	dir := t.TempDir()
	script := []byte("# custom pipeline\n")
	path := filepath.Join(dir, "pipeline.py")
	if err := os.WriteFile(path, script, 0600); err != nil {
		t.Fatal(err)
	}
	runner := &PythonRunner{cfg: PythonConfig{Script: path}}
	standalone, err := runner.PipelineSHA256()
	if err != nil || standalone != transcription.Digest(script) {
		t.Fatal("standalone script compatibility changed", err)
	}
	helper := filepath.Join(dir, "podcast_provenance.py")
	if err := os.WriteFile(helper, []byte("version = 1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := runner.PipelineSHA256()
	if err != nil || first == standalone {
		t.Fatal("helper was omitted from pipeline identity", err)
	}
	if err := os.WriteFile(helper, []byte("version = 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := runner.PipelineSHA256()
	if err != nil || second == first {
		t.Fatal("helper changes did not invalidate pipeline identity", err)
	}
}
