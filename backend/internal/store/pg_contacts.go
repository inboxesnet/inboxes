package store

import (
	"context"
	"strings"
)

func (s *PgStore) SuggestContacts(ctx context.Context, orgID, query string, limit int, role string, aliasLabels []string) ([]map[string]any, error) {
	if limit <= 0 {
		limit = 10
	}

	pattern := "%" + escapeLIKE(query) + "%"

	// Members only see counterparties from threads their aliases can see;
	// admins see the whole org. Without the filter a member could enumerate
	// every colleague's contacts one prefix at a time.
	isAdmin := role == "admin"
	if aliasLabels == nil {
		aliasLabels = []string{}
	}

	rows, err := s.q.Query(ctx,
		`SELECT address AS email, COUNT(*) as count
		 FROM (
		   SELECT from_address as address FROM emails e WHERE e.org_id = $1
		     AND ($4 OR EXISTS (SELECT 1 FROM thread_labels tl
		                        WHERE tl.thread_id = e.thread_id AND tl.label = ANY($5::text[])))
		   UNION ALL
		   SELECT jsonb_array_elements_text(to_addresses) as address FROM emails e WHERE e.org_id = $1
		     AND ($4 OR EXISTS (SELECT 1 FROM thread_labels tl
		                        WHERE tl.thread_id = e.thread_id AND tl.label = ANY($5::text[])))
		 ) sub
		 WHERE address ILIKE $2
		 GROUP BY address
		 ORDER BY count DESC
		 LIMIT $3`,
		orgID, pattern, limit, isAdmin, aliasLabels)
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
