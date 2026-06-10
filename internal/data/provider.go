package data

import (
	"context"
	"fmt"
	"time"

	"github.com/google/wire"
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	repousers "github.com/openmodu/oneshot/internal/repo/users"
	pkgsql "github.com/openmodu/oneshot/pkg/sql"
)

type MySQLDSN string

var ProviderSet = wire.NewSet(
	ProvideData,
	ProvideSQL,
	repoagents.NewAgentsRepo,
	repobilling.NewBillingRepo,
	repoorders.NewOrdersRepo,
	repousers.NewUsersRepo,
	NewOneShotRepo,
)

func ProvideData(dsn MySQLDSN) (*Data, func(), error) {
	if dsn == "" {
		return NewDataWithSQL(nil), func() {}, nil
	}

	db, err := pkgsql.NewMySQL(string(dsn))
	if err != nil {
		return nil, nil, fmt.Errorf("open mysql: %w", err)
	}

	raw, err := db.RawDB()
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("get mysql raw db: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := raw.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("ping mysql: %w", err)
	}

	data := NewDataWithSQL(db)
	return data, func() { _ = data.Close() }, nil
}

func ProvideSQL(d *Data) *pkgsql.Sql {
	return d.Sql
}
