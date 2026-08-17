package users

import (
	"context"
	"os"
	"testing"

	domainusers "github.com/openmodu/onecatch/internal/domain/users"
	pkgsql "github.com/openmodu/onecatch/pkg/sql"
)

func TestFindOrCreateByIdentityReusesProviderSubject(t *testing.T) {
	repo := NewUsersRepo(nil)
	ctx := context.Background()
	identity := domainusers.AuthIdentity{
		Provider:        "google",
		ProviderSubject: "google-123",
		DisplayName:     "Google User",
		Email:           "google@example.com",
	}

	first, err := repo.FindOrCreateByIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("FindOrCreateByIdentity() error = %v", err)
	}
	second, err := repo.FindOrCreateByIdentity(ctx, identity)
	if err != nil {
		t.Fatalf("FindOrCreateByIdentity() repeat error = %v", err)
	}

	if first.ID == "" {
		t.Fatal("user id is empty")
	}
	if second.ID != first.ID {
		t.Fatalf("repeat user id = %q, want %q", second.ID, first.ID)
	}
}

func TestFindOrCreateByIdentityLinksByRequestedUserID(t *testing.T) {
	repo := NewUsersRepo(nil)
	ctx := context.Background()

	wechat, err := repo.FindOrCreateByIdentity(ctx, domainusers.AuthIdentity{
		UserID:          domainusers.DevUserID,
		Provider:        "wechat",
		ProviderSubject: "local-dev",
		DisplayName:     "Local Developer",
	})
	if err != nil {
		t.Fatalf("wechat identity error = %v", err)
	}

	google, err := repo.FindOrCreateByIdentity(ctx, domainusers.AuthIdentity{
		UserID:          domainusers.DevUserID,
		Provider:        "google",
		ProviderSubject: "local-dev",
		DisplayName:     "Local Developer",
		Email:           "dev@onecatch.local",
	})
	if err != nil {
		t.Fatalf("google identity error = %v", err)
	}

	if wechat.ID != domainusers.DevUserID {
		t.Fatalf("wechat user id = %q, want %q", wechat.ID, domainusers.DevUserID)
	}
	if google.ID != wechat.ID {
		t.Fatalf("google user id = %q, want %q", google.ID, wechat.ID)
	}
}

func TestFindOrCreateByIdentityRequiresProviderSubject(t *testing.T) {
	repo := NewUsersRepo(nil)
	_, err := repo.FindOrCreateByIdentity(context.Background(), domainusers.AuthIdentity{
		Provider: "google",
	})
	if err == nil {
		t.Fatal("FindOrCreateByIdentity() error is nil, want validation error")
	}
}

func TestUsersRepoMySQLSchemaWithDSN(t *testing.T) {
	dsn := os.Getenv("ONECATCH_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("set ONECATCH_MYSQL_TEST_DSN to run users repo MySQL schema test")
	}

	db, err := pkgsql.NewMySQL(dsn)
	if err != nil {
		t.Fatalf("NewMySQL() error = %v", err)
	}
	defer db.Close()

	// Schema is created at startup now, not on the request path.
	if err := Migrate(db.Gorm()); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}

	repo := NewUsersRepo(db)
	ctx := context.Background()
	first, err := repo.FindOrCreateByIdentity(ctx, domainusers.AuthIdentity{
		Provider:        "google",
		ProviderSubject: "mysql-schema-test",
		DisplayName:     "MySQL Schema Test",
		Email:           "mysql-schema-test@onecatch.local",
	})
	if err != nil {
		t.Fatalf("FindOrCreateByIdentity() error = %v", err)
	}
	second, err := repo.FindOrCreateByIdentity(ctx, domainusers.AuthIdentity{
		Provider:        "google",
		ProviderSubject: "mysql-schema-test",
		DisplayName:     "MySQL Schema Test",
		Email:           "mysql-schema-test@onecatch.local",
	})
	if err != nil {
		t.Fatalf("FindOrCreateByIdentity() repeat error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("repeat MySQL user id = %q, want %q", second.ID, first.ID)
	}

	if !db.Gorm().Migrator().HasTable("users") {
		t.Fatal("users table does not exist")
	}
	if !db.Gorm().Migrator().HasTable("auth_identities") {
		t.Fatal("auth_identities table does not exist")
	}
}
