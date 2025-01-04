package utils

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ManeeshWije/watch-together/db"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var googleOauthConfig *oauth2.Config

var userInfo struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

func InitOAuthConfig() {
	googleOauthConfig = &oauth2.Config{
		RedirectURL:  "http://localhost:8080/auth/google/callback",
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}
}

func OauthGoogleLogin(w http.ResponseWriter, r *http.Request) {
	oauthState := generateStateOauthCookie(w)
	u := googleOauthConfig.AuthCodeURL(oauthState, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, r, u, http.StatusTemporaryRedirect)
}

func OauthGoogleCallback(dbConn *sql.DB, w http.ResponseWriter, r *http.Request) {
	data, err := getUserDataFromGoogle(r.FormValue("code"))
	if err != nil {
		log.Printf("Error fetching user data from Google: %v", err)
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}
	json.Unmarshal(data, &userInfo)

	user, err := db.GetUser(dbConn, userInfo.Name, userInfo.Email)
	if err != nil {
		log.Printf("Error retrieving user from database: %v", err)
		return
	}

	if user == nil {
		// No user found, create user and session
		if err := createUserAndSession(dbConn, userInfo, w); err != nil {
			log.Printf("Error creating user or session: %v", err)
		}
		http.Redirect(w, r, "/videos", http.StatusSeeOther)
		return
	}

	// User exists, check session
	session, err := db.GetUserSession(dbConn, user.UUID)
	if err != nil {
		log.Printf("Error retrieving user session: %v", err)
		return
	}

	// If no valid session, create a new one
	if session == nil {
		if err := createSession(dbConn, user.UUID, w); err != nil {
			log.Printf("Error creating session: %v", err)
		}
	} else {
		// If a valid session exists but no cookie, set the cookie
		cookie, err := r.Cookie("auth")
		if err != nil || cookie.Value != session.UUID.String() {
			http.SetCookie(w, &http.Cookie{
				Name:     "auth",
				Value:    session.UUID.String(),
				Expires:  session.ExpiresAt,
				HttpOnly: true,
				SameSite: http.SameSiteLaxMode,
				Path:     "/",
			})
			log.Println("Auth cookie set from valid session in database")
		}
	}
	http.Redirect(w, r, "/videos", http.StatusSeeOther)
}

func generateStateOauthCookie(_ http.ResponseWriter) string {
	b := make([]byte, 16)
	rand.Read(b)
	state := base64.URLEncoding.EncodeToString(b)

	return state
}

func getUserDataFromGoogle(code string) ([]byte, error) {
	// Use code to get token and get user info from Google
	token, err := googleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		return nil, fmt.Errorf("code exchange wrong: %s", err.Error())
	}

	response, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed getting user info: %s", err.Error())
	}
	defer response.Body.Close()
	contents, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed read response: %s", err.Error())
	}

	return contents, nil
}

func createUserAndSession(dbConn *sql.DB, userInfo struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}, w http.ResponseWriter) error {
	userUUID := uuid.New()
	createdAt := time.Now().UTC()
	if err := db.CreateUser(dbConn, userUUID, userInfo.Name, userInfo.Email, createdAt); err != nil {
		return fmt.Errorf("could not create user: %w", err)
	}
	return createSession(dbConn, userUUID, w)
}

func createSession(dbConn *sql.DB, userUUID uuid.UUID, w http.ResponseWriter) error {
	sessionToken := uuid.New()
	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(24 * time.Hour)

	if err := db.CreateUserSession(dbConn, sessionToken, userUUID, createdAt, expiresAt); err != nil {
		return fmt.Errorf("could not create session: %w", err)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    sessionToken.String(),
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	return nil
}

func VerifySession(dbConn *sql.DB, r *http.Request) (bool, *db.UserSession) {
	cookie, err := r.Cookie("auth")
	if err != nil {
		log.Println("No auth cookie found:", err)
		return false, nil
	}

	sessionToken := cookie.Value
	tokenUUID, err := uuid.Parse(sessionToken)
	if err != nil {
		log.Println("Invalid auth cookie value:", err)
		return false, nil
	}

	// Verify the session in the database
	session, err := db.GetUserSessionByToken(dbConn, tokenUUID)
	if err != nil {
		log.Printf("Error retrieving user session from VerifySession: %v", err)
		return false, nil
	}
	if session == nil {
		log.Println("Session not found or expired")
		return false, nil
	}

	return true, session
}
