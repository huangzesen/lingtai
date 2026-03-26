package fs

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func IsAlive(dir string, thresholdSec float64) bool {
	data, err := os.ReadFile(filepath.Join(dir, ".agent.heartbeat"))
	if err != nil {
		return false
	}
	ts, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return false
	}
	age := time.Since(time.Unix(int64(ts), 0)).Seconds()
	return age < thresholdSec
}

func IsAliveHuman() bool {
	return true
}
