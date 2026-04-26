package notification

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// encodeCursor returns a base64 string representing the position after the
// given item. The cursor encodes created_at (RFC3339Nano) and id so that
// the next page query can use a stable keyset: WHERE (created_at, id) < cursor.
func encodeCursor(createdAt time.Time, id string) string {
	raw := fmt.Sprintf("%s|%s", createdAt.UTC().Format(time.RFC3339Nano), id)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeCursor parses a cursor string back into its components.
func decodeCursor(cursor string) (time.Time, string, error) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor encoding: %w", err)
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return time.Time{}, "", fmt.Errorf("invalid cursor format")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid cursor timestamp: %w", err)
	}
	return t, parts[1], nil
}
