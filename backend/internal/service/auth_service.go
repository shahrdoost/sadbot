package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"sadbot/internal/domain"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type AdminRepository interface {
	Create(ctx context.Context, email, passwordHash, displayName, role string) (*domain.Admin, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Admin, error)
	GetByEmail(ctx context.Context, email string) (*domain.Admin, error)
	List(ctx context.Context) ([]domain.Admin, error)
	Update(ctx context.Context, id uuid.UUID, displayName, role, status *string) (*domain.Admin, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	Delete(ctx context.Context, id uuid.UUID) error
	Count(ctx context.Context) (int64, error)
	GetBotAccess(ctx context.Context, adminID, botID uuid.UUID) (*domain.BotAccess, error)
	ListBotAccessByAdmin(ctx context.Context, adminID uuid.UUID) ([]domain.BotAccess, error)
	UpsertBotAccess(ctx context.Context, adminID, botID uuid.UUID, perms []string, grantedBy *uuid.UUID) (*domain.BotAccess, error)
	HasBotPermission(ctx context.Context, adminID, botID uuid.UUID, perm string) (bool, error)
}

type AuditLogger interface {
	Insert(ctx context.Context, actorAdminID *uuid.UUID, action, targetType, targetID string, botID *uuid.UUID, details map[string]any, ip *string) error
}

type AuthService struct {
	admins AdminRepository
	jwt    []byte
}

func NewAuthService(admins AdminRepository, jwtSecret string) *AuthService {
	return &AuthService{admins: admins, jwt: []byte(jwtSecret)}
}

type TokenClaims struct {
	AdminID uuid.UUID `json:"aid"`
	Role    string    `json:"role"`
	jwt.RegisteredClaims
}

func (s *AuthService) Login(ctx context.Context, email, password string) (string, *domain.Admin, error) {
	admin, err := s.admins.GetByEmail(ctx, email)
	if err != nil {
		return "", nil, errors.New("ایمیل یا رمز عبور نادرست است")
	}
	if admin.Status != domain.AdminStatusActive {
		return "", nil, errors.New("حساب غیرفعال است")
	}
	if bcrypt.CompareHashAndPassword([]byte(admin.PasswordHash), []byte(password)) != nil {
		return "", nil, errors.New("ایمیل یا رمز عبور نادرست است")
	}
	token, err := s.issueToken(admin)
	if err != nil {
		return "", nil, err
	}
	return token, admin, nil
}

func (s *AuthService) issueToken(admin *domain.Admin) (string, error) {
	claims := TokenClaims{
		AdminID: admin.ID,
		Role:    admin.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   admin.ID.String(),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwt)
}

func (s *AuthService) ParseToken(token string) (*TokenClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &TokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.jwt, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*TokenClaims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// ParseClaims loads the fresh admin record for a parsed token.
func (s *AuthService) ParseClaims(ctx context.Context, claims *TokenClaims) (*domain.Admin, error) {
	return s.admins.GetByID(ctx, claims.AdminID)
}

func HashPassword(password string) (string, error) {
	if len(password) < 8 {
		return "", errors.New("password must be at least 8 characters")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}
