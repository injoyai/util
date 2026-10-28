// Package bytesize 解析和格式化字节大小。KB/MB 等沿用旧库的 1024 进制，
// 输出使用 KiB/MiB 消除歧义。解析使用有理数，避免 uint64 边界浮点溢出。
package bytesize

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"
)

const (
	KiB uint64 = 1 << 10
	MiB uint64 = 1 << 20
	GiB uint64 = 1 << 30
	TiB uint64 = 1 << 40
	PiB uint64 = 1 << 50
	EiB uint64 = 1 << 60
)

var token = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]*)`)
var units = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB", "EiB"}

// Parse 支持 "1.5 MiB"、"1GB512MB" 和无单位的字节数。
// 负数、未知单位、溢出、非整数总字节数均返回错误。
func Parse(s string) (uint64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("bytesize: empty size")
	}
	total := new(big.Rat)
	for s != "" {
		m := token.FindStringSubmatch(s)
		if m == nil {
			return 0, fmt.Errorf("bytesize: invalid size %q", s)
		}
		power := -1
		for i, unit := range units {
			if strings.EqualFold(m[2], unit) || strings.EqualFold(m[2], strings.ReplaceAll(unit, "i", "")) {
				power = i
				break
			}
		}
		if m[2] == "" {
			power = 0
		}
		if power < 0 {
			return 0, fmt.Errorf("bytesize: unknown unit %q", m[2])
		}
		value, ok := new(big.Rat).SetString(m[1])
		if !ok {
			return 0, errors.New("bytesize: invalid number")
		}
		multiplier := new(big.Int).Lsh(big.NewInt(1), uint(power*10))
		total.Add(total, value.Mul(value, new(big.Rat).SetInt(multiplier)))
		s = strings.TrimSpace(s[len(m[0]):])
	}
	if !total.IsInt() || !total.Num().IsUint64() {
		return 0, errors.New("bytesize: fractional bytes or overflow")
	}
	return total.Num().Uint64(), nil
}

func format(n float64, precision int) string {
	precision = max(0, min(precision, 6))
	i := 0
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	return fmt.Sprintf("%.*f%s", precision, n, units[i])
}

// Format precision 会限制在 0..6；仅显示值使用浮点近似。
func Format(n uint64, precision int) string { return format(float64(n), precision) }

// Speed 先计算每秒字节数再选择单位，elapsed 必须为正数。
func Speed(n uint64, elapsed time.Duration, precision int) (string, error) {
	if elapsed <= 0 {
		return "", errors.New("bytesize: elapsed must be positive")
	}
	return format(float64(n)/elapsed.Seconds(), precision) + "/s", nil
}
