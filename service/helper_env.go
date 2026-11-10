package service

import (
	"os"
	"strings"
)

// helperEnvKeys are the parent variables a Python helper may inherit: what the
// interpreter, CUDA, model caches and our logging read. Everything else,
// including DATABASE_URL, PASSWORD and API keys, stays in this process.
var helperEnvKeys = map[string]bool{
	"PATH": true, "HOME": true, "USER": true, "LANG": true, "LANGUAGE": true,
	"LC_ALL": true, "LC_CTYPE": true, "TZ": true, "TMPDIR": true, "TEMP": true, "TMP": true,
	"SYSTEMROOT": true, "WINDIR": true,
	"PYTHONPATH": true, "PYTHONHOME": true, "PYTHONIOENCODING": true, "PYTHONUTF8": true, "VIRTUAL_ENV": true,
	"LD_LIBRARY_PATH": true, "CUDA_VISIBLE_DEVICES": true, "NVIDIA_VISIBLE_DEVICES": true, "NVIDIA_DRIVER_CAPABILITIES": true,
	"XDG_CACHE_HOME": true, "HF_HOME": true, "HUGGINGFACE_HUB_CACHE": true, "TORCH_HOME": true, "NLTK_DATA": true,
	"HF_HUB_OFFLINE": true, "TRANSFORMERS_OFFLINE": true, "HF_DATASETS_OFFLINE": true,
	"WHISPERX_THIRD_PARTY_LOG_LEVEL": true,
}

// helperEnvLogPrefix covers the logging settings the shared Python logging
// module reads (LOG_LEVEL, LOG_FORMAT, LOG_RUN_TIMESTAMP and friends).
const helperEnvLogPrefix = "LOG_"

// helperBaseEnv returns the allow-listed part of this process's environment.
// LOG_OUTPUT is left out; callers set a sanitized value.
func helperBaseEnv() []string {
	base := os.Environ()
	env := make([]string, 0, len(helperEnvKeys))
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		upper := strings.ToUpper(key)
		if upper == logOutputEnv {
			continue
		}
		if helperEnvKeys[upper] || strings.HasPrefix(upper, helperEnvLogPrefix) {
			env = append(env, entry)
		}
	}
	return env
}
