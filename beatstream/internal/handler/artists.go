package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Artists struct {
	db *pgxpool.Pool
}

func NewArtists(db *pgxpool.Pool) *Artists {
	return &Artists{db: db}
}

type artistRow struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

func (h *Artists) Create(c *gin.Context) {
	var body struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, apiError("name is required"))
		return
	}

	var a artistRow
	err := h.db.QueryRow(c.Request.Context(),
		`INSERT INTO artists (name) VALUES ($1) RETURNING id, name, created_at`,
		body.Name).Scan(&a.ID, &a.Name, &a.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, apiError("failed to create artist"))
		return
	}
	c.JSON(http.StatusCreated, a)
}

func (h *Artists) Get(c *gin.Context) {
	id := c.Param("id")
	var a artistRow
	err := h.db.QueryRow(c.Request.Context(),
		`SELECT id, name, created_at FROM artists WHERE id = $1`, id).
		Scan(&a.ID, &a.Name, &a.CreatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, apiError("artist not found"))
		return
	}
	c.JSON(http.StatusOK, a)
}
