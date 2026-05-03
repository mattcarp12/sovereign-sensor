package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
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
	Clientset *kubernetes.Clientset
}

func (s *Server) Start(addr string) error {
	mux := http.NewServeMux()

	// 1. API Endpoints
	mux.HandleFunc("GET /api/policies", listPolicies(s.Client))
	mux.HandleFunc("POST /api/policies", createPolicy(s.Client))
	mux.HandleFunc("DELETE /api/policies", deletePolicy(s.Client))
	mux.HandleFunc("GET /api/violations", s.handleViolations)

	// 2. Serve the Embedded React App
	// We extract the 'dist' folder from the embedded filesystem
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
		json.NewEncoder(w).Encode(policies.Items)
	}
}

// 2. POST /api/policies - Create a new SovereigntyPolicy
func createPolicy(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse the JSON from React
		var req struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			Country   string `json:"country"`
			Action    string `json:"action"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		// Construct the Custom Resource
		policy := &secv1alpha1.SovereigntyPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: req.Name},
			Spec: secv1alpha1.SovereigntyPolicySpec{
				Namespaces:          []string{req.Namespace},
				DisallowedCountries: []string{req.Country},
				Actions:             []secv1alpha1.Action{secv1alpha1.Action(req.Action)},
				Description:         "Generated via Web UI",
			},
		}

		if err := c.Create(r.Context(), policy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// 3. DELETE /api/policies/{name} - Delete a Policy
func deletePolicy(c client.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]

		policy := &secv1alpha1.SovereigntyPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}

		if err := c.Delete(r.Context(), policy); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// ViolationsHandler queries the K8s API for SovereigntyViolation events
func (s *Server) handleViolations(w http.ResponseWriter, r *http.Request) {
	// We only want our specific security events from all namespaces
	events, err := s.Clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{
		FieldSelector: "reason=SovereigntyViolation",
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Format the response
	w.Header().Set("Content-Type", "application/json")
	// (In a production app, we would map this to a cleaner struct,
	// but returning the raw K8s events array works perfectly for v0.1)
	json.NewEncoder(w).Encode(events.Items)
}
