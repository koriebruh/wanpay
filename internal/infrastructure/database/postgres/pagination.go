package postgres

const defaultLimit = 20
const maxLimit = 100

// normalizePage returns safe page and limit values.
// Page is 1-indexed. Limit is capped at maxLimit.
func normalizePage(page, limit int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return page, limit
}
