package model

import "errors"

func resolveQueries(q *Queries) (*Queries, error) {
	if q != nil {
		return q, nil
	}
	target, err := getDefaultQueries()
	if err != nil {
		if errors.Is(err, ErrSQLCNotReady) {
			return nil, ErrDBNotInitialized
		}
		return nil, err
	}
	return target, nil
}

// ResolveQueries exposes the default query resolver for external packages.
func ResolveQueries(q *Queries) (*Queries, error) {
	return resolveQueries(q)
}
