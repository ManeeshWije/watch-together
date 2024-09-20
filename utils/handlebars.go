package utils

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

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
	RenderTemplate(w, "index.hbs", nil)
}

func SubmitHandler(w http.ResponseWriter, r *http.Request) {
	passwordEnv, exists := os.LookupEnv("PASSWORD")
	if !exists {
		RenderTemplate(w, "partials/loginError.hbs", map[string]interface{}{
			"Error": "No password set in the environment.",
		})
		return
	}

	if passwordEnv == r.FormValue("password") {
		RenderTemplate(w, "partials/video.hbs", nil)
	} else {
		// Wrong password, send a 200 status but indicate the error in the response
		RenderTemplate(w, "partials/loginError.hbs", map[string]interface{}{
			"Error": "Incorrect password.",
		})
	}
}
