package app_test

import (
	"testing"
)

// ---------- ids ----------

func TestIDsAreUUIDs(t *testing.T) {
	e := newEnv(t)
	// every primary and foreign key except external references is a uuid
	n := scalar[int64](e, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema='public' AND (column_name='id' OR column_name LIKE '%\_id') AND data_type NOT IN ('uuid','bigint')
		AND table_name NOT IN ('webhook_events','sim_clock')`)
	if n != 0 {
		t.Fatalf("%d id columns are not uuid", n)
	}
}
