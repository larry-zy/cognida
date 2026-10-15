package tools

import (
	"fmt"
	"regexp"
)

// Private system tables are accessible only through their owner-scoped domain
// services. The restriction applies to every connection, including an external
// datasource pointing back at this application's database. Match identifier
// tokens anywhere (also CTEs, subqueries, qualifiers, quoted identifiers and
// literals) conservatively, without depending on a dialect-specific SQL parser.
var privateSystemIdentifier = regexp.MustCompile(`(?i)(^|[^a-z0-9_$])user_memories([^a-z0-9_$]|$)`)

func validatePrivateDataAccess(sqlOrIdentifier string) error {
	if privateSystemIdentifier.MatchString(sqlOrIdentifier) {
		return fmt.Errorf("private system tables are unavailable to general data tools; use the owner-scoped memory tools")
	}
	return nil
}
