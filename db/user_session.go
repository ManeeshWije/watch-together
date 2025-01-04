package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type UserSession struct {
	UUID      uuid.UUID
	UserUUID  uuid.UUID
	CreatedAt time.Time
	ExpiresAt time.Time
}

func CreateUserSession(db *sql.DB, uuid uuid.UUID, userUuid uuid.UUID, createdAt time.Time, expiresAt time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	sql := `INSERT INTO user_sessions (uuid, user_uuid, created_at, expires_at)
           VALUES ($1, $2, $3, $4)`

	_, err = tx.Exec(sql, uuid, userUuid, createdAt, expiresAt)
	if err != nil {
		return fmt.Errorf("failed to create user session: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func GetUserSession(db *sql.DB, userUuid uuid.UUID) (*UserSession, error) {
    query := `SELECT uuid, user_uuid, created_at, expires_at FROM user_sessions WHERE user_uuid = $1 AND expires_at > $2`
	rows, err := db.Query(query, userUuid, time.Now().UTC())
	if err != nil {
		return nil, fmt.Errorf("failed to query user sessions: %v", err)
	}
	defer rows.Close()

	var validSession *UserSession
	for rows.Next() {
		var session UserSession
		if err := rows.Scan(&session.UUID, &session.UserUUID, &session.CreatedAt, &session.ExpiresAt); err != nil {
			return nil, fmt.Errorf("failed to scan user session: %v", err)
		}
		// If a valid session is found, return it immediately
		validSession = &session
		break
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating through user sessions: %v", err)
	}

	return validSession, nil
}

func GetUserSessionByToken(db *sql.DB, token uuid.UUID) (*UserSession, error) {
	query := `SELECT uuid, user_uuid, created_at, expires_at 
              FROM user_sessions 
              WHERE uuid = $1 AND expires_at > $2`

	row := db.QueryRow(query, token, time.Now().UTC())

	var session UserSession
	if err := row.Scan(&session.UUID, &session.UserUUID, &session.CreatedAt, &session.ExpiresAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to retrieve session: %v", err)
	}

	return &session, nil
}

func DeleteUserSessionByToken(db *sql.DB, token uuid.UUID) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	query := `DELETE FROM user_sessions WHERE uuid = $1`

	result, err := tx.Exec(query, token)
	if err != nil {
		return fmt.Errorf("failed to delete user session: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to retrieve rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no session found with the provided token")
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}
