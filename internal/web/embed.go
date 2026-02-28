package web

import (
	"embed"
	"net/http"
	"path/filepath"
)

//go:embed templates
var content embed.FS

func GetTemplates() *embed.FS {
	return &content
}

func ServeTemplate(w http.ResponseWriter, templateName string) error {
	templateContent, err := content.ReadFile("templates/" + templateName)
	if err != nil {
		return err
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	}
	if w.Header().Get("Cache-Control") == "" {
		// Templates are shared shells and do not include user-private data server-side.
		w.Header().Set("Cache-Control", "public, max-age=120")
	}
	_, err = w.Write(templateContent)
	return err
}

func ServeStatic(w http.ResponseWriter, staticPath string) error {
	staticContent, err := content.ReadFile("templates" + staticPath)
	if err != nil {
		return err
	}
	ext := filepath.Ext(staticPath)
	switch ext {
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".woff", ".woff2":
		w.Header().Set("Content-Type", "font/"+ext[1:])
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".eot":
		w.Header().Set("Content-Type", "application/vnd.ms-fontobject")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".txt":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case ".xml":
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	}
	if w.Header().Get("Cache-Control") == "" {
		switch {
		case staticPath == "/sw.js" || staticPath == "/manifest.json":
			// Keep worker and manifest fresh so clients update quickly after deploy.
			w.Header().Set("Cache-Control", "no-cache")
		case ext == ".js" || ext == ".css":
			w.Header().Set("Cache-Control", "public, max-age=3600")
		case ext == ".woff" || ext == ".woff2" || ext == ".ttf" || ext == ".eot" || ext == ".svg" || ext == ".png" || ext == ".ico":
			w.Header().Set("Cache-Control", "public, max-age=604800")
		default:
			w.Header().Set("Cache-Control", "public, max-age=300")
		}
	}
	_, err = w.Write(staticContent)
	return err
}
