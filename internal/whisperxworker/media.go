package whisperxworker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"

	"github.com/ctaylor1/briefcast/internal/transcription"
)

func readLimit(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("artifact exceeds limit")
	}
	return b, nil
}
func verifyMedia(ctx context.Context, root *os.Root, input transcription.MediaInput) error {
	if !transcription.ValidMediaPath(input.RelativePath) {
		return transcription.Error("INVALID_CONFIG", "Invalid media reference", false)
	}
	// Stat before opening rejects FIFOs/devices without blocking in Open. The
	// media tree has trusted writers; os.Root also enforces symlink containment.
	info, err := root.Stat(input.RelativePath)
	if err != nil {
		if os.IsNotExist(err) {
			return transcription.Error("INPUT_MISSING", "Media is unavailable in the worker mount", true)
		}
		return transcription.Error("INPUT_UNREADABLE", "Media cannot be read", true)
	}
	if !info.Mode().IsRegular() {
		return transcription.Error("INPUT_UNREADABLE", "Media must be a regular file", false)
	}
	f, err := root.Open(input.RelativePath)
	if err != nil {
		return transcription.Error("INPUT_UNREADABLE", "Media cannot be read", true)
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return transcription.Error("INPUT_UNREADABLE", "Media cannot be read", true)
	}
	if info.Size() != input.SizeBytes {
		return transcription.Error("INPUT_CHANGED", "Media fingerprint changed", false)
	}
	hash := sha256.New()
	buf := make([]byte, 128*1024)
	var count int64
	for {
		if err = ctx.Err(); err != nil {
			return err
		}
		n, readErr := f.Read(buf)
		if n > 0 {
			count += int64(n)
			if count > input.SizeBytes {
				return transcription.Error("INPUT_CHANGED", "Media fingerprint changed", false)
			}
			hash.Write(buf[:n])
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return transcription.Error("INPUT_UNREADABLE", "Media cannot be read", true)
		}
	}
	if count != input.SizeBytes || hex.EncodeToString(hash.Sum(nil)) != input.SHA256 {
		return transcription.Error("INPUT_CHANGED", "Media fingerprint changed", false)
	}
	return nil
}
