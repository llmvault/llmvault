package databaseintegration

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type RedisCommand struct {
	Command string
	Args    []string
}

var redisReadCommands = map[string]bool{
	"DBSIZE": true, "EXISTS": true, "GET": true, "GETRANGE": true,
	"HEXISTS": true, "HGET": true, "HLEN": true, "HMGET": true, "HSCAN": true, "HSTRLEN": true,
	"INFO": true, "LINDEX": true, "LLEN": true, "LRANGE": true,
	"MGET": true, "PING": true, "PTTL": true, "SCAN": true, "SCARD": true,
	"SISMEMBER": true, "SRANDMEMBER": true, "SSCAN": true, "STRLEN": true,
	"TIME": true, "TTL": true, "TYPE": true, "ZCARD": true, "ZCOUNT": true,
	"ZRANGE": true, "ZRANK": true, "ZREVRANGE": true, "ZREVRANK": true,
	"ZSCAN": true, "ZSCORE": true,
}

var redisRangeCommands = map[string]bool{
	"LRANGE": true, "ZRANGE": true, "ZREVRANGE": true,
}

func ValidateRedisCommand(body []byte, policy Policy) (RedisCommand, error) {
	cmd, err := parseRedisCommand(body)
	if err != nil {
		return RedisCommand{}, err
	}
	if !redisReadCommands[cmd.Command] {
		return RedisCommand{}, fmt.Errorf("only read-only Redis commands are allowed")
	}
	if err := validateRedisCommandShape(cmd, policy); err != nil {
		return RedisCommand{}, err
	}
	if err := enforceRedisPolicy(cmd, policy); err != nil {
		return RedisCommand{}, err
	}
	return applyRedisLimit(cmd, policy.MaxRows), nil
}

func parseRedisCommand(body []byte) (RedisCommand, error) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return RedisCommand{}, fmt.Errorf("Redis command is required")
	}
	var object struct {
		Command string `json:"command"`
		Cmd     string `json:"cmd"`
		Args    []any  `json:"args"`
	}
	if strings.HasPrefix(raw, "{") {
		if err := json.Unmarshal(body, &object); err != nil {
			return RedisCommand{}, fmt.Errorf("invalid Redis command JSON")
		}
		command := object.Command
		if command == "" {
			command = object.Cmd
		}
		return newRedisCommand(command, object.Args)
	}
	if strings.HasPrefix(raw, "[") {
		var parts []any
		if err := json.Unmarshal(body, &parts); err != nil {
			return RedisCommand{}, fmt.Errorf("invalid Redis command JSON")
		}
		if len(parts) == 0 {
			return RedisCommand{}, fmt.Errorf("Redis command is required")
		}
		return newRedisCommand(fmt.Sprint(parts[0]), parts[1:])
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return RedisCommand{}, fmt.Errorf("Redis command is required")
	}
	args := make([]any, 0, len(fields)-1)
	for _, field := range fields[1:] {
		args = append(args, field)
	}
	return newRedisCommand(fields[0], args)
}

func newRedisCommand(command string, args []any) (RedisCommand, error) {
	command = strings.ToUpper(strings.TrimSpace(command))
	if command == "" {
		return RedisCommand{}, fmt.Errorf("Redis command is required")
	}
	out := RedisCommand{Command: command, Args: make([]string, 0, len(args))}
	for _, arg := range args {
		switch typed := arg.(type) {
		case string:
			out.Args = append(out.Args, typed)
		case float64:
			out.Args = append(out.Args, strconv.FormatFloat(typed, 'f', -1, 64))
		case bool:
			out.Args = append(out.Args, strconv.FormatBool(typed))
		default:
			out.Args = append(out.Args, fmt.Sprint(typed))
		}
	}
	return out, nil
}

