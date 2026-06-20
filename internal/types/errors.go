package types

import "errors"

var (
	ErrDBTimeout  = errors.New("db timeout")
	ErrRunTimeout = errors.New("run timeout")

	ErrRequestCanceled = errors.New("request canceled")
)
