package ingest

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ttlLog         = 7 * 24 * time.Hour
	ttlFlow        = 14 * 24 * time.Hour
	ttlCounter     = 30 * 24 * time.Hour
	ttlSystem      = 30 * 24 * time.Hour
	lookaheadDays  = 7
	lookaheadWeeks = 2
)

// validPartitionName guards against unexpected names from pg_class before DDL.
var validPartitionName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// PartitionManager creates future partitions and drops expired ones.
type PartitionManager struct {
	pool *pgxpool.Pool
}

func NewPartitionManager(pool *pgxpool.Pool) *PartitionManager {
	return &PartitionManager{pool: pool}
}

// Start runs the partition manager; blocks until ctx is cancelled.
func (pm *PartitionManager) Start(ctx context.Context) {
	pm.run(ctx)
	tick := time.NewTicker(24 * time.Hour)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			pm.run(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (pm *PartitionManager) run(ctx context.Context) {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	for i := 0; i <= lookaheadDays; i++ {
		d := today.AddDate(0, 0, i)
		dNext := d.AddDate(0, 0, 1)
		pm.createPartition(ctx, "log_events",
			"log_events_"+d.Format("2006_01_02"), d, dNext)
		pm.createPartition(ctx, "flow_events",
			"flow_events_"+d.Format("2006_01_02"), d, dNext)
	}

	w := isoWeekStart(today)
	for i := 0; i <= lookaheadWeeks; i++ {
		wNext := w.AddDate(0, 0, 7)
		year, week := w.ISOWeek()
		name := fmt.Sprintf("counter_snapshots_%04d_%02d", year, week)
		pm.createPartition(ctx, "counter_snapshots", name, w, wNext)
		w = wNext
	}

	pm.dropExpiredDaily(ctx, "log_events", today, ttlLog)
	pm.dropExpiredDaily(ctx, "flow_events", today, ttlFlow)
	pm.dropExpiredWeekly(ctx, today, ttlCounter)
	pm.pruneSystemEvents(ctx, now.Add(-ttlSystem))
}

func (pm *PartitionManager) createPartition(ctx context.Context, parent, name string, from, to time.Time) {
	// Inputs are controlled by this package; format strings are safe.
	sql := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')",
		name, parent, from.Format("2006-01-02"), to.Format("2006-01-02"),
	)
	if _, err := pm.pool.Exec(ctx, sql); err != nil {
		slog.Error("partition manager: create failed", "table", name, "err", err)
	}
}

func (pm *PartitionManager) dropExpiredDaily(ctx context.Context, base string, today time.Time, ttl time.Duration) {
	cutoff := today.Add(-ttl)
	names := pm.listPartitions(ctx, base)
	for _, name := range names {
		t, err := parseDailyPartitionDate(name, base)
		if err != nil {
			continue
		}
		if t.Before(cutoff) {
			pm.dropPartition(ctx, name)
		}
	}
}

func (pm *PartitionManager) dropExpiredWeekly(ctx context.Context, today time.Time, ttl time.Duration) {
	cutoff := today.Add(-ttl)
	names := pm.listPartitions(ctx, "counter_snapshots")
	for _, name := range names {
		t, err := parseWeeklyPartitionDate(name)
		if err != nil {
			continue
		}
		if t.AddDate(0, 0, 7).Before(cutoff) {
			pm.dropPartition(ctx, name)
		}
	}
}

func (pm *PartitionManager) listPartitions(ctx context.Context, parent string) []string {
	rows, err := pm.pool.Query(ctx,
		`SELECT c.relname
		 FROM pg_inherits i
		 JOIN pg_class c ON c.oid = i.inhrelid
		 JOIN pg_class p ON p.oid = i.inhparent
		 WHERE p.relname = $1`, parent)
	if err != nil {
		slog.Error("partition manager: list partitions failed", "parent", parent, "err", err)
		return nil
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err == nil {
			names = append(names, name)
		}
	}
	return names
}

func (pm *PartitionManager) dropPartition(ctx context.Context, name string) {
	if !validPartitionName.MatchString(name) {
		slog.Error("partition manager: suspicious partition name, skipping drop", "name", name)
		return
	}
	if _, err := pm.pool.Exec(ctx, "DROP TABLE IF EXISTS "+name); err != nil {
		slog.Error("partition manager: drop failed", "table", name, "err", err)
	} else {
		slog.Info("partition manager: dropped expired partition", "table", name)
	}
}

func (pm *PartitionManager) pruneSystemEvents(ctx context.Context, before time.Time) {
	tag, err := pm.pool.Exec(ctx, "DELETE FROM system_events WHERE created_at < $1", before)
	if err != nil {
		slog.Error("partition manager: prune system_events failed", "err", err)
	} else if tag.RowsAffected() > 0 {
		slog.Info("partition manager: pruned system_events", "rows", tag.RowsAffected())
	}
}

func parseDailyPartitionDate(name, base string) (time.Time, error) {
	prefix := base + "_"
	if len(name) <= len(prefix) {
		return time.Time{}, fmt.Errorf("name too short")
	}
	return time.Parse("2006_01_02", name[len(prefix):])
}

func parseWeeklyPartitionDate(name string) (time.Time, error) {
	const prefix = "counter_snapshots_"
	if len(name) <= len(prefix) {
		return time.Time{}, fmt.Errorf("name too short")
	}
	var year, week int
	if _, err := fmt.Sscanf(name[len(prefix):], "%d_%d", &year, &week); err != nil {
		return time.Time{}, err
	}
	return isoWeekStartYW(year, week), nil
}

func isoWeekStart(t time.Time) time.Time {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return t.AddDate(0, 0, -(wd - 1))
}

func isoWeekStartYW(year, week int) time.Time {
	// Jan 4 is always in ISO week 1.
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	mondayWeek1 := jan4.AddDate(0, 0, -(wd - 1))
	return mondayWeek1.AddDate(0, 0, (week-1)*7)
}
