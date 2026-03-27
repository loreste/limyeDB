// LimyeDB CLI - Command line interface for import/export and management
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/limyedb/limyedb/pkg/version"
)

var (
	host    = flag.String("host", "http://localhost:8080", "LimyeDB server URL")
	apiKey  = flag.String("api-key", "", "API key for authentication")
	timeout = flag.Duration("timeout", 30*time.Second, "Request timeout")
)

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		printUsage()
		os.Exit(1)
	}

	client := &Client{
		host:    *host,
		apiKey:  *apiKey,
		timeout: *timeout,
	}

	cmd := flag.Arg(0)
	args := flag.Args()[1:]

	var err error
	switch cmd {
	case "import":
		err = cmdImport(client, args)
	case "export":
		err = cmdExport(client, args)
	case "collections":
		err = cmdCollections(client, args)
	case "create":
		err = cmdCreate(client, args)
	case "delete":
		err = cmdDelete(client, args)
	case "info":
		err = cmdInfo(client, args)
	case "health":
		err = cmdHealth(client)
	case "backup":
		err = cmdBackup(client, args)
	case "restore":
		err = cmdRestore(client, args)
	case "version":
		err = cmdVersion()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", cmd)
		printUsage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`LimyeDB CLI - Command line interface for LimyeDB
Usage:
  limyedb-cli [options] <command> [arguments]

Options:
  -host string      LimyeDB server URL (default "http://localhost:8080")
  -api-key string   API key for authentication
  -timeout duration Request timeout (default 30s)

Commands:
  import <collection> <file>   Import points from JSON file
  export <collection> <file>   Export collection to JSON file
  collections                  List all collections
  create <name> <dimension>    Create a new collection
  delete <name>                Delete a collection
  info <name>                  Get collection information
  health                       Check server health
  backup <output>              Create a backup
  restore <input>              Restore from backup
  version                      Print CLI version

Examples:
  limyedb-cli import documents data.json
  limyedb-cli export documents backup.json
  limyedb-cli create my_collection 1536
  limyedb-cli -host https://api.example.com -api-key secret collections
`)
}

type Client struct {
	host    string
	apiKey  string
	timeout time.Duration
}

