package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/wire"
	repoagents "github.com/openmodu/onecatch/internal/repo/agents"
	repoartifacts "github.com/openmodu/onecatch/internal/repo/artifacts"
	repobilling "github.com/openmodu/onecatch/internal/repo/billing"
	repoconversations "github.com/openmodu/onecatch/internal/repo/conversations"
	repoorders "github.com/openmodu/onecatch/internal/repo/orders"
	reposessions "github.com/openmodu/onecatch/internal/repo/sessions"
	repousers "github.com/openmodu/onecatch/internal/repo/users"
	pkgsql "github.com/openmodu/onecatch/pkg/sql"
	"gorm.io/gorm"
)

type MySQLDSN string

var ProviderSet = wire.NewSet(
	ProvideData,
	ProvideSQL,
	repoagents.NewAgentsRepo,
	repoartifacts.NewArtifactsRepo,
	repobilling.NewBillingRepo,
	repoconversations.NewConversationsRepo,
	repoorders.NewOrdersRepo,
	reposessions.NewSessionsRepo,
	repousers.NewUsersRepo,
	NewOneCatchRepo,
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

	// Run all schema migrations up front so failures surface at startup
	// instead of on the first request.
	for _, migrate := range []func(*gorm.DB) error{
		repoagents.Migrate,
		repoartifacts.Migrate,
		repobilling.Migrate,
		repoorders.Migrate,
		reposessions.Migrate,
		repousers.Migrate,
	} {
		if err := migrate(db.Gorm()); err != nil {
			_ = db.Close()
			return nil, nil, fmt.Errorf("migrate schema: %w", err)
		}
	}

	data := NewDataWithSQL(db)
	if err := Migrate(data); err != nil {
		_ = data.Close()
		return nil, nil, fmt.Errorf("migrate schema: %w", err)
	}
	return data, func() { _ = data.Close() }, nil
}

func ProvideSQL(d *Data) *pkgsql.Sql {
	return d.Sql
}
