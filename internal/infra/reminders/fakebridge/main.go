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
	var data []byte
	{
		var err error
		//nolint:gosec // The test harness sets this to a temporary fixture path.
		data, err = os.ReadFile(os.Getenv("FAKE_BRIDGE_SCRIPT"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
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
		response := map[string]any{"id": req.ID}
		if errObj := extractError(reply); errObj != nil {
			response["error"] = errObj
		} else {
			response["result"] = reply
		}
		out, err := json.Marshal(response)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, string(out))
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
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
