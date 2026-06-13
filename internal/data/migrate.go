package data

import (
	repoagents "github.com/openmodu/oneshot/internal/repo/agents"
	repoartifacts "github.com/openmodu/oneshot/internal/repo/artifacts"
	repobilling "github.com/openmodu/oneshot/internal/repo/billing"
	repoconversations "github.com/openmodu/oneshot/internal/repo/conversations"
	repoorders "github.com/openmodu/oneshot/internal/repo/orders"
	reposessions "github.com/openmodu/oneshot/internal/repo/sessions"
	repousers "github.com/openmodu/oneshot/internal/repo/users"
	"gorm.io/gorm"
)

// Migrate creates or updates every repo's tables and seeds reference data.
// Run once at startup so request handling never triggers schema work. It is a
// no-op in memory mode (no SQL handle).
func Migrate(d *Data) error {
	if d == nil || d.Sql == nil || d.Sql.Gorm() == nil {
		return nil
	}
	db := d.Sql.Gorm()
	for _, migrate := range []func(db *gorm.DB) error{
		repoagents.Migrate,
		repoartifacts.Migrate,
		repobilling.Migrate,
		repoconversations.Migrate,
		repoorders.Migrate,
		reposessions.Migrate,
		repousers.Migrate,
	} {
		if err := migrate(db); err != nil {
			return err
		}
	}
	return nil
}
