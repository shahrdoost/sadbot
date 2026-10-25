package service

import (
	"context"
	"fmt"

	"sadbot/internal/domain"

	"github.com/google/uuid"
)

type AdminService struct {
	admins AdminRepository
	audit  AuditLogger
}

func NewAdminService(admins AdminRepository, audit AuditLogger) *AdminService {
	return &AdminService{admins: admins, audit: audit}
}

func (s *AdminService) CreateAdmin(ctx context.Context, actor *domain.Admin, email, password, displayName, role string, ip *string) (*domain.Admin, error) {
	if !actor.IsSuperAdmin() {
		return nil, fmt.Errorf("forbidden")
	}
	if role != domain.RoleAdmin && role != domain.RoleSuperAdmin {
		return nil, fmt.Errorf("invalid role")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	a, err := s.admins.Create(ctx, email, hash, displayName, role)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "admin.create", "admin", a.ID.String(), nil, map[string]any{"email": email, "role": role}, ip)
	return a, nil
}

func (s *AdminService) ListAdmins(ctx context.Context, actor *domain.Admin) ([]domain.Admin, error) {
	if !actor.IsSuperAdmin() {
		return nil, fmt.Errorf("forbidden")
	}
	return s.admins.List(ctx)
}

func (s *AdminService) UpdateAdmin(ctx context.Context, actor *domain.Admin, id uuid.UUID, displayName, role, status *string, ip *string) (*domain.Admin, error) {
	if !actor.IsSuperAdmin() {
		return nil, fmt.Errorf("forbidden")
	}
	a, err := s.admins.Update(ctx, id, displayName, role, status)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "admin.update", "admin", id.String(), nil, map[string]any{"display_name": displayName, "role": role, "status": status}, ip)
	return a, nil
}

func (s *AdminService) DeleteAdmin(ctx context.Context, actor *domain.Admin, id uuid.UUID, ip *string) error {
	if !actor.IsSuperAdmin() {
		return fmt.Errorf("forbidden")
	}
	if id == actor.ID {
		return fmt.Errorf("cannot delete yourself")
	}
	if err := s.admins.Delete(ctx, id); err != nil {
		return err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "admin.delete", "admin", id.String(), nil, nil, ip)
	return nil
}

func (s *AdminService) SetBotAccess(ctx context.Context, actor *domain.Admin, adminID, botID uuid.UUID, perms []string, ip *string) (*domain.BotAccess, error) {
	if !actor.IsSuperAdmin() {
		return nil, fmt.Errorf("forbidden")
	}
	for _, p := range perms {
		if !domain.ValidPermission(p) {
			return nil, fmt.Errorf("invalid permission: %s", p)
		}
	}
	acc, err := s.admins.UpsertBotAccess(ctx, adminID, botID, perms, &actor.ID)
	if err != nil {
		return nil, err
	}
	_ = s.audit.Insert(ctx, &actor.ID, "admin.bot_access", "admin", adminID.String(), &botID, map[string]any{"permissions": perms}, ip)
	return acc, nil
}

func (s *AdminService) GetBotAccess(ctx context.Context, actor *domain.Admin, adminID uuid.UUID) ([]domain.BotAccess, error) {
	if !actor.IsSuperAdmin() {
		return nil, fmt.Errorf("forbidden")
	}
	return s.admins.ListBotAccessByAdmin(ctx, adminID)
}

// Can checks whether an admin may perform an action on a bot.
func (s *AdminService) Can(ctx context.Context, admin *domain.Admin, botID uuid.UUID, perm string) (bool, error) {
	if admin.IsSuperAdmin() {
		return true, nil
	}
	return s.admins.HasBotPermission(ctx, admin.ID, botID, perm)
}
