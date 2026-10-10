package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/Arpit529Srivastava/Foyr/internal/store"
)

// partitionsAhead is how many future months of audit partitions are kept ready.
const partitionsAhead = 2

// maintain runs housekeeping once now and then every interval until ctx ends.
func maintain(ctx context.Context, st *store.Store, log *slog.Logger, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		maintainOnce(ctx, st, log)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// maintainOnce creates upcoming audit partitions and prunes rows past retention (ADR-026).
func maintainOnce(ctx context.Context, st *store.Store, log *slog.Logger) {
	now := time.Now()
	created, err := st.EnsureAuditPartitions(ctx, now, partitionsAhead)
	if err != nil {
		log.Error("audit partitions failed", "err", err)
	} else if len(created) > 0 {
		log.Info("audit partitions created", "partitions", created)
	}
	pruned, err := st.Prune(ctx, now)
	if err != nil {
		log.Error("prune failed", "err", err)
		return
	}
	log.Info("maintenance done", "pruned_deliveries", pruned.Deliveries, "pruned_jobs", pruned.Jobs)
}
