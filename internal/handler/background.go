package handler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"readress/internal/db/sqlc"
)

func (a *App) StartBackground(ctx context.Context) {
	jobs := make(chan int64, 128)
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case feedID := <-jobs:
					result, err := a.fetcher.FetchFeed(ctx, a.userID, feedID)
					if err != nil {
						a.logger.Warn("scheduled feed fetch failed", slog.Int64("feed_id", feedID), slog.String("error", err.Error()))
						continue
					}
					if result.Inserted > 0 {
						a.logger.Info("scheduled feed fetch complete", slog.Int64("feed_id", feedID), slog.Int("inserted", result.Inserted))
					}
				}
			}
		}(i)
	}

	go func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		defer close(jobs)
		a.runScheduledMaintenance(ctx, jobs)
		for {
			select {
			case <-ctx.Done():
				wg.Wait()
				return
			case <-ticker.C:
				a.runScheduledMaintenance(ctx, jobs)
			}
		}
	}()
}

func (a *App) runScheduledMaintenance(ctx context.Context, jobs chan<- int64) {
	settings, err := a.queries.GetReaderSettings(ctx, a.userID)
	if err == nil && settings.RetentionDays > 0 {
		if err := a.queries.DeleteReadArticlesOlderThan(ctx, sqlc.DeleteReadArticlesOlderThanParams{PRINTF: settings.RetentionDays, UserID: a.userID}); err != nil {
			a.logger.Warn("retention cleanup failed", slog.String("error", err.Error()))
		}
	}
	feeds, err := a.queries.ListDueFeeds(ctx, sqlc.ListDueFeedsParams{UserID: a.userID, Limit: 50})
	if err != nil {
		a.logger.Warn("list due feeds failed", slog.String("error", err.Error()))
		return
	}
	for _, feed := range feeds {
		select {
		case jobs <- feed.ID:
		default:
			a.logger.Warn("scheduler queue full", slog.Int64("feed_id", feed.ID))
		}
	}
}
