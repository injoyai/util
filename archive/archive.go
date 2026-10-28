// Package archive 提供流式 ZIP/tar.gz 解压、ZIP 打包和 gzip 编解码。
package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/injoyai/util/oss"
)

// Limits 限制条目数量（含目录）及实际解压字节总量。0 使用默认值。
type Limits struct {
	Files int
	Bytes int64
}

type extractor struct {
	root      *os.Root
	files     int
	remaining int64
}

// budgetReader 也限制 tar.Reader 内部消费的 PAX、GNU 扩展等元数据。
type budgetReader struct {
	reader    io.Reader
	remaining int64
}

func (r *budgetReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining <= 0 {
		return 0, errors.New("archive: tar stream limit exceeded")
	}
	if int64(len(p)) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.reader.Read(p)
	r.remaining -= int64(n)
	return n, err
}

func newExtractor(dir string, limits Limits) (*extractor, error) {
	if limits.Files == 0 {
		limits.Files = 10000
	}
	if limits.Bytes == 0 {
		limits.Bytes = 1 << 30
	}
	if limits.Files < 0 || limits.Bytes < 0 || limits.Bytes == math.MaxInt64 {
		return nil, errors.New("archive: invalid limits")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	return &extractor{root: r, files: limits.Files, remaining: limits.Bytes}, nil
}

func (e *extractor) extract(name string, dir bool, reader io.Reader) (err error) {
	e.files--
	if e.files < 0 {
		return errors.New("archive: entry limit exceeded")
	}
	if name == "" || strings.ContainsAny(name, "\\:\x00") || path.IsAbs(name) {
		return fmt.Errorf("archive: invalid path %q", name)
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return fmt.Errorf("archive: parent path %q", name)
		}
	}
	name = path.Clean(name)
	if name == "." && dir {
		return nil
	}
	local := filepath.FromSlash(name)
	if !filepath.IsLocal(local) {
		return fmt.Errorf("archive: non-local path %q", name)
	}
	if dir {
		return e.root.MkdirAll(local, 0755)
	}
	if err = e.root.MkdirAll(filepath.Dir(local), 0755); err != nil {
		return err
	}
	f, err := e.root.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			_ = e.root.Remove(local)
		}
	}()
	n, err := io.Copy(f, io.LimitReader(reader, e.remaining+1))
	e.remaining -= n
	if err != nil {
		return err
	}
	if e.remaining < 0 {
		return errors.New("archive: byte limit exceeded")
	}
	return nil
}

// ExtractZIP 拒绝路径穿越、链接、特殊文件及覆盖已有文件。
// os.Root 限制写入范围。失败时删除当前半成品文件，先前成功条目仍保留。
func ExtractZIP(filename, dir string, limits Limits) error {
	r, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer r.Close()
	e, err := newExtractor(dir, limits)
	if err != nil {
		return err
	}
	defer e.root.Close()
	for _, file := range r.File {
		mode := file.Mode()
		if !mode.IsRegular() && !mode.IsDir() {
			return fmt.Errorf("archive: unsupported ZIP entry %q", file.Name)
		}
		if mode.IsDir() {
			if err := e.extract(file.Name, true, nil); err != nil {
				return err
			}
			continue
		}
		if file.UncompressedSize64 > uint64(e.remaining) {
			return errors.New("archive: byte limit exceeded")
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		err = e.extract(file.Name, false, input)
		err = errors.Join(err, input.Close())
		if err != nil {
			return err
		}
	}
	return nil
}

// ExtractTarGZ 与 ExtractZIP 使用相同路径、覆盖和资源限制。
// 整个 tar 流（含内部扩展元数据）还限制为 Bytes + Files*4096 + 1 MiB。
func ExtractTarGZ(filename, dir string, limits Limits) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	e, err := newExtractor(dir, limits)
	if err != nil {
		return err
	}
	defer e.root.Close()
	if e.remaining > math.MaxInt64-(1<<20) || int64(e.files) > (math.MaxInt64-e.remaining-(1<<20))/4096 {
		return errors.New("archive: tar limits too large")
	}
	stream := &budgetReader{reader: gz, remaining: e.remaining + int64(e.files)*4096 + (1 << 20)}
	r := tar.NewReader(stream)
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = e.extract(h.Name, true, nil)
		case tar.TypeReg, tar.TypeRegA:
			if h.Size > e.remaining {
				return errors.New("archive: byte limit exceeded")
			}
			err = e.extract(h.Name, false, r)
		default:
			return fmt.Errorf("archive: unsupported tar entry %q", h.Name)
		}
		if err != nil {
			return err
		}
	}
	// 继续读到 gzip EOF 校验 CRC。tar 块尾填充不计入文件正文，
	// 但仍受整个 stream 的上限限制，防止无限尾随数据。
	_, err = io.Copy(io.Discard, stream)
	return err
}

