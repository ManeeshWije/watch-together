package db

import (
	"database/sql"
	"fmt"
	"time"
)

type Video struct {
	URL       string
	Title     string
	CreatedAt time.Time
}

func ListVideos(db *sql.DB) ([]Video, error) {
	query := `SELECT url, title, created_at FROM videos`
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query videos: %v", err)
	}
	defer rows.Close()

	var videos []Video
	for rows.Next() {
		var video Video
		if err := rows.Scan(&video.URL, &video.Title, &video.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan video: %v", err)
		}
		videos = append(videos, video)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %v", err)
	}

	return videos, nil
}

func GetVideo(db *sql.DB, url, title string) (*Video, error) {
	query := `SELECT url, title, created_at FROM videos WHERE url = $1 AND title = $2`
	row := db.QueryRow(query, url, title)

	var video Video
	if err := row.Scan(&video.URL, &video.Title, &video.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // No video found
		}
		return nil, fmt.Errorf("failed to scan video: %v", err)
	}

	return &video, nil
}

func DeleteVideo(db *sql.DB, url string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	query := `DELETE FROM videos WHERE url = $1`
	result, err := tx.Exec(query, url)
	if err != nil {
		return fmt.Errorf("failed to delete video: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %v", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no video found with the given URL")
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}

func CreateVideo(db *sql.DB, url, title string, createdAt time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback()

	query := `INSERT INTO videos (url, title, created_at) VALUES ($1, $2, $3)`
	_, err = tx.Exec(query, url, title, createdAt)
	if err != nil {
		return fmt.Errorf("failed to insert video: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	return nil
}
