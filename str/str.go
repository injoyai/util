// Package str 提供标准库之外的字符串工具。索引及长度按 Unicode 码点计算。
package str

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"unicode/utf8"
)

// Reverse 按码点倒序；组合字符和 emoji 序列可能被拆开。
func Reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}

func HasRepeat(s string) bool {
	seen := make(map[rune]struct{})
	for _, r := range s {
		if _, ok := seen[r]; ok {
			return true
		}
		seen[r] = struct{}{}
	}
	return false
}

// CropFirst 从第一个 marker 开始取子串；找不到则返回原串。
func CropFirst(s, marker string, retain bool) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return s
	}
	if !retain {
		i += len(marker)
	}
	return s[i:]
}

// CropLast 取到最后一个 marker 为止；找不到则返回原串。
func CropLast(s, marker string, retain bool) string {
	i := strings.LastIndex(s, marker)
	if i < 0 {
		return s
	}
	if retain {
		i += len(marker)
	}
	return s[:i]
}

// SplitAt 返回第 index 个分段，支持负数索引；越界返回 false。
func SplitAt(s, sep string, index int) (string, bool) {
	parts := strings.Split(s, sep)
	if index < 0 {
		index += len(parts)
	}
	if index < 0 || index >= len(parts) {
		return "", false
	}
	return parts[index], true
}

// Truncate 最多保留 n 个码点，n<=0 返回空串。
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

const Alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// Random 使用 crypto/rand 均匀选择字符。alphabet 为空时使用 Alphanumeric。
// alphabet 必须为有效 UTF-8 且无重复码点；length 为码点数。
func Random(length int, alphabet string) (string, error) {
	if length < 0 {
		return "", errors.New("str: negative length")
	}
	if alphabet == "" {
		alphabet = Alphanumeric
	}
	if !utf8.ValidString(alphabet) || HasRepeat(alphabet) {
		return "", errors.New("str: invalid or duplicate alphabet characters")
	}
	runes := []rune(alphabet)
	result := make([]rune, length)
	bound := big.NewInt(int64(len(runes)))
	for i := range result {
		n, err := rand.Int(rand.Reader, bound)
		if err != nil {
			return "", err
		}
		result[i] = runes[n.Int64()]
	}
	return string(result), nil
}
