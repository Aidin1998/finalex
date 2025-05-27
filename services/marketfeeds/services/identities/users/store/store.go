package store

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/litebittech/cex/internal/identities/models"
	"github.com/litebittech/cex/pkg/dbutil"
	"github.com/litebittech/cex/pkg/errors"
	"gorm.io/gorm"
)

// Input structs for more complex user operations
type CreateIn struct {
	ID         uuid.UUID
	ProviderID string
	Username   string
	Email      string
	FirstName  string
	LastName   string
}

type UpdateIn struct {
	Username  string
	Email     string
	FirstName string
	LastName  string
}

type Store interface {
	Create(ctx context.Context, in CreateIn) error
	User(ctx context.Context, id uuid.UUID) (*models.User, error)
	UserByProviderID(ctx context.Context, providerId string) (*models.User, error)
	UserByUsername(ctx context.Context, username string) (*models.User, error)
	UserByEmail(ctx context.Context, email string) (*models.User, error)
	List(ctx context.Context, filter models.UserFilter) ([]models.User, error)
	Update(ctx context.Context, id uuid.UUID, in UpdateIn) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type StoreImp struct {
	log *slog.Logger
	db  *gorm.DB
}

var _ Store = (*StoreImp)(nil)

func New(log *slog.Logger, db *gorm.DB) *StoreImp {
	return &StoreImp{log, db}
}

func (s *StoreImp) Create(ctx context.Context, in CreateIn) error {
	now := time.Now()
	user := &models.User{
		ID:         in.ID,
		Username:   in.Username,
		Email:      in.Email,
		FirstName:  in.FirstName,
		ProviderID: in.ProviderID,
		LastName:   in.LastName,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	result := s.db.WithContext(ctx).Create(user)
	if result.Error != nil {
		// Check for unique constraint violations
		if result.Error.Error() == "ERROR: duplicate key value violates unique constraint" {
			return errors.Conflict.Explain("user already exists")
		}

		return errors.New("failed to create user").Wrap(result.Error)
	}

	return nil
}

func (s *StoreImp) User(ctx context.Context, id uuid.UUID) (*models.User, error) {
	if id == uuid.Nil {
		return nil, errors.Invalid.Explain("user ID is required")
	}

	var user models.User
	result := s.db.WithContext(ctx).Where("id = ?", id).First(&user)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, errors.NotFound.Explain("user not found")
		}

		return nil, errors.New("failed to get user by ID").Wrap(result.Error)
	}

	return &user, nil
}

func (s *StoreImp) UserByUsername(ctx context.Context, username string) (*models.User, error) {
	if username == "" {
		return nil, errors.Invalid.Explain("username is required")
	}

	user, err := dbutil.FindOne[models.User](s.db.WithContext(ctx).Where("username = ?", username))
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, errors.NotFound.Explain("user not found")
		}
		return nil, errors.New("failed to get user by username").Wrap(err)
	}

	return user, nil
}

func (s *StoreImp) UserByEmail(ctx context.Context, email string) (*models.User, error) {
	if email == "" {
		return nil, errors.Invalid.Explain("email is required")
	}

	user, err := dbutil.FindOne[models.User](s.db.WithContext(ctx).Where("email = ?", email))
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			return nil, errors.NotFound.Explain("user not found")
		}
		return nil, errors.New("failed to get user by email").Wrap(err)
	}

	return user, nil
}

func (s *StoreImp) UserByProviderID(ctx context.Context, providerID string) (*models.User, error) {
	if providerID == "" {
		return nil, errors.Invalid.Explain("provider ID is required")
	}

	user, err := dbutil.FindOne[models.User](s.db.WithContext(ctx).Where("provider_id = ?", providerID))
	if err != nil {
		return nil, errors.New("failed to get user by provider ID").Wrap(err)
	}

	return user, nil
}

func (s *StoreImp) List(ctx context.Context, filter models.UserFilter) ([]models.User, error) {
	// Set default values if not provided
	limit := filter.Limit
	if limit <= 0 {
		limit = 10 // Default limit
	}

	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Build the query
	query := s.db.WithContext(ctx)

	// Apply created_before filter if provided
	if filter.CreatedBefore != nil {
		query = query.Where("created_at <= ?", filter.CreatedBefore)
	}

	// Apply created_after filter if provided
	if filter.CreatedAfter != nil {
		query = query.Where("created_at >= ?", filter.CreatedAfter)
	}

	var users []models.User
	result := query.Limit(limit).Offset(offset).Find(&users)
	if result.Error != nil {
		return nil, errors.New("failed to list users").Wrap(result.Error)
	}

	return users, nil
}

func (s *StoreImp) Update(ctx context.Context, id uuid.UUID, in UpdateIn) error {
	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if in.Username != "" {
		updates["username"] = in.Username
	}

	if in.Email != "" {
		updates["email"] = in.Email
	}

	if in.FirstName != "" {
		updates["first_name"] = in.FirstName
	}
	if in.LastName != "" {
		updates["last_name"] = in.LastName
	}

	err := s.db.WithContext(ctx).Model(&models.User{ID: id}).Updates(updates).Error
	if err != nil {
		return errors.New("failed to update user").Wrap(err)
	}

	return nil
}

func (s *StoreImp) Delete(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return errors.Invalid.Explain("user ID is required")
	}

	result := s.db.WithContext(ctx).Delete(&models.User{}, "id = ?", id)
	if result.Error != nil {
		return errors.New("failed to delete user").Wrap(result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.NotFound.Explain("user not found")
	}

	return nil
}
