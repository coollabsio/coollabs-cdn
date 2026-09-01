package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJeanModelCatalogIncludesFable51(t *testing.T) {
	content, err := jsonFiles.ReadFile("json/jean/models.json")
	if err != nil {
		t.Fatal(err)
	}

	var catalog struct {
		Backends struct {
			Claude struct {
				Models []struct {
					ID    string `json:"id"`
					Label string `json:"label"`
				} `json:"models"`
			} `json:"claude"`
		} `json:"backends"`
	}
	if err := json.Unmarshal(content, &catalog); err != nil {
		t.Fatal(err)
	}

	for _, model := range catalog.Backends.Claude.Models {
		if model.ID == "claude-fable-5-1" && model.Label == "Claude Fable 5.1" {
			return
		}
	}
	t.Fatal("expected Claude Fable 5.1 in Jean model catalog")
}

func TestLoadJSONFilesIncludesCoolifyArtifacts(t *testing.T) {
	files := make(map[string]*fileData)
	etags := make(map[string]string)

	if err := loadJSONFiles("json", "", files, etags); err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{
		"/coolify/install.sh",
		"/coolify/.env.production",
		"/coolify/docker-compose.yml",
		"/coolify/nightly/install.sh",
	} {
		if _, exists := files[path]; !exists {
			t.Errorf("expected embedded file %s to be loaded", path)
		}
	}
}

func TestCoolifyRoutesServeLocalFiles(t *testing.T) {
	files := map[string]*fileData{
		"/coolify/install.sh":         {content: []byte("stable-local"), modTime: time.Now()},
		"/coolify/nightly/install.sh": {content: []byte("nightly-local"), modTime: time.Now()},
	}
	etags := map[string]string{
		"/coolify/install.sh":         `"stable"`,
		"/coolify/nightly/install.sh": `"nightly"`,
	}

	tests := []struct {
		path string
		want string
	}{
		{path: "/coolify/install.sh", want: "stable-local"},
		{path: "/coolify-nightly/install.sh", want: "nightly-local"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)

			handleRequest(recorder, request, "coollabs.io", files, etags)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d", recorder.Code)
			}
			if body := recorder.Body.String(); body != tt.want {
				t.Fatalf("expected local body %q, got %q", tt.want, body)
			}
		})
	}
}
