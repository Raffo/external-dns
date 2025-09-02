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
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

func main() {
	listenAddress := flag.String("listen-address", "127.0.0.1", "Address to listen on")
	port := flag.Int("port", 8888, "Port to listen on")
	flag.Parse()

	http.HandleFunc("/", negotiateHandler)
	http.HandleFunc("/records", recordsHandler)
	http.HandleFunc("/adjustendpoints", adjustEndpointsHandler)
	http.HandleFunc("/healthz", healthzHandler)

	addr := fmt.Sprintf("%s:%d", *listenAddress, *port)
	log.Printf("Starting webhook provider on %s\n", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func negotiateHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", api.MediaTypeFormatAndVersion)
	// Return your supported DomainFilter here
	json.NewEncoder(w).Encode(endpoint.DomainFilter{})
}

func recordsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", api.MediaTypeFormatAndVersion)
		hosts, err := os.Open("/etc/hosts")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer hosts.Close()

		var endpoints []endpoint.Endpoint
		scanner := bufio.NewScanner(hosts)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}

			ip := fields[0]
			for _, hostname := range fields[1:] {
				if strings.HasPrefix(hostname, "#") {
					break
				}
				endpoints = append(endpoints, endpoint.Endpoint{
					DNSName:    hostname,
					RecordType: "A",
					Targets:    []string{ip},
				})
			}
		}

		if err := scanner.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(endpoints)
		return
	}
	if r.Method == http.MethodPost { // TODO review this one here
		w.Header().Set("Content-Type", api.MediaTypeFormatAndVersion)
		var changes plan.Changes
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err = json.Unmarshal(body, &changes)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hosts, err := os.ReadFile("/etc/hosts")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		lines := strings.Split(string(hosts), "\n")
		var newLines []string

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				newLines = append(newLines, line)
				continue
			}

			fields := strings.Fields(trimmed)
			if len(fields) < 2 {
				newLines = append(newLines, line)
				continue
			}

			shouldKeep := true
			for _, del := range changes.Delete {
				for _, hostname := range fields[1:] {
					if hostname == del.DNSName {
						shouldKeep = false
						break
					}
				}
				if !shouldKeep {
					break
				}
			}

			if shouldKeep {
				newLines = append(newLines, line)
			}
		}

		for _, create := range changes.Create {
			if len(create.Targets) > 0 {
				newLines = append(newLines, fmt.Sprintf("%s\t%s", create.Targets[0], create.DNSName))
			}
		}

		newContent := strings.Join(newLines, "\n")
		err = os.WriteFile("/etc/hosts", []byte(newContent), 0644)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func adjustEndpointsHandler(w http.ResponseWriter, r *http.Request) {
	// read the endpoints from the input, return them straight back
	var endpoints []endpoint.Endpoint
	if err := json.NewDecoder(r.Body).Decode(&endpoints); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", api.MediaTypeFormatAndVersion)
	json.NewEncoder(w).Encode(endpoints)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
