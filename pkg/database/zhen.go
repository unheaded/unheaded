// SPDX-License-Identifier: GPL-3.0-or-later
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

// ZhenConversation represents a single message in a Zhen conversation.
type ZhenConversation struct {
	ID           int64           `json:"id"`
	SessionID    string          `json:"session_id"`
	Role         string          `json:"role"`
	Content      string          `json:"content"`
	Sources      json.RawMessage `json:"sources"` // JSONB
	Model        string          `json:"model"`
	TokensInput  int             `json:"tokens_input"`
	TokensOutput int             `json:"tokens_output"`
	ElapsedMs    int             `json:"elapsed_ms"`
	CreatedAt    time.Time       `json:"created_at"`
}

// ZhenStore provides PostgreSQL-backed Zhen conversation persistence.
type ZhenStore struct {
	db *sql.DB
}

// NewZhenStore creates a new ZhenStore backed by the given database connection.
func NewZhenStore(db *sql.DB) *ZhenStore {
	return &ZhenStore{db: db}
}

// LogConversation inserts a conversation message into The Well.
func (s *ZhenStore) LogConversation(ctx context.Context, conv ZhenConversation) error {
	sources := conv.Sources
	if len(sources) == 0 {
		sources = json.RawMessage("[]")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO zhen_conversations
		    (session_id, role, content, sources, model, tokens_input, tokens_output, elapsed_ms)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8)`,
		conv.SessionID, conv.Role, conv.Content, string(sources),
		conv.Model, conv.TokensInput, conv.TokensOutput, conv.ElapsedMs)
	return err
}

// ListRecent returns the most recent conversations, ordered newest first.
func (s *ZhenStore) ListRecent(ctx context.Context, limit int) ([]ZhenConversation, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, sources, model,
		        tokens_input, tokens_output, elapsed_ms, created_at
		 FROM zhen_conversations
		 ORDER BY created_at DESC
		 LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []ZhenConversation
	for rows.Next() {
		var c ZhenConversation
		var sources []byte
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Role, &c.Content, &sources,
			&c.Model, &c.TokensInput, &c.TokensOutput, &c.ElapsedMs, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Sources = json.RawMessage(sources)
		convs = append(convs, c)
	}
	return convs, rows.Err()
}

// Search performs full-text search over conversation content using PostgreSQL tsvector.
func (s *ZhenStore) Search(ctx context.Context, query string, limit int) ([]ZhenConversation, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, sources, model,
		        tokens_input, tokens_output, elapsed_ms, created_at
		 FROM zhen_conversations
		 WHERE search_vector @@ websearch_to_tsquery('english', $1)
		 ORDER BY ts_rank(search_vector, websearch_to_tsquery('english', $1)) DESC,
		          created_at DESC
		 LIMIT $2`, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var convs []ZhenConversation
	for rows.Next() {
		var c ZhenConversation
		var sources []byte
		if err := rows.Scan(&c.ID, &c.SessionID, &c.Role, &c.Content, &sources,
			&c.Model, &c.TokensInput, &c.TokensOutput, &c.ElapsedMs, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Sources = json.RawMessage(sources)
		convs = append(convs, c)
	}
	return convs, rows.Err()
}
