package badtx

import (
	"context"

	"github.com/openclarion/openclarion/internal/persistence/ent"
)

func run(ctx context.Context, client *ent.Client, tx *ent.Tx) {
	_, _ = client.Tx(ctx) // want "only repository boundary packages may call ent.Client.Tx"
	_ = tx.Commit()       // want "only repository boundary packages may call ent.Tx.Commit"
	_ = tx.Rollback()     // want "only repository boundary packages may call ent.Tx.Rollback"
}
