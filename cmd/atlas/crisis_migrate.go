package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/newthinker/atlas/internal/crisis"
)

var migrateDBPath string

var crisisMigrateCmd = &cobra.Command{
	Use:   "migrate-bitemporal",
	Short: "Migrate macro_observations to the bitemporal (ts, indicator, fetched_at) key",
	Long: `Rewrites macro_observations with a three-part primary key so the same
observation date can carry several revisions, keeps the pre-migration table as
macro_observations_v1, and rebuilds the index and the v_macro_current view.

Idempotent: running it on an already-migrated db is a no-op. The db is taken
by --db rather than the crisis config, so an operator always names the file
being rewritten. Three counts are printed — RowsBefore, RowsAfter and ViewRows
— and they must all be equal for the migration to be considered good.`,
	RunE: runCrisisMigrate,
}

func init() {
	crisisMigrateCmd.Flags().StringVar(&migrateDBPath, "db", "",
		"path to the crisis sqlite db to migrate (required)")
	crisisCmd.AddCommand(crisisMigrateCmd)
}

func runCrisisMigrate(cmd *cobra.Command, args []string) error {
	return executeCrisisMigrate(cmd.Context(), cmd.OutOrStdout(), migrateDBPath)
}

// executeCrisisMigrate 是可单测的主体（模式同 executeCrisisReport）。
//
// 迁移走 crisis.MigrateBitemporal，它刻意不经 NewStore —— 老形状的库正是
// 它要处理的对象，而 NewStore 之后会有守卫拒绝那种库（AD-5）。
func executeCrisisMigrate(ctx context.Context, out io.Writer, dbPath string) error {
	if dbPath == "" {
		return fmt.Errorf("--db is required (path to the crisis sqlite db)")
	}

	res, err := crisis.MigrateBitemporal(ctx, dbPath)
	if err != nil {
		return err
	}
	if res.AlreadyMigrated {
		fmt.Fprintf(out, "%s: already migrated, nothing to do\n", dbPath)
		return nil
	}

	fmt.Fprintf(out, "migrated %s\n", dbPath)
	fmt.Fprintf(out, "  RowsBefore %d\n", res.RowsBefore)
	fmt.Fprintf(out, "  RowsAfter  %d\n", res.RowsAfter)
	fmt.Fprintf(out, "  ViewRows   %d\n", res.ViewRows)
	// 三者相等才算好；不相等时说出来，别让运维自己比对三行数字。
	if res.RowsBefore != res.RowsAfter || res.RowsAfter != res.ViewRows {
		fmt.Fprintf(out, "  ⚠ counts differ — expected all three to match; inspect %s\n", dbPath)
	}
	return nil
}
