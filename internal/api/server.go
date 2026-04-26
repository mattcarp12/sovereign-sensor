package api

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"

	"sigs.k8s.io/controller-runtime/pkg/client"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1" // Update to your module path
)

// We tell Go to embed the compiled React assets.
// NOTE: This requires you to run 'npm run build' in the frontend folder before compiling Go.
//
//go:embed dist/*
var frontendAssets embed.FS

type Server struct {
	Client client.Client
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// 1. API Endpoints
	mux.HandleFunc("/api/policies", s.handlePolicies)

	// 2. Serve the Embedded React App
	// We extract the 'dist' folder from the embedded filesystem
	subFS, err := fs.Sub(frontendAssets, "dist")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	return http.ListenAndServe(addr, mux)
}

// handlePolicies queries the Kubernetes API directly for all active policies
func (s *Server) handlePolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		var policyList secv1alpha1.SovereigntyPolicyList

		// Query the K8s API directly across all namespaces
		if err := s.Client.List(context.Background(), &policyList); err != nil {
			http.Error(w, "Failed to fetch policies from Kubernetes", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		// We return the .Items array, which is the raw list of policies
		json.NewEncoder(w).Encode(policyList.Items)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}