// CreateZIP 打包单个文件或目录内容（不含根目录名称），拒绝链接及特殊文件。
// 输出不能位于源目录内部，以免将临时文件也打包。
func CreateZIP(source, filename string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	filename, err = filepath.Abs(filename)
	if err != nil {
		return err
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.IsDir() {
		resolvedSource, err := filepath.EvalSymlinks(source)
		if err != nil {
			return err
		}
		resolvedParent, err := resolveMissingPath(filepath.Dir(filename))
		if err != nil {
			return err
		}
		resolvedOutput := filepath.Join(resolvedParent, filepath.Base(filename))
		rel, err := filepath.Rel(resolvedSource, resolvedOutput)
		if err != nil {
			return err
		}
		if filepath.IsLocal(rel) || rel == "." {
			return errors.New("archive: output is inside source directory")
		}
	}
	return oss.Write(filename, 0644, func(output io.Writer) error {
		w := zip.NewWriter(output)
		err := filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() && !info.IsDir() {
				return fmt.Errorf("archive: unsupported source %q", filename)
			}
			if filename == source && info.IsDir() {
				return nil
			}
			h, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			if filename == source {
				h.Name = filepath.Base(source)
			} else {
				rel, err := filepath.Rel(source, filename)
				if err != nil {
					return err
				}
				h.Name = filepath.ToSlash(rel)
			}
			if info.IsDir() {
				h.Name += "/"
			} else {
				h.Method = zip.Deflate
			}
			out, err := w.CreateHeader(h)
			if err != nil || info.IsDir() {
				return err
			}
			f, err := os.Open(filename)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, f)
			return errors.Join(err, f.Close())
		})
		return errors.Join(err, w.Close())
	})
}

// resolveMissingPath 解析已存在祖先的符号链接，保留尚未创建的路径段。
func resolveMissingPath(name string) (string, error) {
	var missing []string
	for {
		resolved, err := filepath.EvalSymlinks(name)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(name)
		if parent == name {
			return "", err
		}
		missing = append(missing, filepath.Base(name))
		name = parent
	}
}

// Gzip 不关闭调用方的 reader/writer，传播压缩流收尾错误。
func Gzip(output io.Writer, input io.Reader) error {
	w := gzip.NewWriter(output)
	_, err := io.Copy(w, input)
	return errors.Join(err, w.Close())
}

// Gunzip maxBytes 必须>0，超限前最多向 output 写入 maxBytes 字节。
func Gunzip(output io.Writer, input io.Reader, maxBytes int64) error {
	if maxBytes <= 0 {
		return errors.New("archive: positive byte limit required")
	}
	r, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	defer r.Close()
	if _, err := io.Copy(output, io.LimitReader(r, maxBytes)); err != nil {
		return err
	}
	var extra [1]byte
	n, err := io.ReadFull(r, extra[:])
	if n != 0 {
		return errors.New("archive: byte limit exceeded")
	}
	if err == io.EOF {
		return nil
	}
	return err
}
