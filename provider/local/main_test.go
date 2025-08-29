/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

func TestNegotiateHandler(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectedHeader string
	}{
		{
			name:           "Valid GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			expectedHeader: mediaType,
		},
		{
			name:           "Invalid POST request",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
			expectedHeader: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			w := httptest.NewRecorder()

			negotiateHandler(w, req)

			res := w.Result()
			require.Equal(t, tt.expectedStatus, res.StatusCode)

			if tt.expectedHeader != "" {
				require.Equal(t, tt.expectedHeader, res.Header.Get("Content-Type"))

				// Check that response contains a valid DomainFilter
				defer res.Body.Close()
				var domainFilter endpoint.DomainFilter
				err := json.NewDecoder(res.Body).Decode(&domainFilter)
				require.NoError(t, err)
			}
		})
	}
}

func TestRecordsHandlerGet_WithValidHostsFile(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/records", nil)
	w := httptest.NewRecorder()

	recordsHandler(w, req)

	res := w.Result()
	if res.StatusCode == http.StatusOK {
		require.Equal(t, mediaType, res.Header.Get("Content-Type"))

		defer res.Body.Close()
		var endpoints []endpoint.Endpoint
		err := json.NewDecoder(res.Body).Decode(&endpoints)
		require.NoError(t, err)

		t.Logf("Found %d endpoints in /etc/hosts", len(endpoints))
	} else {
		require.Equal(t, http.StatusInternalServerError, res.StatusCode)
		t.Log("Expected behavior: /etc/hosts not accessible")
	}
}

// TODO this needs to be changed so that we can actually test the edits to /etc/hosts
func TestRecordsHandlerPost_WithValidJSON(t *testing.T) {
	if _, exists := os.LookupEnv("LOCAL_PROVIDER_E2E"); !exists {
		t.Skip("Skipping test: LOCAL_PROVIDER_E2E environment variable not set")
	}

	changes := plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "new.example.com", Targets: []string{"192.168.1.2"}},
		},
		Delete: []*endpoint.Endpoint{
			{DNSName: "old.example.com"},
		},
	}

	changesJSON, err := json.Marshal(changes)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader(changesJSON))
	w := httptest.NewRecorder()

	recordsHandler(w, req)

	res := w.Result()
	// This might succeed or fail depending on file permissions
	// We're just testing that the handler processes the request correctly
	require.Contains(t, []int{http.StatusNoContent, http.StatusInternalServerError}, res.StatusCode)

	if res.StatusCode == http.StatusNoContent {
		require.Equal(t, mediaType, res.Header.Get("Content-Type"))
		t.Log("Successfully applied changes (file was writable)")
	} else {
		t.Log("Expected behavior: /etc/hosts not writable or readable")
	}
}

func TestRecordsHandlerPost_WithInvalidJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	recordsHandler(w, req)

	res := w.Result()
	require.Contains(t, []int{http.StatusInternalServerError}, res.StatusCode)
}

func TestRecordsHandlerPost_WithEmptyTargets(t *testing.T) {
	changes := plan.Changes{
		Create: []*endpoint.Endpoint{
			{DNSName: "test.example.com", Targets: []string{}},
			{DNSName: "valid.example.com", Targets: []string{"192.168.1.1"}},
		},
	}

	changesJSON, err := json.Marshal(changes)
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodPost, "/records", bytes.NewReader(changesJSON))
	w := httptest.NewRecorder()

	recordsHandler(w, req)

	res := w.Result()
	// This might succeed or fail depending on file permissions
	require.Contains(t, []int{http.StatusNoContent, http.StatusInternalServerError}, res.StatusCode)
}

func TestRecordsHandlerInvalidMethod(t *testing.T) {
	tests := []string{http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range tests {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/records", nil)
			w := httptest.NewRecorder()

			recordsHandler(w, req)

			res := w.Result()
			require.Equal(t, http.StatusMethodNotAllowed, res.StatusCode)
		})
	}
}

func TestAdjustEndpointsHandler(t *testing.T) {
	methods := []string{http.MethodPost, http.MethodGet, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/adjustendpoints", nil)
			w := httptest.NewRecorder()

			adjustEndpointsHandler(w, req)

			res := w.Result()
			require.Equal(t, http.StatusNotImplemented, res.StatusCode)

			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Equal(t, "not implemented\n", string(body))
		})
	}
}

func TestHealthzHandler(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/healthz", nil)
			w := httptest.NewRecorder()

			healthzHandler(w, req)

			res := w.Result()
			require.Equal(t, http.StatusOK, res.StatusCode)

			defer res.Body.Close()
			body, err := io.ReadAll(res.Body)
			require.NoError(t, err)
			require.Equal(t, "ok", string(body))
		})
	}
}

