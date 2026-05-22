package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strings"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// We tell Go to embed the compiled React assets.
// NOTE: This requires you to run 'npm run build' in the frontend folder before compiling Go.
//
//go:embed dist/*
var frontendAssets embed.FS

type Server struct {
	Client    client.Client
	Clientset kubernetes.Interface
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// 1. API Endpoints
	mux.HandleFunc("GET /api/policies", listPolicies(s.Client))
	mux.HandleFunc("POST /api/policies", createPolicy(s.Client))

	// FIX: Added {name} wildcard to the DELETE route
	mux.HandleFunc("DELETE /api/policies/{name}", deletePolicy(s.Client))

	// NEW: Added PUT route for updates
	mux.HandleFunc("PUT /api/policies/{name}", updatePolicy(s.Client))

	mux.HandleFunc("GET /api/violations", s.handleViolations)

	// 2. Serve the Embedded React App
	subFS, err := fs.Sub(frontendAssets, "dist")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	return http.ListenAndServe(addr, mux)
}

// 1. GET /api/policies - List all SovereigntyPolicies
func listPolicies(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var policies secv1alpha1.SovereigntyPolicyList
		if err := c.List(r.Context(), &policies); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(policies.Items); err != nil {
			log.Printf("failed to encode policies: %v", err)
		}
	}
}

// POST /api/policies
func createPolicy(c client.Client) http.HandlerFunc {
	nameRegex := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Country   string `json:"country"`
			Action    string `json:"action"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		req.Name = strings.TrimSpace(req.Name)
		if !nameRegex.MatchString(req.Name) {
			http.Error(w, "Invalid name: must be lowercase alphanumeric and hyphens only", http.StatusBadRequest)
			return
		}
		if req.Namespace == "" {
			req.Namespace = "default"
		}
		if len(req.Country) != 2 {
			http.Error(w, "Invalid country code: must be exactly 2 characters (ISO Alpha-2)", http.StatusBadRequest)
			return
		}
		req.Country = strings.ToUpper(req.Country)
		if req.Action != "block-kill" && req.Action != "block-noconn" && req.Action != "log" {
			http.Error(w, "Invalid action: must be block-kill, block-noconn, or log", http.StatusBadRequest)
			return
		}

		policy := &secv1alpha1.SovereigntyPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: req.Name},
			Spec: secv1alpha1.SovereigntyPolicySpec{
				Namespaces:          []string{req.Namespace},
				DisallowedCountries: []string{req.Country},
				Actions:             []secv1alpha1.Action{secv1alpha1.Action(req.Action)},
				Description:         "Generated via Sovereign Sensor Web UI",
			},
		}

		if err := c.Create(r.Context(), policy); err != nil {
			http.Error(w, "Failed to create policy in cluster: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// DELETE /api/policies/{name}
func deletePolicy(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "Policy name is required", http.StatusBadRequest)
			return
		}

		policy := &secv1alpha1.SovereigntyPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}

		if err := client.IgnoreNotFound(c.Delete(r.Context(), policy)); err != nil {
			http.Error(w, "Failed to delete policy: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// NEW: PUT /api/policies/{name}
func updatePolicy(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if name == "" {
			http.Error(w, "Policy name is required", http.StatusBadRequest)
			return
		}

		var req struct {
			Namespace string `json:"namespace"`
			Country   string `json:"country"`
			Action    string `json:"action"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// Input Validation
		if req.Namespace == "" {
			req.Namespace = "default"
		}
		if len(req.Country) != 2 {
			http.Error(w, "Invalid country code: must be exactly 2 characters (ISO Alpha-2)", http.StatusBadRequest)
			return
		}
		req.Country = strings.ToUpper(req.Country)
		if req.Action != "block-kill" && req.Action != "block-noconn" && req.Action != "log" {
			http.Error(w, "Invalid action: must be block-kill, block-noconn, or log", http.StatusBadRequest)
			return
		}

		// 1. Fetch the existing policy so we have its ResourceVersion
		existingPolicy := &secv1alpha1.SovereigntyPolicy{}
		err := c.Get(r.Context(), client.ObjectKey{Name: name}, existingPolicy)
		if err != nil {
			http.Error(w, "Failed to find existing policy: "+err.Error(), http.StatusNotFound)
			return
		}

		// 2. Modify the spec fields
		existingPolicy.Spec.Namespaces = []string{req.Namespace}
		existingPolicy.Spec.DisallowedCountries = []string{req.Country}
		existingPolicy.Spec.Actions = []secv1alpha1.Action{secv1alpha1.Action(req.Action)}

		// 3. Push the update to the cluster
		if err := c.Update(r.Context(), existingPolicy); err != nil {
			http.Error(w, "Failed to update policy: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
	}
}

// ViolationsHandler queries the K8s API for SovereigntyViolation events
func (s *Server) handleViolations(w http.ResponseWriter, r *http.Request) {
	events, err := s.Clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{
		FieldSelector: "reason=SovereigntyViolation",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(events.Items); err != nil {
		log.Printf("failed to encode events: %v", err)
	}
}
