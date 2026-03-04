package store

import (
	"context"
	"strings"
)

func (s *PgStore) SuggestContacts(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}

	pattern := "%" + escapeLIKE(query) + "%"

	rows, err := s.q.Query(ctx,
		`SELECT address AS email, COUNT(*) as count
		 FROM (
		   SELECT from_address as address FROM emails WHERE org_id = $1
		   UNION ALL
		   SELECT jsonb_array_elements_text(to_addresses) as address FROM emails WHERE org_id = $1
		 ) sub
		 WHERE address ILIKE $2
		 GROUP BY address
		 ORDER BY count DESC
		 LIMIT $3`,
		orgID, pattern, limit)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

// escapeLIKE escapes LIKE/ILIKE metacharacters (%, _, \) in a search string.
func escapeLIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}
