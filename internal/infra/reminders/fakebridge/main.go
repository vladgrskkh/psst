package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type bridgeRequest struct {
	ID     int             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if exitCode, err := strconv.Atoi(os.Getenv("FAKE_BRIDGE_EXIT_CODE")); err == nil && exitCode != 0 {
		os.Exit(exitCode)
	}
}

func run() error {
	replies, err := readReplies()
	if err != nil {
		return err
	}
	return serve(bufio.NewScanner(os.Stdin), replies)
}

func readReplies() (map[string]json.RawMessage, error) {
	//nolint:gosec // The test harness sets this to a temporary fixture path.
	data, err := os.ReadFile(os.Getenv("FAKE_BRIDGE_SCRIPT"))
	if err != nil {
		return nil, err
	}
	var replies map[string]json.RawMessage
	return replies, json.Unmarshal(data, &replies)
}

func serve(scanner *bufio.Scanner, replies map[string]json.RawMessage) error {
	for scanner.Scan() {
		var req bridgeRequest
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
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
	return scanner.Err()
}

func extractError(reply json.RawMessage) any {
	var obj map[string]any
	if err := json.Unmarshal(reply, &obj); err != nil {
		return nil
	}
	return obj["error"]
}
