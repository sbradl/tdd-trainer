// Package setup downloads what the judge needs: the llama.cpp libraries
// and the model, both pinned and checked with SHA-256.
package setup

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Asset is a pinned download.
type Asset struct {
	URL    string
	SHA256 string
	Size   int64 // 0 if unknown
}

// Model is the judge model: Qwen3.5-4B Q4_K_M by bartowski (Apache-2.0).
var Model = Asset{
	URL:    "https://huggingface.co/bartowski/Qwen_Qwen3.5-4B-GGUF/resolve/4168f45a16a1290d65a4ec0fa312ae917a4c15d6/Qwen_Qwen3.5-4B-Q4_K_M.gguf",
	SHA256: "13c16f426047e2de38cd075bdade4a7bcbc8c774384876f677740cda65f8a983",
	Size:   3013027808,
}

// Libs are the llama.cpp b11146 builds that yzma v1.28.0 is tested with.
// The Vulkan builds contain the CPU backends too, so one download serves
// machines with and without a GPU.
var Libs = map[string]Asset{
	"linux": {
		URL:    "https://github.com/ggml-org/llama.cpp/releases/download/b11146/llama-b11146-bin-ubuntu-vulkan-x64.tar.gz",
		SHA256: "d3ce40fce7403cc93bcf5718fc46c6efb61ed9709f8e5d9f10c86bf0e30e8fb3",
	},
	"windows": {
		URL:    "https://github.com/ggml-org/llama.cpp/releases/download/b11146/llama-b11146-bin-win-vulkan-x64.zip",
		SHA256: "55a378aa095b466979d85075234f66d7655c7a7483222af0c006c0e55b4d7bd6",
	},
}

// Download fetches a to dest, resuming a partial download, and checks its
// SHA-256. progress receives a line now and then (nil: silent).
func Download(ctx context.Context, a Asset, dest string, progress io.Writer) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	part := dest + ".part"
	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	have, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return err
	}
	if have > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusPartialContent:
	case resp.StatusCode == http.StatusOK:
		// no resume support (or nothing to resume): start over
		if err := f.Truncate(0); err != nil {
			return err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		have = 0
	case resp.StatusCode == http.StatusRequestedRangeNotSatisfiable && a.Size > 0 && have == a.Size:
		// already complete
	default:
		return fmt.Errorf("downloading %s: %s", a.URL, resp.Status)
	}

	total := a.Size
	if total == 0 && resp.ContentLength > 0 {
		total = have + resp.ContentLength
	}
	w := &progressWriter{out: progress, done: have, total: total, name: path.Base(a.URL), last: time.Now()}
	if resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		if _, err := io.Copy(io.MultiWriter(f, w), resp.Body); err != nil {
			return fmt.Errorf("downloading %s: %w (run setup again to resume)", a.URL, err)
		}
	}
	w.finish()
	if err := f.Close(); err != nil {
		return err
	}
	if err := Verify(part, a.SHA256, progress); err != nil {
		os.Remove(part)
		return err
	}
	return os.Rename(part, dest)
}

// Verify checks a file's SHA-256.
func Verify(file, want string, progress io.Writer) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	if progress != nil {
		fmt.Fprintf(progress, "checking %s…\n", filepath.Base(file))
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s: SHA-256 is %s, want %s", filepath.Base(file), got, want)
	}
	return nil
}

type progressWriter struct {
	out         io.Writer
	done, total int64
	name        string
	last        time.Time
	sinceLast   int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	p.sinceLast += int64(len(b))
	if p.out != nil && time.Since(p.last) >= time.Second {
		rate := float64(p.sinceLast) / time.Since(p.last).Seconds() / 1e6
		if p.total > 0 {
			fmt.Fprintf(p.out, "%s: %5.1f%% of %.0f MB, %.1f MB/s\n", p.name, 100*float64(p.done)/float64(p.total), float64(p.total)/1e6, rate)
		} else {
			fmt.Fprintf(p.out, "%s: %.0f MB, %.1f MB/s\n", p.name, float64(p.done)/1e6, rate)
		}
		p.last, p.sinceLast = time.Now(), 0
	}
	return len(b), nil
}

func (p *progressWriter) finish() {
	if p.out != nil {
		fmt.Fprintf(p.out, "%s: %.0f MB downloaded\n", p.name, float64(p.done)/1e6)
	}
}

// InstallLibs downloads the llama.cpp libraries for goos into dir.
func InstallLibs(ctx context.Context, goos, dir string, progress io.Writer) error {
	a, ok := Libs[goos]
	if !ok {
		return fmt.Errorf("no llama.cpp build for %s", goos)
	}
	tmp, err := os.MkdirTemp("", "tddt-libs-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	archive := filepath.Join(tmp, path.Base(a.URL))
	if err := Download(ctx, a, archive, progress); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(archive, dir)
	}
	return extractTarGz(archive, dir)
}

// wanted keeps shared libraries and licence files; the llama.cpp tools
// and the libraries only they use are not needed.
func wanted(name string) bool {
	base := strings.ToLower(path.Base(name))
	for _, skip := range []string{"-impl.", "llama-common", "mtmd", "ggml-rpc"} {
		if strings.Contains(base, skip) {
			return false
		}
	}
	return strings.HasSuffix(base, ".dll") || strings.Contains(base, ".so") || strings.HasPrefix(base, "license")
}

func extractTarGz(archive, dir string) error {
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !wanted(h.Name) {
			continue
		}
		// archives nest everything in one folder; flatten it
		dest := filepath.Join(dir, path.Base(h.Name))
		switch h.Typeflag {
		case tar.TypeReg:
			if err := writeFile(dest, tr, h.FileInfo().Mode().Perm()|0o644); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if strings.Contains(h.Linkname, "/") || strings.Contains(h.Linkname, `\`) {
				return fmt.Errorf("unexpected link %s -> %s", h.Name, h.Linkname)
			}
			os.Remove(dest)
			if err := os.Symlink(h.Linkname, dest); err != nil {
				return err
			}
		}
	}
}

func extractZip(archive, dir string) error {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || !wanted(zf.Name) {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		err = writeFile(filepath.Join(dir, path.Base(zf.Name)), rc, 0o644)
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func writeFile(dest string, r io.Reader, mode os.FileMode) error {
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		return err
	}
	os.Remove(dest) // may be a symlink from an earlier install
	return os.WriteFile(dest, buf.Bytes(), mode)
}
