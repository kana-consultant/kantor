package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"net"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Entry struct {
	UserID     string
	Action     string
	Module     string
	Resource   string
	ResourceID string
	OldValue   interface{}
	NewValue   interface{}
	IPAddress  string
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Insert(ctx context.Context, entry Entry) error {
	oldJSON, err := marshalNullableJSON(entry.OldValue)
	if err != nil {
		return fmt.Errorf("marshal old_value: %w", err)
	}

	newJSON, err := marshalNullableJSON(entry.NewValue)
	if err != nil {
		return fmt.Errorf("marshal new_value: %w", err)
	}

	var ipAddr *net.IP
	if entry.IPAddress != "" {
		parsed := net.ParseIP(entry.IPAddress)
		if parsed != nil {
			ipAddr = &parsed
		}
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO audit_logs (user_id, action, module, resource, resource_id, old_value, new_value, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, entry.UserID, entry.Action, entry.Module, entry.Resource, entry.ResourceID, oldJSON, newJSON, ipAddr)
	if err != nil {
		return fmt.Errorf("insert audit log: %w", err)
	}

	return nil
}

func marshalNullableJSON(v interface{}) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