func validateRedisCommandShape(cmd RedisCommand, policy Policy) error {
	if redisFirstKeyCommand(cmd.Command) && len(cmd.Args) == 0 {
		return fmt.Errorf("Redis command %s requires a key", cmd.Command)
	}
	if cmd.Command == "MGET" || cmd.Command == "EXISTS" {
		if len(cmd.Args) == 0 {
			return fmt.Errorf("Redis command %s requires at least one key", cmd.Command)
		}
		if len(cmd.Args) > redisLimit(policy.MaxRows) {
			return fmt.Errorf("Redis command %s exceeds the configured key limit", cmd.Command)
		}
	}
	if redisRangeCommands[cmd.Command] {
		if len(cmd.Args) < 3 {
			return fmt.Errorf("Redis command %s requires start and stop offsets", cmd.Command)
		}
		start, err := strconv.Atoi(cmd.Args[1])
		if err != nil || start < 0 {
			return fmt.Errorf("Redis command %s requires a non-negative start offset", cmd.Command)
		}
		stop, err := strconv.Atoi(cmd.Args[2])
		if err != nil || stop < start {
			return fmt.Errorf("Redis command %s requires a bounded stop offset", cmd.Command)
		}
		if stop-start+1 > redisLimit(policy.MaxRows) {
			return fmt.Errorf("Redis command %s exceeds the configured result limit", cmd.Command)
		}
	}
	if cmd.Command == "SRANDMEMBER" && len(cmd.Args) > 1 {
		count, err := strconv.Atoi(cmd.Args[1])
		if err != nil || absInt(count) > redisLimit(policy.MaxRows) {
			return fmt.Errorf("Redis command SRANDMEMBER exceeds the configured result limit")
		}
	}
	return nil
}

func enforceRedisPolicy(cmd RedisCommand, policy Policy) error {
	keys := redisCommandKeys(cmd)
	if len(keys) == 0 {
		if cmd.Command == "SCAN" && len(policy.AllowedKeys) > 0 {
			match := redisOptionValue(cmd.Args, "MATCH")
			if match == "" || !redisKeyAllowed(match, policy.AllowedKeys) {
				return fmt.Errorf("Redis SCAN pattern is outside the configured database access policy")
			}
		}
		return nil
	}
	for _, key := range keys {
		if !redisKeyAllowed(key, policy.AllowedKeys) {
			return fmt.Errorf("Redis key %q is outside the configured database access policy", key)
		}
	}
	return nil
}

func redisCommandKeys(cmd RedisCommand) []string {
	switch {
	case cmd.Command == "MGET" || cmd.Command == "EXISTS":
		return cmd.Args
	case redisFirstKeyCommand(cmd.Command):
		return cmd.Args[:1]
	default:
		return nil
	}
}

func redisFirstKeyCommand(command string) bool {
	switch command {
	case "GET", "GETRANGE", "STRLEN", "TTL", "PTTL", "TYPE",
		"HGET", "HMGET", "HLEN", "HEXISTS", "HSTRLEN", "HSCAN",
		"LLEN", "LINDEX", "LRANGE",
		"SCARD", "SISMEMBER", "SRANDMEMBER", "SSCAN",
		"ZCARD", "ZCOUNT", "ZSCORE", "ZRANK", "ZREVRANK", "ZRANGE", "ZREVRANGE", "ZSCAN":
		return true
	default:
		return false
	}
}

func redisKeyAllowed(key string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	key = strings.ToLower(strings.TrimSpace(key))
	for _, pattern := range allowed {
		if redisGlobMatch(strings.ToLower(strings.TrimSpace(pattern)), key) {
			return true
		}
	}
	return false
}

func redisGlobMatch(pattern, value string) bool {
	if pattern == "" {
		return false
	}
	quoted := regexp.QuoteMeta(pattern)
	quoted = strings.ReplaceAll(quoted, `\*`, ".*")
	quoted = strings.ReplaceAll(quoted, `\?`, ".")
	matched, err := regexp.MatchString("^"+quoted+"$", value)
	return err == nil && matched
}

func applyRedisLimit(cmd RedisCommand, maxRows int) RedisCommand {
	switch cmd.Command {
	case "SCAN", "HSCAN", "SSCAN", "ZSCAN":
		if redisOptionValue(cmd.Args, "COUNT") == "" {
			cmd.Args = append(cmd.Args, "COUNT", strconv.Itoa(redisLimit(maxRows)))
		}
	}
	return cmd
}

func redisOptionValue(args []string, option string) string {
	option = strings.ToUpper(option)
	for idx := 0; idx < len(args)-1; idx++ {
		if strings.ToUpper(args[idx]) == option {
			return args[idx+1]
		}
	}
	return ""
}

func redisLimit(maxRows int) int {
	if maxRows <= 0 || maxRows > 1000 {
		return DefaultMaxRows
	}
	return maxRows
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
