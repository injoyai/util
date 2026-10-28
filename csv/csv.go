// Package csv 提供 UTF-8 CSV 流式读写，并识别 UTF-8 BOM。
package csv

import (
	"bufio"
	"bytes"
	stdcsv "encoding/csv"
	"errors"
	"io"
)

const BOM = "\xef\xbb\xbf"

// Range 按行回调；返回 false 提前停止。允许各行列数不同。
// UTF-16 必须先转为 UTF-8，不会只丢弃 BOM 后假装完成编码转换。
func Range(input io.Reader, fn func(int, []string) bool) error {
	if fn == nil {
		return errors.New("csv: nil callback")
	}
	r := bufio.NewReader(input)
	prefix, err := r.Peek(3)
	if err != nil && err != io.EOF {
		return err
	}
	if bytes.HasPrefix(prefix, []byte{0xff, 0xfe}) || bytes.HasPrefix(prefix, []byte{0xfe, 0xff}) {
		return errors.New("csv: UTF-16 input must be converted to UTF-8")
	}
	if bytes.HasPrefix(prefix, []byte(BOM)) {
		_, _ = r.Discard(3)
	}
	c := stdcsv.NewReader(r)
	c.FieldsPerRecord = -1
	for i := 0; ; i++ {
		row, err := c.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !fn(i, row) {
			return nil
		}
	}
}

func Read(input io.Reader) ([][]string, error) {
	var rows [][]string
	err := Range(input, func(_ int, row []string) bool { rows = append(rows, row); return true })
	return rows, err
}

// Write 可选输出 BOM，并检查缓冲区 Flush 的错误。不关闭 output。
func Write(output io.Writer, rows [][]string, bom bool) error {
	if bom {
		n, err := io.WriteString(output, BOM)
		if err != nil {
			return err
		}
		if n != len(BOM) {
			return io.ErrShortWrite
		}
	}
	w := stdcsv.NewWriter(output)
	for _, row := range rows {
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
