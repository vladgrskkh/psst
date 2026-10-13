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
	data, err := os.ReadFile(os.Getenv("FAKE_BRIDGE_SCRIPT"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := json.Unmarshal(data, &replies); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			continue
		}
		reply, ok := replies[req.Method]
		if !ok {
			reply = []byte(`{}`)
		}
		if errObj := extractError(reply); errObj != nil {
			out, _ := json.Marshal(map[string]any{"id": req.ID, "error": errObj})
			fmt.Println(string(out))
			continue
		}
		out, _ := json.Marshal(map[string]any{"id": req.ID, "result": reply})
		fmt.Println(string(out))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if code, err := strconv.Atoi(os.Getenv("FAKE_BRIDGE_EXIT_CODE")); err == nil && code != 0 {
		os.Exit(code)
	}
}

func extractError(reply json.RawMessage) any {
	var obj map[string]any
	if err := json.Unmarshal(reply, &obj); err != nil {
		return nil
	}
	return obj["error"]
}
