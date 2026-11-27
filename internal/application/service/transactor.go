package service

import "context"

// Transactor runs fn in one transaction. fn must use its callback context for all
// repository calls. A nil result commits; an error or panic rolls back.
// External side effects belong after WithinTx returns successfully.
type Transactor interface {
	WithinTx(ctx context.Context, fn func(context.Context) error) error
}
