package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

func main() {
	var replies map[string]json.RawMessage
	//nolint:gosec // The test harness sets this to a temporary fixture path.
	data, readErr := os.ReadFile(os.Getenv("FAKE_BRIDGE_SCRIPT"))
	if readErr != nil {
		fmt.Fprintln(os.Stderr, readErr)
		os.Exit(1)
	}
	if decodeErr := json.Unmarshal(data, &replies); decodeErr != nil {
		fmt.Fprintln(os.Stderr, decodeErr)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if requestErr := json.Unmarshal(scanner.Bytes(), &req); requestErr != nil {
			continue
		}
		reply, ok := replies[req.Method]
		if !ok {
			reply = []byte(`{}`)
		}
		if errObj := extractError(reply); errObj != nil {
			out, _ := json.Marshal(map[string]any{"id": req.ID, "error": errObj})
			fmt.Fprintln(os.Stdout, string(out))
			continue
		}
		out, _ := json.Marshal(map[string]any{"id": req.ID, "result": reply})
		fmt.Fprintln(os.Stdout, string(out))
	}
	if scanErr := scanner.Err(); scanErr != nil {
		fmt.Fprintln(os.Stderr, scanErr)
		os.Exit(1)
	}
	if exitCode, err := strconv.Atoi(os.Getenv("FAKE_BRIDGE_EXIT_CODE")); err == nil && exitCode != 0 {
		os.Exit(exitCode)
	}
}

func extractError(reply json.RawMessage) any {
	var obj map[string]any
	if err := json.Unmarshal(reply, &obj); err != nil {
		return nil
	}
	return obj["error"]
}
