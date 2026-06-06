package precontext

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/hindsight"
)

func (s *Service) fetchMemoriesSection(ctx context.Context, req Request) (string, error) {
	if isNilValue(s.cfg.Memory) || req.OrgID == uuid.Nil {
		return "", nil
	}
	if !isNilValue(s.cfg.MemoryBank) {
		if err := s.cfg.MemoryBank.EnsureOrgBank(ctx, req.OrgID); err != nil {
			return "", fmt.Errorf("ensure memory bank: %w", err)
		}
	}
	resp, err := s.cfg.Memory.ListMemories(ctx, hindsight.OrgBankID(req.OrgID), 100, 0)
	if err != nil {
		return "", fmt.Errorf("list memories: %w", err)
	}
	if resp == nil || len(resp.Items) == 0 {
		return "", nil
	}
	var lines []string
	for _, item := range resp.Items {
		text := memoryText(item)
		if text == "" {
			continue
		}
		prefix := "- "
		if timestamp := mapTimestamp(item, "created_at", "createdAt", "updated_at", "updatedAt", "timestamp", "time"); timestamp != "" {
			prefix += "[" + timestamp + "] "
		}
		lines = append(lines, prefix+trimToBytes(text, 180))
	}
	return section("## Recent memories", strings.Join(lines, "\n"), MemoriesBudgetBytes), nil
}

func memoryText(item map[string]any) string {
	if item == nil {
		return ""
	}
	return firstString(
		item["content"],
		item["text"],
		item["summary"],
		item["fact"],
		item["observation"],
		item["memory"],
		item["document"],
	)
}

func mapTimestamp(item map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := item[key].(type) {
		case string:
			if out := parseTimestampString(value); out != "" {
				return out
			}
		case float64:
			if out := unixTimestamp(value); out != "" {
				return out
			}
		case int64:
			if out := unixTimestamp(float64(value)); out != "" {
				return out
			}
		case int:
			if out := unixTimestamp(float64(value)); out != "" {
				return out
			}
		}
	}
	return ""
}

func parseTimestampString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC().Format(time.RFC3339)
		}
	}
	return cleanText(value)
}

func unixTimestamp(value float64) string {
	if value <= 0 {
		return ""
	}
	seconds := int64(value)
	if seconds > 9999999999 {
		seconds = seconds / 1000
	}
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}
