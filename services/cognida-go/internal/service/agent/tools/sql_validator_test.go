package tools

import (
	"testing"
)

func TestSQLValidator_Validate_ValidQueries(t *testing.T) {
	v := NewSQLValidator()

	validQueries := []string{
		"SELECT * FROM users",
		"SELECT id, name FROM users WHERE id = 1",
		"SELECT COUNT(*) FROM orders GROUP BY status",
		"WITH cte AS (SELECT * FROM users) SELECT * FROM cte",
		"SELECT * FROM users LIMIT 10",
	}

	for _, sql := range validQueries {
		t.Run(sql, func(t *testing.T) {
			if err := v.Validate(sql); err != nil {
				t.Errorf("Valid query rejected: %s, error: %v", sql, err)
			}
		})
	}
}

func TestSQLValidator_Validate_DangerousQueries(t *testing.T) {
	v := NewSQLValidator()

	dangerousQueries := []struct {
		sql string
		expectErr string
	}{
		{"INSERT INTO users VALUES (1, 'test')", "dangerous keyword"},
		{"UPDATE users SET name = 'test'", "dangerous keyword"},
		{"DELETE FROM users", "dangerous keyword"},
		{"DROP TABLE users", "dangerous keyword"},
		{"SELECT * FROM users WHERE 1=1; DROP TABLE users", "injection"},
		{"SELECT * FROM users WHERE id = 1 OR 1=1", "injection"},
		{"SELECT * FROM users; SELECT * FROM orders", "multiple statements"},
	}

	for _, tc := range dangerousQueries {
		t.Run(tc.sql, func(t *testing.T) {
			if err := v.Validate(tc.sql); err == nil {
				t.Errorf("Dangerous query not rejected: %s", tc.sql)
			}
		})
	}
}

func TestSQLValidator_Tokenize(t *testing.T) {
	v := NewSQLValidator()

	sql := "SELECT id, name FROM users WHERE id = 1"
	tokens := v.tokenize(sql)

	expectedTokens := []string{"SELECT", "id", ",", "name", "FROM", "users", "WHERE", "id", "=", "1"}
	if len(tokens) != len(expectedTokens) {
		t.Errorf("Token count mismatch: got %d, want %d", len(tokens), len(expectedTokens))
	}
}
