package repository_test

import (
	"context"

	"github.com/openclarion/openclarion/internal/persistence/ent"
)

func run(ctx context.Context, client *ent.Client, tx *ent.Tx) {
	_, _ = client.Tx(ctx)
	_ = tx.Commit()
	_ = tx.Rollback()
}
