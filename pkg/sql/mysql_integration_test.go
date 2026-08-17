package sql

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestMySQLPingWithDSN(t *testing.T) {
	dsn := os.Getenv("ONECATCH_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("set ONECATCH_MYSQL_TEST_DSN to run MySQL connectivity test")
	}

	db, err := NewMySQL(dsn)
	if err != nil {
		t.Fatalf("NewMySQL() error = %v", err)
	}
	defer db.Close()

	raw, err := db.RawDB()
	if err != nil {
		t.Fatalf("RawDB() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		t.Fatalf("PingContext() error = %v", err)
	}
}
