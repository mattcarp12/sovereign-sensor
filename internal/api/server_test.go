package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	secv1alpha1 "github.com/mattcarp12/sovereign-sensor/api/v1alpha1"
)

func setupTestServer(t *testing.T, initObjs ...client.Object) (*Server, *http.ServeMux) {
	scheme := runtime.NewScheme()
	require.NoError(t, secv1alpha1.AddToScheme(scheme))

	// Create a fake controller-runtime client populated with our initial objects
	clientBuilder := fakeclient.NewClientBuilder().WithScheme(scheme)
	if len(initObjs) > 0 {
		clientBuilder.WithObjects(initObjs...)
	}

	fakeCtrlClient := clientBuilder.Build()

	// Create a standard fake clientset for the CoreV1 Events API
	fakeClientset := fake.NewClientset()

	srv := &Server{
		Client:    fakeCtrlClient,
		Clientset: fakeClientset,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/policies", listPolicies(srv.Client))
	mux.HandleFunc("POST /api/policies", createPolicy(srv.Client))
	mux.HandleFunc("DELETE /api/policies/{name}", deletePolicy(srv.Client))

	return srv, mux
}

func TestListPolicies(t *testing.T) {
	policy := &secv1alpha1.SovereigntyPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-policy",
		},
		Spec: secv1alpha1.SovereigntyPolicySpec{
			Namespaces: []string{"default"},
			Actions:    []secv1alpha1.Action{secv1alpha1.ActionLog},
		},
	}

	_, mux := setupTestServer(t, policy)

	req := httptest.NewRequest(http.MethodGet, "/api/policies", nil)
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var policies []secv1alpha1.SovereigntyPolicy
	err := json.Unmarshal(rr.Body.Bytes(), &policies)
	require.NoError(t, err)

	assert.Len(t, policies, 1)
	assert.Equal(t, "test-policy", policies[0].Name)
}

func TestCreatePolicy_InvalidInput(t *testing.T) {
	_, mux := setupTestServer(t)

	// Test invalid country code length
	payload := map[string]string{
		"name":      "bad-country",
		"namespace": "default",
		"country":   "USA", // Invalid: must be 2 chars
		"action":    "block-kill",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/policies", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "Invalid country code")
}

func TestCreatePolicy_Success(t *testing.T) {
	srv, mux := setupTestServer(t)

	payload := map[string]string{
		"name":      "valid-policy",
		"namespace": "prod",
		"country":   "RU",
		"action":    "block-kill",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/policies", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusCreated, rr.Code)

	// Verify the object was actually persisted in the fake Kubernetes cache
	var createdPolicy secv1alpha1.SovereigntyPolicy
	err := srv.Client.Get(context.Background(), client.ObjectKey{Name: "valid-policy"}, &createdPolicy)
	require.NoError(t, err)

	assert.Contains(t, createdPolicy.Spec.DisallowedCountries, "RU")
	assert.Equal(t, secv1alpha1.ActionBlockKill, createdPolicy.Spec.Actions[0])
}
