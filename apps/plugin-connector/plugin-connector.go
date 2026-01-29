package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	PORT       = 37890
	INPUT_FILE = "input.json"
	BAKER_EXE  = "gui-baker.exe" // or "gui-baker" on Linux/Mac
)

type BakeRequest struct {
	Elements []map[string]interface{} `json:"elements"`
}

func printBanner() {
	fmt.Println("╔════════════════════════════════════════╗")
	fmt.Println("║    Roblox GUI Baker Server v1.0        ║")
	fmt.Println("╚════════════════════════════════════════╝")
	fmt.Println()
}

func logWithTime(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] %s\n", timestamp, fmt.Sprintf(format, args...))
}

func handleBake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logWithTime("📨 Received bake request from %s", r.RemoteAddr)

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logWithTime("❌ Error reading request: %v", err)
		http.Error(w, "Error reading request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate JSON
	var elements []interface{}
	err = json.Unmarshal(body, &elements)
	if err != nil {
		logWithTime("❌ Invalid JSON: %v", err)
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	logWithTime("✓ Received %d GUI elements", len(elements))

	// Write to input file
	logWithTime("💾 Writing to %s...", INPUT_FILE)
	err = os.WriteFile(INPUT_FILE, body, 0644)
	if err != nil {
		logWithTime("❌ Error writing file: %v", err)
		http.Error(w, "Error writing file", http.StatusInternalServerError)
		return
	}

	logWithTime("✓ Saved input file")

	// Check if baker executable exists
	bakerPath := BAKER_EXE
	if _, err := os.Stat(bakerPath); os.IsNotExist(err) {
		// Try without .exe extension
		bakerPath = "gui-baker"
		if _, err := os.Stat(bakerPath); os.IsNotExist(err) {
			logWithTime("❌ Baker executable not found!")
			logWithTime("   Looking for: %s or gui-baker", BAKER_EXE)
			http.Error(w, "Baker executable not found", http.StatusInternalServerError)
			return
		}
	}

	// Execute baker
	logWithTime("🎨 Starting baker...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	absPath, _ := filepath.Abs(bakerPath)
	cmd := exec.Command(absPath, "--input", INPUT_FILE)

	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logWithTime("❌ Error creating stdout pipe: %v", err)
		http.Error(w, "Error starting baker", http.StatusInternalServerError)
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		logWithTime("❌ Error creating stderr pipe: %v", err)
		http.Error(w, "Error starting baker", http.StatusInternalServerError)
		return
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		logWithTime("❌ Error starting baker: %v", err)
		http.Error(w, "Error starting baker", http.StatusInternalServerError)
		return
	}

	// Read output in real-time
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			fmt.Println("  " + scanner.Text())
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println("  ⚠ " + scanner.Text())
		}
	}()

	// Wait for command to finish
	err = cmd.Wait()

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if err != nil {
		logWithTime("❌ Baker failed: %v", err)
		http.Error(w, "Baker execution failed", http.StatusInternalServerError)
		return
	}

	logWithTime("✅ Bake complete!")

	// Send success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "GUI baked successfully!",
		"output":  "baked_gui.png",
	})
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"server": "Roblox GUI Baker Server",
	})
}

func main() {
	printBanner()

	// Check if baker exists
	bakerPath := BAKER_EXE
	if _, err := os.Stat(bakerPath); os.IsNotExist(err) {
		bakerPath = "gui-baker"
		if _, err := os.Stat(bakerPath); os.IsNotExist(err) {
			logWithTime("⚠ Warning: Baker executable not found!")
			logWithTime("   Make sure 'gui-baker' or 'gui-baker.exe' is in the same directory")
		} else {
			logWithTime("✓ Found baker: %s", bakerPath)
		}
	} else {
		logWithTime("✓ Found baker: %s", bakerPath)
	}

	// Setup routes
	http.HandleFunc("/bake", handleBake)
	http.HandleFunc("/health", handleHealth)

	// Start server
	addr := fmt.Sprintf(":%d", PORT)
	logWithTime("🚀 Server starting on http://localhost%s", addr)
	logWithTime("📡 Listening for Roblox Studio connections...")
	logWithTime("")
	logWithTime("Endpoints:")
	logWithTime("  POST http://localhost%s/bake   - Bake GUI elements", addr)
	logWithTime("  GET  http://localhost%s/health - Health check", addr)
	logWithTime("")
	logWithTime("Press Ctrl+C to stop")
	logWithTime("")

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
