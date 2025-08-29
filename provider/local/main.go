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
)

const mediaType = "application/vnd.external-dns.webhook+json;version=1"

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
	w.Header().Set("Content-Type", mediaType)
	// Return your supported DomainFilter here
	json.NewEncoder(w).Encode(endpoint.DomainFilter{})
}

func recordsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", mediaType)
		// Return your DNS records here
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
	if r.Method == http.MethodPost {
		w.Header().Set("Content-Type", mediaType)
		// Read and apply changes
		var changes plan.Changes
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &changes)
		// Read current hosts file
		hosts, err := os.ReadFile("/etc/hosts")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		lines := strings.Split(string(hosts), "\n")
		var newLines []string

		// Process each line, removing entries that match endpoints to delete
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

			// Check if this line contains any hostname we need to delete
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

		// Add new entries
		for _, create := range changes.Create {
			if len(create.Targets) > 0 {
				newLines = append(newLines, fmt.Sprintf("%s\t%s", create.Targets[0], create.DNSName))
			}
		}

		// Write back to hosts file
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
	log.Println("adjustEndpoints method not implemented")
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func healthzHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
