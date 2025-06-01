package tokens

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
)

// parseUnixTimestamp parses a string as a Unix timestamp (seconds since epoch).
func parseUnixTimestamp(ts string) (int64, error) {
	return strconv.ParseInt(ts, 10, 64)
}

// fileSHA256 computes the SHA256 hex digest of a file at the given path.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// FormatDurationHuman takes a duration in seconds and returns a human-readable string.
func FormatDurationHuman(seconds int64) string {
	switch {
	case seconds < SecondsPerMinute:
		return fmt.Sprintf("%d sec", seconds)
	case seconds < SecondsPerHour:
		return fmt.Sprintf("%d min, %d sec", seconds/SecondsPerMinute, seconds%SecondsPerMinute)
	case seconds < SecondsPerDay:
		return fmt.Sprintf("%d hr, %d min", seconds/SecondsPerHour, (seconds%SecondsPerHour)/SecondsPerMinute)
	default:
		days := seconds / SecondsPerDay
		hours := (seconds % SecondsPerDay) / SecondsPerHour
		return fmt.Sprintf("%d day%s, %d hr", days, plural(days), hours)
	}
}

// plural returns "s" if n != 1, otherwise "".
func plural(n int64) string {
	if n == 1 {
		return ""
	}
	return "s"
}
