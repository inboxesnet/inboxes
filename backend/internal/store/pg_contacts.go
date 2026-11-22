package store

import (
	"context"
	"strings"
)

// suggestScanLimit is the number of newest emails a contact suggestion reads.
const suggestScanLimit = 5000

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

	// Read only the newest emails of the org: this query runs on each key
	// press, and a scan of all mail is too slow for that. An address is
	// reduced to its bare, lower-case form ("Name <a@b.c>" becomes "a@b.c"),
	// so that the client can use a suggestion as a recipient.
	rows, err := s.q.Query(ctx,
		`WITH recent AS MATERIALIZED (
		   SELECT e.from_address, e.to_addresses FROM emails e
		   WHERE e.org_id = $1
		     AND ($4 OR EXISTS (SELECT 1 FROM thread_labels tl
		                        WHERE tl.thread_id = e.thread_id AND tl.label = ANY($5::text[])))
		   ORDER BY e.created_at DESC
		   LIMIT $6
		 )
		 SELECT address AS email, COUNT(*) as count
		 FROM (
		   SELECT lower(btrim(COALESCE(substring(raw from '<([^<>]+)>'), raw))) AS address
		   FROM (
		     SELECT from_address AS raw FROM recent
		     UNION ALL
		     SELECT jsonb_array_elements_text(to_addresses) AS raw FROM recent
		   ) addrs
		 ) sub
		 WHERE address ILIKE $2
		 GROUP BY address
		 ORDER BY count DESC, address
		 LIMIT $3`,
		orgID, pattern, limit, isAdmin, aliasLabels, suggestScanLimit)
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
