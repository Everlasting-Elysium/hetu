package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Everlasting-Elysium/hetu/internal/domain"
	"github.com/Everlasting-Elysium/hetu/internal/store/db"
)

// CreateShare inserts a new share link. The token must be unique; a duplicate
// surfaces as the driver's UNIQUE constraint error.
func (s *SQLite) CreateShare(ctx context.Context, sh domain.Share) error {
	if err := s.q.CreateShare(ctx, db.CreateShareParams{
		ID:           sh.ID.String(),
		OwnerID:      sh.Owner.String(),
		TargetType:   sh.TargetType,
		TargetID:     sh.TargetID,
		Token:        sh.Token,
		ExpiresAt:    timeToNullUnix(sh.ExpiresAt),
		PasswordHash: sh.PasswordHash,
		Permission:   sh.Permission,
		CreatedAt:    sh.CreatedAt.Unix(),
	}); err != nil {
		return fmt.Errorf("create share %s: %w", sh.ID, err)
	}
	return nil
}

// GetShareByToken resolves a share by its public token, or domain.ErrNotFound.
func (s *SQLite) GetShareByToken(ctx context.Context, token string) (domain.Share, error) {
	row, err := s.q.GetShareByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Share{}, fmt.Errorf("get share by token: %w", domain.ErrNotFound)
		}
		return domain.Share{}, fmt.Errorf("get share by token: %w", err)
	}
	return rowToShare(row)
}

func rowToShare(r db.Share) (domain.Share, error) {
	id, err := domain.NewShareID(r.ID)
	if err != nil {
		return domain.Share{}, fmt.Errorf("row share id: %w", err)
	}
	owner, err := domain.NewOwnerID(r.OwnerID)
	if err != nil {
		return domain.Share{}, fmt.Errorf("row share owner: %w", err)
	}
	return domain.Share{
		ID:           id,
		Owner:        owner,
		TargetType:   r.TargetType,
		TargetID:     r.TargetID,
		Token:        r.Token,
		ExpiresAt:    nullUnixToTime(r.ExpiresAt),
		PasswordHash: r.PasswordHash,
		Permission:   r.Permission,
		CreatedAt:    time.Unix(r.CreatedAt, 0).UTC(),
	}, nil
}
