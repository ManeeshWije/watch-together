package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type User struct {
	UUID       uuid.UUID
	Username   string
	Email      string
	CreatedAt  time.Time
	NumUploads int
	IsAdmin    bool
}

func CreateUser(db *sql.DB, uuid uuid.UUID, username string, email string, createdAt time.Time, numUploads int, isAdmin bool) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	sql := `INSERT INTO users (uuid, username, email, created_at, num_uploads, is_admin)
           VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = tx.Exec(sql, uuid, username, email, createdAt, numUploads, isAdmin)
	if err != nil {
		return fmt.Errorf("failed to insert user: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func GetUser(db *sql.DB, username string, email string) (*User, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	query := `SELECT * from users WHERE username = $1 AND email = $2`

	row := tx.QueryRow(query, username, email)

	var user User
	err = row.Scan(&user.UUID, &user.Username, &user.Email, &user.CreatedAt, &user.NumUploads, &user.IsAdmin)

	if err == sql.ErrNoRows {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("failed to scan user: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return &user, nil
}

func GetUserBySession(db *sql.DB, sessionID uuid.UUID) (*User, error) {
	query := `
        SELECT u.uuid, u.username, u.email, u.created_at, u.num_uploads, u.is_admin
        FROM users u
        INNER JOIN user_sessions s ON u.uuid = s.user_uuid
        WHERE s.uuid = $1 AND s.expires_at > $2
    `

	row := db.QueryRow(query, sessionID, time.Now().UTC())

	var user User
	if err := row.Scan(&user.UUID, &user.Username, &user.Email, &user.CreatedAt, &user.NumUploads, &user.IsAdmin); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No user found for the session
		}
		return nil, fmt.Errorf("failed to get user by session: %v", err)
	}

	return &user, nil
}

func IncrementUploads(db *sql.DB, userID uuid.UUID) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	query := `SELECT num_uploads, is_admin FROM users WHERE uuid = $1`
	var numUploads int
	var isAdmin bool
	err = tx.QueryRow(query, userID).Scan(&numUploads, &isAdmin)
	if err == sql.ErrNoRows {
		return false, errors.New("user not found")
	}
	if err != nil {
		return false, fmt.Errorf("failed to retrieve user data: %v", err)
	}

	// If the user is an admin, allow the upload without incrementing
	if isAdmin {
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("failed to commit transaction for admin user: %v", err)
		}
		return true, nil
	}

	// If num_uploads is already at 5, deny the upload without incrementing
	if numUploads >= 5 {
		return false, nil
	}

	// Increment num_uploads
	updateQuery := `UPDATE users SET num_uploads = num_uploads + 1 WHERE uuid = $1`
	_, err = tx.Exec(updateQuery, userID)
	if err != nil {
		return false, fmt.Errorf("failed to increment num_uploads: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("failed to commit transaction: %v", err)
	}

	return true, nil
}
