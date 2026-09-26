package setup

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func serve(t *testing.T, name string, body []byte) (*httptest.Server, *int) {
	t.Helper()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &requests
}

func TestDownloadChecksSHA(t *testing.T) {
	body := bytes.Repeat([]byte("model"), 100000)
	srv, _ := serve(t, "m.gguf", body)
	dest := filepath.Join(t.TempDir(), "models", "m.gguf")
	var log bytes.Buffer
	if err := Download(context.Background(), Asset{URL: srv.URL + "/m.gguf", SHA256: sum(body), Size: int64(len(body))}, dest, &log); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Fatal("content differs")
	}
	if !strings.Contains(log.String(), "downloaded") {
		t.Fatalf("no progress: %s", log.String())
	}
	if _, err := os.Stat(dest + ".part"); !os.IsNotExist(err) {
		t.Fatal(".part left behind")
	}
}

func TestDownloadResumes(t *testing.T) {
	body := bytes.Repeat([]byte("0123456789"), 50000)
	srv, _ := serve(t, "m.gguf", body)
	dest := filepath.Join(t.TempDir(), "m.gguf")
	os.WriteFile(dest+".part", body[:123456], 0o644)
	if err := Download(context.Background(), Asset{URL: srv.URL + "/m.gguf", SHA256: sum(body)}, dest, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dest)
	if !bytes.Equal(got, body) {
		t.Fatal("resumed content differs")
	}
}

func TestDownloadRejectsWrongSHA(t *testing.T) {
	srv, _ := serve(t, "m.gguf", []byte("evil"))
	dest := filepath.Join(t.TempDir(), "m.gguf")
	err := Download(context.Background(), Asset{URL: srv.URL + "/m.gguf", SHA256: sum([]byte("good"))}, dest, nil)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("want SHA error, got %v", err)
	}
	for _, p := range []string{dest, dest + ".part"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatalf("%s kept after a bad download", p)
		}
	}
}

func TestDownloadHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	if err := Download(context.Background(), Asset{URL: srv.URL + "/x"}, filepath.Join(t.TempDir(), "x"), nil); err == nil {
		t.Fatal("want error")
	}
}

func tarGz(t *testing.T) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	add := func(h *tar.Header, body string) {
		h.Size = int64(len(body))
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		tw.Write([]byte(body))
	}
	add(&tar.Header{Name: "llama-b1/", Typeflag: tar.TypeDir, Mode: 0o755}, "")
	add(&tar.Header{Name: "llama-b1/libllama.so.0.5.0", Typeflag: tar.TypeReg, Mode: 0o755}, "ELF")
	add(&tar.Header{Name: "llama-b1/libllama.so", Typeflag: tar.TypeSymlink, Linkname: "libllama.so.0.5.0"}, "")
	add(&tar.Header{Name: "llama-b1/llama-server", Typeflag: tar.TypeReg, Mode: 0o755}, "tool")
	add(&tar.Header{Name: "llama-b1/libllama-server-impl.so", Typeflag: tar.TypeReg, Mode: 0o755}, "tool lib")
	add(&tar.Header{Name: "llama-b1/LICENSE", Typeflag: tar.TypeReg, Mode: 0o644}, "MIT")
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func TestInstallLibsTarGz(t *testing.T) {
	archive := tarGz(t)
	srv, _ := serve(t, "l.tar.gz", archive)
	old := Libs["linux"]
	Libs["linux"] = Asset{URL: srv.URL + "/l.tar.gz", SHA256: sum(archive)}
	defer func() { Libs["linux"] = old }()
	dir := t.TempDir()
	if err := InstallLibs(context.Background(), "linux", dir, nil); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "libllama.so")); err != nil || string(data) != "ELF" {
		t.Fatalf("symlinked lib: %q %v", data, err)
	}
	for _, tool := range []string{"llama-server", "libllama-server-impl.so"} {
		if _, err := os.Stat(filepath.Join(dir, tool)); !os.IsNotExist(err) {
			t.Fatalf("%s should be skipped", tool)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "LICENSE")); err != nil {
		t.Fatal("licence missing")
	}
}

func TestInstallLibsZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"llama.dll", "ggml-vulkan.dll", "llama-cli.exe", "sub/ggml.dll", "mtmd.dll"} {
		w, _ := zw.Create(n)
		w.Write([]byte(n))
	}
	zw.Close()
	archive := buf.Bytes()
	srv, _ := serve(t, "l.zip", archive)
	old := Libs["windows"]
	Libs["windows"] = Asset{URL: srv.URL + "/l.zip", SHA256: sum(archive)}
	defer func() { Libs["windows"] = old }()
	dir := t.TempDir()
	if err := InstallLibs(context.Background(), "windows", dir, nil); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if strings.Join(names, ",") != "ggml-vulkan.dll,ggml.dll,llama.dll" {
		t.Fatalf("got %v", names)
	}
}
