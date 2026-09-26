package judge

import (
	"os"
	"path/filepath"
	"runtime"
)

// ModelFile is the judge model: Qwen3.5-4B, Q4_K_M quantisation (Apache-2.0).
const ModelFile = "Qwen_Qwen3.5-4B-Q4_K_M.gguf"

// CacheDir is where `tddt setup` puts the libraries and the model.
func CacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "tddt")
}

// DefaultLibDir finds the llama.cpp libraries: $TDDT_LIB, then lib/ next
// to the executable (release archive), then the cache dir.
func DefaultLibDir() string {
	if d := os.Getenv("TDDT_LIB"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		d := filepath.Join(filepath.Dir(exe), "lib")
		if _, err := os.Stat(filepath.Join(d, libName())); err == nil {
			return d
		}
	}
	return filepath.Join(CacheDir(), "lib")
}

// DefaultModel finds the model: $TDDT_MODEL, then the cache dir.
func DefaultModel() string {
	if m := os.Getenv("TDDT_MODEL"); m != "" {
		return m
	}
	return filepath.Join(CacheDir(), "models", ModelFile)
}

func libName() string {
	if runtime.GOOS == "windows" {
		return "llama.dll"
	}
	return "libllama.so"
}
