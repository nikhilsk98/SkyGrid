package config

import (
	"log/slog"
	"net/url"
	"regexp"
	"strconv"
)

// kvPassword matches password=... in key/value DSNs ("host=x password=y"),
// with or without single quotes.
var kvPassword = regexp.MustCompile(`(?i)(password\s*=\s*)('(?:[^'\\]|\\.)*'|\S+)`)

// RedactDSN hides the password in a Postgres DSN so it is safe to log.
// Both URL ("postgres://u:p@h/db") and key/value ("password=p") forms are handled.
func RedactDSN(dsn string) string {
	if u, err := url.Parse(dsn); err == nil && u.Scheme != "" {
		q := u.Query()
		if q.Has("password") {
			q.Set("password", "xxxxx")
			u.RawQuery = q.Encode()
		}
		return u.Redacted()
	}
	return kvPassword.ReplaceAllString(dsn, "${1}xxxxx")
}

// LogValue logs the metadata section with its password redacted.
func (m Metadata) LogValue() slog.Value {
	return slog.GroupValue(slog.String("dsn", RedactDSN(m.DSN)))
}

// LogValue logs a shard with its password redacted.
func (s Shard) LogValue() slog.Value {
	return slog.GroupValue(slog.Int("id", s.ID), slog.String("dsn", RedactDSN(s.DSN)))
}

// LogValue logs every shard as id -> redacted DSN. Without it the JSON
// handler would marshal the raw slice and leak passwords.
func (s ShardList) LogValue() slog.Value {
	attrs := make([]slog.Attr, 0, len(s))
	for _, sh := range s {
		attrs = append(attrs, slog.String(strconv.Itoa(sh.ID), RedactDSN(sh.DSN)))
	}
	return slog.GroupValue(attrs...)
}

// LogValue logs the whole config with every DSN redacted.
func (c *Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Any("grid", c.Grid),
		slog.Any("metadata", c.Metadata),
		slog.Any("shards", c.Shards),
		slog.Any("coordinator", c.Coordinator),
		slog.Any("recovery", c.Recovery),
		slog.Any("simulator", c.Simulator),
	)
}
