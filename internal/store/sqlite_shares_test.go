package store_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
)

func mkShare(t *testing.T, owner domain.OwnerID, id, token string, expires *time.Time) domain.Share {
	t.Helper()
	sid, err := domain.NewShareID(id)
	if err != nil {
		t.Fatal(err)
	}
	return domain.Share{
		ID: sid, Owner: owner, TargetType: "asset", TargetID: "a1",
		Token: token, ExpiresAt: expires, PasswordHash: "hash", Permission: "read",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}
}

func TestSQLite_ShareCreateAndGetByToken(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	exp := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	want := mkShare(t, owner, "s1", "tok-abc", &exp)
	if err := st.CreateShare(ctx, want); err != nil {
		t.Fatalf("create share: %v", err)
	}

	got, err := st.GetShareByToken(ctx, "tok-abc")
	if err != nil {
		t.Fatalf("get share: %v", err)
	}
	if got.ID != want.ID || got.Owner != owner || got.Token != "tok-abc" ||
		got.TargetType != "asset" || got.TargetID != "a1" ||
		got.PasswordHash != "hash" || got.Permission != "read" {
		t.Fatalf("share mismatch: %+v", got)
	}
	if got.ExpiresAt == nil || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, exp)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("created_at = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
}

func TestSQLite_ShareNeverExpires(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	if err := st.CreateShare(ctx, mkShare(t, owner, "s2", "tok-noexp", nil)); err != nil {
		t.Fatalf("create share: %v", err)
	}
	got, err := st.GetShareByToken(ctx, "tok-noexp")
	if err != nil {
		t.Fatalf("get share: %v", err)
	}
	if got.ExpiresAt != nil {
		t.Fatalf("expires_at = %v, want nil (never expires)", got.ExpiresAt)
	}
}

func TestSQLite_ShareGetByTokenNotFound(t *testing.T) {
	ctx, st, _ := mustOpen(t)
	_, err := st.GetShareByToken(ctx, "missing")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want domain.ErrNotFound", err)
	}
}

func TestSQLite_ShareTokenUnique(t *testing.T) {
	ctx, st, owner := mustOpen(t)
	if err := st.CreateShare(ctx, mkShare(t, owner, "s3", "dup", nil)); err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := st.CreateShare(ctx, mkShare(t, owner, "s4", "dup", nil)); err == nil {
		t.Fatal("create second with duplicate token: want error, got nil")
	}
}