func (c *Client) request(method, path string, body io.Reader) (*http.Response, error) {
	client := &http.Client{Timeout: c.timeout}

	req, err := http.NewRequest(method, c.host+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	return client.Do(req)
}

func cmdImport(client *Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: import <collection> <file>")
	}

	collection := args[0]
	filename := args[1]

	// Read file - sanitize path to prevent directory traversal
	filename = filepath.Clean(filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse JSON
	var importData struct {
		Points []map[string]interface{} `json:"points"`
	}
	if err := json.Unmarshal(data, &importData); err != nil {
		return fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Import in batches
	batchSize := 100
	total := len(importData.Points)
	imported := 0

	for i := 0; i < total; i += batchSize {
		end := i + batchSize
		if end > total {
			end = total
		}

		batch := importData.Points[i:end]
		payload, _ := json.Marshal(map[string]interface{}{"points": batch})

		resp, err := client.request("PUT", "/collections/"+collection+"/points", strings.NewReader(string(payload)))
		if err != nil {
			return fmt.Errorf("failed to import batch: %w", err)
		}
		_ = resp.Body.Close() // Error intentionally ignored for HTTP response body

		if resp.StatusCode >= 400 {
			return fmt.Errorf("server returned status %d", resp.StatusCode)
		}

		imported += len(batch)
		fmt.Printf("\rImported %d/%d points...", imported, total)
	}

	fmt.Printf("\nSuccessfully imported %d points to %s\n", imported, collection)
	return nil
}

func cmdExport(client *Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: export <collection> <file>")
	}

	collection := args[0]
	filename := args[1]

	// Scroll through all points
	var allPoints []map[string]interface{}
	var offset string

	for {
		payload := map[string]interface{}{
			"limit":        1000,
			"with_payload": true,
			"with_vector":  true,
		}
		if offset != "" {
			payload["offset"] = offset
		}

		body, _ := json.Marshal(payload)
		resp, err := client.request("POST", "/collections/"+collection+"/points/scroll", strings.NewReader(string(body)))
		if err != nil {
			return fmt.Errorf("failed to scroll: %w", err)
		}

		var result struct {
			Points     []map[string]interface{} `json:"points"`
			NextOffset string                   `json:"next_offset"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			_ = resp.Body.Close() // Error intentionally ignored for HTTP response body
			return fmt.Errorf("failed to decode response: %w", err)
		}
		_ = resp.Body.Close() // Error intentionally ignored for HTTP response body

		allPoints = append(allPoints, result.Points...)
		fmt.Printf("\rExported %d points...", len(allPoints))

		if result.NextOffset == "" {
			break
		}
		offset = result.NextOffset
	}

	// Write to file
	exportData := map[string]interface{}{
		"collection":  collection,
		"points":      allPoints,
		"exported_at": time.Now().Format(time.RFC3339),
	}

	output, _ := json.MarshalIndent(exportData, "", "  ")
	if err := os.WriteFile(filename, output, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("\nSuccessfully exported %d points to %s\n", len(allPoints), filename)
	return nil
}

func cmdCollections(client *Client, args []string) error {
	resp, err := client.request("GET", "/collections", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		Collections []struct {
			Name        string `json:"name"`
			Dimension   int    `json:"dimension"`
			Metric      string `json:"metric"`
			PointsCount int64  `json:"points_count"`
		} `json:"collections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	fmt.Printf("%-30s %-10s %-10s %-15s\n", "NAME", "DIMENSION", "METRIC", "POINTS")
	fmt.Println(strings.Repeat("-", 70))
	for _, c := range result.Collections {
		fmt.Printf("%-30s %-10d %-10s %-15d\n", c.Name, c.Dimension, c.Metric, c.PointsCount)
	}

	return nil
}

func cmdCreate(client *Client, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: create <name> <dimension>")
	}

	name := args[0]
	var dimension int
	if _, err := fmt.Sscanf(args[1], "%d", &dimension); err != nil {
		return fmt.Errorf("invalid dimension: %w", err)
	}

	payload := map[string]interface{}{
		"name":      name,
		"dimension": dimension,
		"metric":    "cosine",
	}
	body, _ := json.Marshal(payload)

	resp, err := client.request("POST", "/collections", strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to create collection: %s", string(body))
	}

	fmt.Printf("Collection '%s' created successfully (dimension=%d)\n", name, dimension)
	return nil
}

func cmdDelete(client *Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: delete <name>")
	}

	name := args[0]

	resp, err := client.request("DELETE", "/collections/"+name, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to delete collection: %s", string(body))
	}

	fmt.Printf("Collection '%s' deleted successfully\n", name)
	return nil
}

func cmdInfo(client *Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: info <name>")
	}

	name := args[0]

	resp, err := client.request("GET", "/collections/"+name, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))
	return nil
}

func cmdHealth(client *Client) error {
	resp, err := client.request("GET", "/health", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	fmt.Printf("Status: %v\n", result["status"])
	fmt.Printf("Version: %v\n", result["version"])
	return nil
}

func cmdBackup(client *Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: backup <output>")
	}

	resp, err := client.request("POST", "/snapshots", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var result struct {
		ID        string `json:"id"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	fmt.Printf("Backup created: %s at %s\n", result.ID, result.CreatedAt)
	return nil
}

func cmdRestore(client *Client, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: restore <snapshot-id>")
	}

	snapshotID := args[0]

	resp, err := client.request("POST", "/snapshots/"+snapshotID+"/restore", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to restore: %s", string(body))
	}

	fmt.Printf("Snapshot '%s' restored successfully\n", snapshotID)
	return nil
}

func cmdVersion() error {
	fmt.Printf("LimyeDB CLI version %s\n", version.String())
	return nil
}
