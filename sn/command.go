package sn

import (
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func hardwareCommand(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.WaitDelay = time.Second
	data, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return data, err
}

func parsePlatformUUID(data string) (identity, error) {
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok && strings.Trim(key, " \t|") == `"IOPlatformUUID"` {
			uuid, err := strconv.Unquote(strings.TrimSpace(value))
			if err != nil {
				return identity{}, errors.New("sn: invalid IOPlatformUUID property")
			}
			return identity{"platform-uuid", uuid}, nil
		}
	}
	return identity{}, errors.New("sn: IOPlatformUUID is unavailable")
}
