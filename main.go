package main

import (
	"bytes"
	"crypto/md5"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed json/*
var jsonFiles embed.FS

//go:embed images/*
var imageFiles embed.FS

// loadJSONFiles recursively loads all JSON files from the embedded filesystem
func loadJSONFiles(dir, prefix string, files map[string]*fileData, etags map[string]string) error {
	entries, err := jsonFiles.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fullPath := dir + "/" + entry.Name()

		if entry.IsDir() {
			// Recursively process subdirectories
			newPrefix := prefix + "/" + entry.Name()
			if err := loadJSONFiles(fullPath, newPrefix, files, etags); err != nil {
				return err
			}
		} else if strings.HasSuffix(entry.Name(), ".json") {
			// Load JSON file
			content, err := jsonFiles.ReadFile(fullPath)
			if err != nil {
				log.Printf("Failed to read embedded file %s: %v", fullPath, err)
				continue
			}

			// Create URL path (remove "json" prefix and add leading slash)
			urlPath := prefix + "/" + entry.Name()

			files[urlPath] = &fileData{
				content: content,
				modTime: time.Now(), // Use build time as mod time
			}

			// Calculate ETag
			hash := md5.Sum(content)
			etags[urlPath] = fmt.Sprintf("\"%x\"", hash)
		}
	}

	return nil
}

// loadImageFiles recursively loads all image files from the embedded filesystem
func loadImageFiles(dir, prefix string, files map[string]*fileData, etags map[string]string) error {
	entries, err := imageFiles.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		fullPath := dir + "/" + entry.Name()

		if entry.IsDir() {
			// Recursively process subdirectories
			newPrefix := prefix + "/" + entry.Name()
			if err := loadImageFiles(fullPath, newPrefix, files, etags); err != nil {
				return err
			}
		} else if isImageFile(entry.Name()) {
			// Load image file
			content, err := imageFiles.ReadFile(fullPath)
			if err != nil {
				log.Printf("Failed to read embedded file %s: %v", fullPath, err)
				continue
			}

			// Create URL path (remove "images" prefix and add leading slash)
			urlPath := prefix + "/" + entry.Name()

			files[urlPath] = &fileData{
				content: content,
				modTime: time.Now(), // Use build time as mod time
			}

			// Calculate ETag
			hash := md5.Sum(content)
			etags[urlPath] = fmt.Sprintf("\"%x\"", hash)
		}
	}

	return nil
}

// isImageFile checks if a file has a supported image extension
func isImageFile(filename string) bool {
	extensions := []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico"}
	for _, ext := range extensions {
		if strings.HasSuffix(strings.ToLower(filename), ext) {
			return true
		}
	}
	return false
}

// getContentType returns the appropriate Content-Type header for a file path
func getContentType(path string) string {
	lowerPath := strings.ToLower(path)

	switch {
	case strings.HasSuffix(lowerPath, ".json"):
		return "application/json"
	case strings.HasSuffix(lowerPath, ".png"):
		return "image/png"
	case strings.HasSuffix(lowerPath, ".jpg"), strings.HasSuffix(lowerPath, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lowerPath, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lowerPath, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lowerPath, ".svg"):
		return "image/svg+xml"
	case strings.HasSuffix(lowerPath, ".bmp"):
		return "image/bmp"
	case strings.HasSuffix(lowerPath, ".ico"):
		return "image/x-icon"
	default:
		return ""
	}
}

func main() {
	// Read base FQDN from environment variable, default to coollabs.io
	baseFQDN := os.Getenv("BASE_FQDN")
	if baseFQDN == "" {
		baseFQDN = "coollabs.io"
	}

	// Create a map of embedded files with metadata
	files := make(map[string]*fileData)
	etags := make(map[string]string)

	// Recursively load all JSON files from embedded json directory
	err := loadJSONFiles("json", "", files, etags)
	if err != nil {
		log.Fatal("Failed to load JSON files:", err)
	}

	// Recursively load all image files from embedded images directory
	err = loadImageFiles("images", "", files, etags)
	if err != nil {
		log.Fatal("Failed to load image files:", err)
	}

	log.Printf("Loaded %d files total: %v", len(files), getFileList(files))
	log.Printf("Base FQDN for redirects: %s", baseFQDN)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleRequest(w, r, baseFQDN, files, etags)
	})

	log.Println("Starting server on :80")
	log.Fatal(http.ListenAndServe(":80", nil))
}

func handleRequest(w http.ResponseWriter, r *http.Request, baseFQDN string, files map[string]*fileData, etags map[string]string) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Origin, X-Requested-With, Content-Type, Accept")

	// Handle OPTIONS requests for CORS preflight
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Handle root path redirect
	if r.URL.Path == "/" {
		http.Redirect(w, r, "https://"+baseFQDN, http.StatusFound)
		return
	}

	// Handle health check
	if r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("healthy\n"))
		return
	}

	// Backward compatibility: serve files at old /coolify/ paths
	switch r.URL.Path {
	case "/coolify/versions.json":
		r.URL.Path = "/versions.json"
	case "/coolify/upgrade.sh":
		r.URL.Path = "/upgrade.sh"
	}

	// Redirect /coolify/install.sh to cdn.coolify.io
	if r.URL.Path == "/coolify/install.sh" {
		http.Redirect(w, r, "https://cdn.coolify.io/install.sh", http.StatusMovedPermanently)
		return
	}

	// Check if file exists
	fileData, exists := files[r.URL.Path]
	if !exists {
		// 404 redirect to base FQDN (without path)
		http.Redirect(w, r, "https://"+baseFQDN, http.StatusFound)
		return
	}

	// Set content type based on file extension
	contentType := getContentType(r.URL.Path)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	w.Header().Set("Cache-Control", "public, must-revalidate, max-age=600")

	// Handle ETag caching manually for embedded files
	etag := etags[r.URL.Path]
	w.Header().Set("ETag", etag)

	// Check If-None-Match header
	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		// Include ETag in 304 response as per HTTP spec
		w.Header().Set("ETag", etag)
		return
	}

	// Use http.ServeContent for range request support and Last-Modified handling
	reader := bytes.NewReader(fileData.content)
	http.ServeContent(w, r, filepath.Base(r.URL.Path), fileData.modTime, reader)
}

type fileData struct {
	content []byte
	modTime time.Time
}

func getFileList(files map[string]*fileData) []string {
	var names []string
	for path := range files {
		names = append(names, path)
	}
	return names
}
