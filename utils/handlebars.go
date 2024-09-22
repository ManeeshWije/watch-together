package utils

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aymerick/raymond"
	"github.com/joho/godotenv"
)

var once sync.Once

func Init() {
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}
}

func registerPartials() {
	partials, err := os.ReadDir("views/partials")
	if err == nil {
		for _, partial := range partials {
			if !partial.IsDir() {
				partialName := partial.Name()
				partialContent, _ := os.ReadFile(filepath.Join("views/partials", partialName))
				partialName = partialName[:len(partialName)-len(filepath.Ext(partialName))] // Strip extension
				raymond.RegisterPartial(partialName, string(partialContent))
			}
		}
	}
}

func RenderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	// Register partials only once
	once.Do(registerPartials)

	templatePath := filepath.Join("views", tmpl)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	// Parse the template
	tpl := raymond.MustParse(string(templateContent))

	// Execute the template with the provided data
	result, err := tpl.Exec(data)
	if err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}

	w.Write([]byte(result))
}

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	// Check if the auth cookie exists
	cookieVal, cookieExists := os.LookupEnv("COOKIE_VAL")
	if !cookieExists {
		RenderTemplate(w, "index.hbs", map[string]interface{}{
			"Error": "No cookie env var set",
		})
		return
	}
	cookie, err := r.Cookie("auth")
	if err == nil && cookie.Value == cookieVal {
		// User is authenticated, redirect to /videos
		http.Redirect(w, r, "/videos", http.StatusSeeOther)
		return
	}

	// If no cookie, render the login page
	RenderTemplate(w, "index.hbs", nil)
}

func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth",
		Value:    "",
		Expires:  time.Now().Add(-1 * time.Hour), // Set expiration to the past to delete the cookie
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	// Redirect to the login page or render the login template
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func SubmitHandler(w http.ResponseWriter, r *http.Request) {
	passwordEnv, passExists := os.LookupEnv("PASSWORD")
	cookieVal, cookieExists := os.LookupEnv("COOKIE_VAL")
	if !cookieExists {
		RenderTemplate(w, "partials/loginError.hbs", map[string]interface{}{
			"Error": "No cookie env var set",
		})
		return
	}
	if !passExists {
		RenderTemplate(w, "partials/loginError.hbs", map[string]interface{}{
			"Error": "No password env var set",
		})
		return
	}
	if passwordEnv == r.FormValue("password") {
		http.SetCookie(w, &http.Cookie{
			Name:     "auth",
			Value:    cookieVal,
			Expires:  time.Now().Add(24 * time.Hour),
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/videos", http.StatusSeeOther)
	} else {
		// Wrong password, send a 200 status but indicate the error in the response
		RenderTemplate(w, "partials/loginError.hbs", map[string]interface{}{
			"Error": "Incorrect password.",
		})
	}
}

func ListVideosHandler(w http.ResponseWriter, r *http.Request) {
	s3Client, err := CreateS3Client()
	if err != nil {
		http.Error(w, "Failed to fetch s3 client", http.StatusInternalServerError)
		return
	}
	bucket, exists := os.LookupEnv("AWS_S3_BUCKET")
	if !exists {
		http.Error(w, "Bucket env var not set", http.StatusInternalServerError)
		return
	}
	objects, err := ListObjects(*s3Client, bucket)
	if err != nil {
		http.Error(w, "Failed to list videos", http.StatusInternalServerError)
		return
	}

	RenderTemplate(w, "index.hbs", map[string]interface{}{
		"Authenticated": checkCookie(r),
		"objects":       objects,
	})
}

func checkCookie(r *http.Request) bool {
	cookieVal, cookieExists := os.LookupEnv("COOKIE_VAL")
	if !cookieExists {
		return false
	}

	cookie, err := r.Cookie("auth")
	if err == nil && cookie.Value == cookieVal {
		return true
	}

	return false
}