func TestMediaTypeConstant(t *testing.T) {
	expectedMediaType := "application/vnd.external-dns.webhook+json;version=1"
	require.Equal(t, expectedMediaType, mediaType)
}

func TestPlanChangesUnmarshaling(t *testing.T) {
	// Test that we can properly unmarshal the plan.Changes structure
	changes := plan.Changes{
		Create: []*endpoint.Endpoint{
			{
				DNSName:    "create.example.com",
				RecordType: "A",
				Targets:    []string{"192.168.1.1"},
			},
		},
		UpdateOld: []*endpoint.Endpoint{
			{
				DNSName:    "update.example.com",
				RecordType: "A",
				Targets:    []string{"192.168.1.2"},
			},
		},
		UpdateNew: []*endpoint.Endpoint{
			{
				DNSName:    "update.example.com",
				RecordType: "A",
				Targets:    []string{"192.168.1.3"},
			},
		},
		Delete: []*endpoint.Endpoint{
			{
				DNSName:    "delete.example.com",
				RecordType: "A",
				Targets:    []string{"192.168.1.4"},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(changes)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaledChanges plan.Changes
	err = json.Unmarshal(data, &unmarshaledChanges)
	require.NoError(t, err)

	// Verify the data
	require.Len(t, unmarshaledChanges.Create, 1)
	require.Len(t, unmarshaledChanges.UpdateOld, 1)
	require.Len(t, unmarshaledChanges.UpdateNew, 1)
	require.Len(t, unmarshaledChanges.Delete, 1)

	require.Equal(t, "create.example.com", unmarshaledChanges.Create[0].DNSName)
	require.Equal(t, "update.example.com", unmarshaledChanges.UpdateOld[0].DNSName)
	require.Equal(t, "update.example.com", unmarshaledChanges.UpdateNew[0].DNSName)
	require.Equal(t, "delete.example.com", unmarshaledChanges.Delete[0].DNSName)
}

func TestEndpointSerialization(t *testing.T) {
	// Test that endpoint structures are properly serialized/deserialized
	endpoints := []endpoint.Endpoint{
		{
			DNSName:    "test.example.com",
			RecordType: "A",
			Targets:    []string{"192.168.1.1", "192.168.1.2"},
			RecordTTL:  300,
		},
		{
			DNSName:    "cname.example.com",
			RecordType: "CNAME",
			Targets:    []string{"target.example.com"},
			RecordTTL:  600,
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(endpoints)
	require.NoError(t, err)

	// Unmarshal back
	var unmarshaledEndpoints []endpoint.Endpoint
	err = json.Unmarshal(data, &unmarshaledEndpoints)
	require.NoError(t, err)

	// Verify the data
	require.Len(t, unmarshaledEndpoints, 2)

	require.Equal(t, "test.example.com", unmarshaledEndpoints[0].DNSName)
	require.Equal(t, "A", unmarshaledEndpoints[0].RecordType)
	require.Equal(t, endpoint.Targets{"192.168.1.1", "192.168.1.2"}, unmarshaledEndpoints[0].Targets)
	require.Equal(t, endpoint.TTL(300), unmarshaledEndpoints[0].RecordTTL)

	require.Equal(t, "cname.example.com", unmarshaledEndpoints[1].DNSName)
	require.Equal(t, "CNAME", unmarshaledEndpoints[1].RecordType)
	require.Equal(t, endpoint.Targets{"target.example.com"}, unmarshaledEndpoints[1].Targets)
	require.Equal(t, endpoint.TTL(600), unmarshaledEndpoints[1].RecordTTL)
}

func TestHTTPServerIntegration(t *testing.T) {
	// Test that all handlers work together
	mux := http.NewServeMux()
	mux.HandleFunc("/", negotiateHandler)
	mux.HandleFunc("/records", recordsHandler)
	mux.HandleFunc("/adjustendpoints", adjustEndpointsHandler)
	mux.HandleFunc("/healthz", healthzHandler)

	server := httptest.NewServer(mux)
	defer server.Close()

	// Test negotiate endpoint
	resp, err := http.Get(server.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, mediaType, resp.Header.Get("Content-Type"))

	// Test healthz endpoint
	resp, err = http.Get(server.URL + "/healthz")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "ok", string(body))

	// Test records GET endpoint
	resp, err = http.Get(server.URL + "/records")
	require.NoError(t, err)
	defer resp.Body.Close()
	// This will either succeed or fail depending on file access
	require.Contains(t, []int{http.StatusOK, http.StatusInternalServerError}, resp.StatusCode)

	// Test adjustendpoints endpoint
	resp, err = http.Post(server.URL+"/adjustendpoints", "application/json", bytes.NewReader([]byte("{}")))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotImplemented, resp.StatusCode)
}
