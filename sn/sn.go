// Package sn 根据本机硬件标识生成稳定的序列号。
package sn

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const namespace = "sn:v1"

type identity struct {
	source string
	value  string
}

// Get 读取本机硬件标识，返回 16 个大写十六进制字符；显示前缀由调用方添加。
// 支持 Linux、macOS 和 Windows；没有可用标识、标识无效或读取失败时返回错误。
// 不写入本地文件；相同来源和标识得到相同 SN，不保证硬件标识或摘要绝对唯一。
func Get() (string, error) {
	id, err := readHardwareIdentity()
	if err != nil {
		return "", err
	}
	return derive(id)
}

func derive(id identity) (string, error) {
	value := strings.TrimSpace(strings.TrimRight(id.value, "\x00"))
	switch id.source {
	case "smbios-uuid", "platform-uuid":
		if len(value) == 36 && value[8] == '-' && value[13] == '-' && value[18] == '-' && value[23] == '-' {
			value = value[:8] + value[9:13] + value[14:18] + value[19:23] + value[24:]
		}
		fallthrough
	case "mmc-cid":
		if len(value) != 32 {
			return "", fmt.Errorf("sn: invalid %s length", id.source)
		}
		if _, err := hex.DecodeString(value); err != nil {
			return "", fmt.Errorf("sn: invalid %s encoding", id.source)
		}
		value = strings.ToUpper(value)
	}
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("sn: empty or invalid %s", id.source)
	}
	// 仅在检查占位值时忽略常见分隔符，普通序列号仍保留大小写与分隔符。
	placeholder := strings.NewReplacer("-", "", ":", "", "_", "", ".", "", " ", "").Replace(strings.ToUpper(value))
	if strings.Trim(placeholder, "0") == "" || strings.Trim(placeholder, "F") == "" {
		return "", fmt.Errorf("sn: %s is a placeholder", id.source)
	}
	switch placeholder {
	case "UNKNOWN", "NONE", "NOTSPECIFIED", "DEFAULTSTRING", "SYSTEMSERIALNUMBER", "TOBEFILLEDBYOEM":
		return "", fmt.Errorf("sn: %s is a placeholder", id.source)
	}
	if id.source == "" {
		return "", errors.New("sn: identity source is empty")
	}
	digest := sha256.Sum256([]byte(namespace + "\x00" + id.source + "\x00" + value))
	return strings.ToUpper(hex.EncodeToString(digest[:8])), nil
}
