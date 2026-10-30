package services

import "errors"

// errRollback ends a transaction without an error to report: the caller already set the outcome.
var errRollback = errors.New("rollback")
