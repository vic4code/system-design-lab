package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Admin struct {
	db *pgxpool.Pool
}

func NewAdmin(db *pgxpool.Pool) *Admin {
	return &Admin{db: db}
}

// ListUsers godoc  GET /v1/admin/users
func (h *Admin) ListUsers(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, email, name, role, terms_version, deleted_at, created_at
		 FROM users ORDER BY created_at DESC LIMIT 200`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to list users"))
		return
	}
	defer rows.Close()

	type adminUserView struct {
		ID           string     `json:"id"`
		Email        string     `json:"email"`
		Name         string     `json:"name"`
		Role         string     `json:"role"`
		TermsVersion int        `json:"terms_version"`
		DeletedAt    *time.Time `json:"deleted_at,omitempty"`
		CreatedAt    time.Time  `json:"created_at"`
	}

	var users []adminUserView
	for rows.Next() {
		var u adminUserView
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Role, &u.TermsVersion, &u.DeletedAt, &u.CreatedAt); err == nil {
			users = append(users, u)
		}
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "count": len(users)})
}

// DeleteUser godoc  DELETE /v1/admin/users/:id — hard delete (removes PII + account)
func (h *Admin) DeleteUser(c *gin.Context) {
	id := c.Param("id")
	tag, err := h.db.Exec(c.Request.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil || tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, apiError("user not found"))
		return
	}
	c.Status(http.StatusNoContent)
}

// ListAuditLogs godoc  GET /v1/admin/audit-logs
func (h *Admin) ListAuditLogs(c *gin.Context) {
	rows, err := h.db.Query(c.Request.Context(),
		`SELECT id, user_id, action, resource_type, resource_id, ip_address, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT 500`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to list audit logs"))
		return
	}
	defer rows.Close()

	type auditEntry struct {
		ID           string     `json:"id"`
		UserID       *string    `json:"user_id"`
		Action       string     `json:"action"`
		ResourceType string     `json:"resource_type"`
		ResourceID   *string    `json:"resource_id"`
		IPAddress    *string    `json:"ip_address"`
		CreatedAt    time.Time  `json:"created_at"`
	}

	var logs []auditEntry
	for rows.Next() {
		var e auditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.ResourceType, &e.ResourceID, &e.IPAddress, &e.CreatedAt); err == nil {
			logs = append(logs, e)
		}
	}
	c.JSON(http.StatusOK, gin.H{"audit_logs": logs, "count": len(logs)})
}
