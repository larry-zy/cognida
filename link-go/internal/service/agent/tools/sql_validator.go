// Package tools 提供 SQL 安全验证器
package tools

import (
	"fmt"
	"regexp"
	"strings"
)

// SQLValidator SQL 验证器
// 用于验证生成的 SQL 是否安全，防止 SQL 注入和危险操作
type SQLValidator struct {
	// 只读关键字白名单
	readOnlyKeywords map[string]bool

	// 危险关键词黑名单
	dangerousKeywords map[string]bool

	// 注入模式
	injectionPatterns []*regexp.Regexp
}

// NewSQLValidator 创建 SQL 验证器
func NewSQLValidator() *SQLValidator {
	return &SQLValidator{
		readOnlyKeywords: map[string]bool{
			"SELECT": true,
			"WITH":   true,
			"UNION":  true,
			"UNIQUE": true,
			"FROM":   true,
			"WHERE":  true,
			"GROUP":  true,
			"ORDER":  true,
			"HAVING": true,
			"LIMIT":  true,
			"OFFSET": true,
			"JOIN":   true,
			"INNER":  true,
			"LEFT":   true,
			"RIGHT":  true,
			"FULL":   true,
			"CROSS":  true,
			"AS":     true,
			"AND":    true,
			"OR":     true,
			"NOT":    true,
			"IN":     true,
			"EXISTS": true,
			"BETWEEN": true,
			"LIKE":   true,
			"IS":     true,
			"NULL":   true,
			"CASE":   true,
			"WHEN":   true,
			"THEN":   true,
			"ELSE":   true,
			"END":    true,
			"DISTINCT": true,
			"ALL":    true,
			"ANY":    true,
			"SOME":   true,
		},
		dangerousKeywords: map[string]bool{
			"INSERT":   true,
			"UPDATE":   true,
			"DELETE":   true,
			"DROP":     true,
			"CREATE":   true,
			"ALTER":    true,
			"TRUNCATE": true,
			"GRANT":    true,
			"REVOKE":   true,
			"EXECUTE":  true,
			"EXEC":     true,
			"--":       true, // 单行注释
			"/*":       true, // 多行注释开始
			"xp_":      true, // SQL Server 扩展存储过程
			"sp_":      true, // SQL Server 系统存储过程
		},
		injectionPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\bor\s+1\s*=\s*1\b`),     // SQL 注入常见模式
			regexp.MustCompile(`(?i)\bor\s+true\b`),           // OR true 注入
			regexp.MustCompile(`(?i);\s*DROP\b`),              // 多语句注入
			regexp.MustCompile(`(?i);\s*DELETE\b`),            // 多语句注入
			regexp.MustCompile(`(?i);\s*UPDATE\b`),            // 多语句注入
			regexp.MustCompile(`(?i);\s*INSERT\b`),            // 多语句注入
			regexp.MustCompile(`(?i)'.*OR.*'.*'`),             // 条件注入
			regexp.MustCompile(`(?i)\bunion\s+all\s+select\b`), // UNION ALL SELECT
			regexp.MustCompile(`(?i)waitfor\s+delay\b`),       // SQL Server 时间注入
			regexp.MustCompile(`(?i)benchmark\s*\(`),          // MySQL 盲注
			regexp.MustCompile(`(?i)sleep\s*\(`),             // MySQL 时间注入
		},
	}
}

// Validate 验证 SQL 安全性
// 返回 error 如果 SQL 不安全
func (v *SQLValidator) Validate(sql string) error {
	if sql == "" {
		return fmt.Errorf("empty SQL query")
	}

	sqlUpper := strings.ToUpper(strings.TrimSpace(sql))

	// 1. 必须以 SELECT 或 WITH 开头（只读查询）
	if !strings.HasPrefix(sqlUpper, "SELECT") && !strings.HasPrefix(sqlUpper, "WITH") {
		return fmt.Errorf("query must start with SELECT or WITH, got: %s", sqlUpper[:min(20, len(sqlUpper))])
	}

	// 2. 检查危险关键词
	tokens := v.tokenize(sqlUpper)
	for _, token := range tokens {
		if v.dangerousKeywords[token] {
			return fmt.Errorf("dangerous keyword detected: %s", token)
		}
	}

	// 3. 检查注入模式
	for _, pattern := range v.injectionPatterns {
		if pattern.MatchString(sql) {
			return fmt.Errorf("potential SQL injection detected")
		}
	}

	// 4. 检查多语句（分号数量和位置）
	// 允许单个结尾分号，但不允许中间的分号
	trimmedSQL := strings.TrimSpace(sql)
	semicolonIdx := strings.Index(trimmedSQL, ";")
	if semicolonIdx != -1 {
		// 如果有分号，检查是否在末尾
		lastSemicolonIdx := strings.LastIndex(trimmedSQL, ";")
		if lastSemicolonIdx != len(trimmedSQL)-1 {
			return fmt.Errorf("multiple statements not allowed (semicolon in middle)")
		}
		// 检查是否有多个分号（即使最后一个在末尾）
		semicolonCount := strings.Count(trimmedSQL, ";")
		if semicolonCount > 1 {
			return fmt.Errorf("multiple statements not allowed (found %d semicolons)", semicolonCount)
		}
	}

	// 5. 检查未闭合的字符串
	quoteCount := 0
	inSingleQuote := false
	inDoubleQuote := false
	for i, c := range sql {
		if c == '\'' && !inDoubleQuote {
			if !inSingleQuote || (i > 0 && sql[i-1] != '\\') {
				inSingleQuote = !inSingleQuote
				if inSingleQuote {
					quoteCount++
				}
			}
		}
		if c == '"' && !inSingleQuote {
			if !inDoubleQuote || (i > 0 && sql[i-1] != '\\') {
				inDoubleQuote = !inDoubleQuote
			}
		}
	}
	if quoteCount%2 != 0 {
		return fmt.Errorf("unclosed quote detected")
	}

	return nil
}

// tokenize 分词（简单实现，按空格和括号分割）
func (v *SQLValidator) tokenize(sql string) []string {
	// 移除多余空白
	sql = strings.TrimSpace(sql)

	var tokens []string
	var currentToken strings.Builder

	inString := false
	prevChar := ' '

	for i, c := range sql {
		switch c {
		case ' ', '\t', '\n', '\r', '(', ')', ',', ';':
			if inString {
				currentToken.WriteRune(c)
			} else {
				if currentToken.Len() > 0 {
					tokens = append(tokens, currentToken.String())
					currentToken.Reset()
				}
				if c == '(' || c == ')' || c == ',' || c == ';' {
					tokens = append(tokens, string(c))
				}
			}
		case '\'':
			if !inString || (i > 0 && prevChar != '\\') {
				inString = !inString
			}
			currentToken.WriteRune(c)
		default:
			currentToken.WriteRune(c)
		}
		prevChar = c
	}

	if currentToken.Len() > 0 {
		tokens = append(tokens, currentToken.String())
	}

	return tokens
}

// IsReadOnlyKeyword 检查是否为只读关键字
func (v *SQLValidator) IsReadOnlyKeyword(keyword string) bool {
	return v.readOnlyKeywords[strings.ToUpper(keyword)]
}

// IsDangerousKeyword 检查是否为危险关键字
func (v *SQLValidator) IsDangerousKeyword(keyword string) bool {
	return v.dangerousKeywords[strings.ToUpper(keyword)]
}
