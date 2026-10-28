package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func makeZIP(t *testing.T, name string, mode os.FileMode, body string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	h := &zip.FileHeader{Name: name, Method: zip.Deflate}
	h.SetMode(mode)
	out, err := w.CreateHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(out, body); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestZIPRoundTrip(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "中文.txt"), []byte("你好"), 0600); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "out.zip")
	if err := CreateZIP(source, filename); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	if err := ExtractZIP(filename, dest, Limits{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dest, "中文.txt"))
	if err != nil || string(b) != "你好" {
		t.Fatal(string(b), err)
	}
	if _, err := os.Stat(filepath.Join(dest, "empty")); err != nil {
		t.Fatal(err)
	}
	if err := CreateZIP(source, filepath.Join(source, "inside.zip")); err == nil {
		t.Fatal("self inclusion accepted")
	}
	if err := ExtractZIP(filename, dest, Limits{}); err == nil {
		t.Fatal("overwrite accepted")
	}
}

func TestZIPOutputViaSymlink(t *testing.T) {
	source := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(source, alias); err != nil {
		t.Fatal(err)
	}
	if err := CreateZIP(source, filepath.Join(alias, "empty.zip")); err == nil {
		t.Fatal("output inside source via symlink accepted")
	}
	if err := CreateZIP(source, filepath.Join(alias, "missing", "empty.zip")); err == nil {
		t.Fatal("output via symlink and missing parent accepted")
	}
	// 输出文件本身是链接时，临时文件仍位于其父目录。
	output := filepath.Join(source, "out.zip")
	if err := os.Symlink(filepath.Join(t.TempDir(), "external.zip"), output); err != nil {
		t.Fatal(err)
	}
	if err := CreateZIP(source, output); err == nil {
		t.Fatal("output file symlink bypassed parent check")
	}
}

func TestZIPUnsafeEntriesAndLimits(t *testing.T) {
	for _, name := range []string{"../escape", "a/../../escape", "/absolute", "C:/windows", "a\\escape", "file:stream"} {
		t.Run(name, func(t *testing.T) {
			filename := makeZIP(t, name, 0644, "data")
			if err := ExtractZIP(filename, t.TempDir(), Limits{}); err == nil {
				t.Fatal("unsafe path accepted")
			}
		})
	}
	if err := ExtractZIP(makeZIP(t, "link", os.ModeSymlink|0777, "../target"), t.TempDir(), Limits{}); err == nil {
		t.Fatal("symlink accepted")
	}
	dir := t.TempDir()
	filename := makeZIP(t, "file", 0644, "123456")
	if err := ExtractZIP(filename, dir, Limits{Bytes: 5}); err == nil {
		t.Fatal("size limit")
	}
	if _, err := os.Stat(filepath.Join(dir, "file")); !os.IsNotExist(err) {
		t.Fatal("partial file left", err)
	}
	// 现有目录符号链接也不能将写入引到目标根之外。
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZIP(makeZIP(t, "outside/escape", 0644, "bad"), dir, Limits{}); err == nil {
		t.Fatal("existing symlink escape accepted")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
		t.Fatal("wrote outside extraction root", err)
	}
	if err := ExtractZIP(makeZIP(t, "dir/", os.ModeDir|0755, ""), t.TempDir(), Limits{Files: -1}); err == nil {
		t.Fatal("negative limit")
	}
}

func makeTarGZ(t *testing.T, headers []*tar.Header, bodies []string) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "test.tar.gz")
	f, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(f)
	w := tar.NewWriter(gz)
	for i, h := range headers {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, bodies[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestTarGZ(t *testing.T) {
	filename := makeTarGZ(t, []*tar.Header{{Name: "dir/", Typeflag: tar.TypeDir, Mode: 0755}, {Name: "dir/file", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, []string{"", "abc"})
	dir := t.TempDir()
	if err := ExtractTarGZ(filename, dir, Limits{}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "dir", "file"))
	if err != nil || string(b) != "abc" {
		t.Fatal(string(b), err)
	}
	if err := ExtractTarGZ(filename, t.TempDir(), Limits{Files: 1}); err == nil {
		t.Fatal("file count limit")
	}
	if err := ExtractTarGZ(filename, t.TempDir(), Limits{Bytes: 2}); err == nil {
		t.Fatal("byte limit")
	}
	for _, h := range []*tar.Header{{Name: "../escape", Typeflag: tar.TypeReg}, {Name: "link", Typeflag: tar.TypeSymlink, Linkname: "../escape"}, {Name: "hard", Typeflag: tar.TypeLink, Linkname: "../escape"}} {
		if err := ExtractTarGZ(makeTarGZ(t, []*tar.Header{h}, []string{""}), t.TempDir(), Limits{}); err == nil {
			t.Fatal("unsafe entry", h.Name)
		}
	}
	// tar EOF 之后仍须校验 gzip 校验和。
	b, err = os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)-8] ^= 0xff
	if err := os.WriteFile(filename, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExtractTarGZ(filename, t.TempDir(), Limits{}); err == nil {
		t.Fatal("bad CRC accepted")
	}
}

func TestTarPaddingDoesNotCountAsFileBytes(t *testing.T) {
	filename := makeTarGZ(t, []*tar.Header{{Name: "file", Typeflag: tar.TypeReg, Mode: 0644, Size: 3}}, []string{"abc"})
	compressed, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	r, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	// GNU tar 等工具会把结尾补齐到一个完整数据块。
	plain = append(plain, make([]byte, 4096)...)
	var padded bytes.Buffer
	if err := Gzip(&padded, bytes.NewReader(plain)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, padded.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExtractTarGZ(filename, t.TempDir(), Limits{Bytes: 3}); err != nil {
		t.Fatal(err)
	}
}

func TestGzipStreaming(t *testing.T) {
	var compressed bytes.Buffer
	input := bytes.Repeat([]byte("中文"), 100)
	if err := Gzip(&compressed, bytes.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := Gunzip(&output, bytes.NewReader(compressed.Bytes()), int64(len(input))); err != nil || !bytes.Equal(input, output.Bytes()) {
		t.Fatal(err)
	}
	output.Reset()
	if err := Gunzip(&output, bytes.NewReader(compressed.Bytes()), 10); err == nil || output.Len() != 10 {
		t.Fatal(output.Len(), err)
	}
}
