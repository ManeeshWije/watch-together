package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type User struct {
	UUID      uuid.UUID
	Username  string
	Email     string
	CreatedAt time.Time
}

func CreateUser(db *sql.DB, uuid uuid.UUID, username string, email string, createdAt time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	sql := `INSERT INTO users (uuid, username, email, created_at)
           VALUES ($1, $2, $3, $4)`

	_, err = tx.Exec(sql, uuid, username, email, createdAt)
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
	err = row.Scan(&user.UUID, &user.Username, &user.Email, &user.CreatedAt)

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
        SELECT u.uuid, u.username, u.email, u.created_at
        FROM users u
        INNER JOIN user_sessions s ON u.uuid = s.user_uuid
        WHERE s.uuid = $1 AND s.expires_at > $2
    `

	row := db.QueryRow(query, sessionID, time.Now().UTC())

	var user User
	if err := row.Scan(&user.UUID, &user.Username, &user.Email, &user.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No user found for the session
		}
		return nil, fmt.Errorf("failed to get user by session: %v", err)
	}

	return &user, nil
}
