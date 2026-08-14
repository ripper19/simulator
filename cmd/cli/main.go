// Command cli is the `sim` command-line client for the simulator API.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	base := os.Getenv("SIM_URL")
	if base == "" {
		base = "http://127.0.0.1:8080"
	}

	var err error
	switch cmd {
	case "models":
		err = doModels(base, args)
	case "create":
		err = doCreate(base, args)
	case "start":
		err = doAction(base, args, "start")
	case "pause":
		err = doAction(base, args, "pause")
	case "resume":
		err = doAction(base, args, "resume")
	case "stop":
		err = doAction(base, args, "stop")
	case "step":
		err = doAction(base, args, "step")
	case "status":
		err = doStatus(base, args)
	case "metrics":
		err = doMetrics(base, args)
	case "snapshot":
		err = doSnapshot(base, args)
	case "restore":
		err = doRestore(base, args)
	case "replay":
		err = doAction(base, args, "replay")
	case "model":
		err = doModel(base, args)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage: sim <command> [args]

commands:
  models                     list registered models
  model <id>                 inspect a model
  create --model <id> [--seed n] [--n n]   create a simulation
  start   <id>               start a simulation
  pause   <id>               pause a simulation
  resume  <id>               resume a simulation
  stop    <id>               stop a simulation
  step    <id>               advance one step
  status  <id>               show simulation state
  metrics <id>               show simulation metrics
  snapshot <id>              capture a snapshot
  restore  <id> <file>       restore a snapshot from a JSON file
  replay  <id>               replay a simulation from seed

env: SIM_URL (default http://127.0.0.1:8080)`)
}

func doModels(base string, args []string) error {
	return getAndPrint(base + "/api/v1/models")
}

func doModel(base string, args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("model requires a model id")
	}
	return getAndPrint(base + "/api/v1/models/" + args[0])
}

func doRestore(base string, args []string) error {
	id, err := requireID(args, "restore")
	if err != nil {
		return err
	}
	if len(args) < 2 {
		return fmt.Errorf("restore requires a snapshot file")
	}
	data, err := os.ReadFile(args[1])
	if err != nil {
		return err
	}
	return postAndPrint(fmt.Sprintf("%s/api/v1/simulations/%s/restore", base, id), data)
}

func doCreate(base string, args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	model := fs.String("model", "", "model id")
	seed := fs.Uint64("seed", 0, "random seed")
	n := fs.Int("n", 0, "counter model: number of entities")
	fs.Parse(args)
	if *model == "" {
		return fmt.Errorf("--model is required")
	}
	config := map[string]any{}
	if *n > 0 {
		config["n"] = *n
	}
	body, _ := json.Marshal(map[string]any{
		"model_id": *model,
		"seed":     *seed,
		"config":   config,
	})
	return postAndPrint(base+"/api/v1/simulations", body)
}

func doAction(base string, args []string, action string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s requires a simulation id", action)
	}
	id := args[0]
	return postAndPrint(fmt.Sprintf("%s/api/v1/simulations/%s/%s", base, id, action), nil)
}

func doStatus(base string, args []string) error {
	id, err := requireID(args, "status")
	if err != nil {
		return err
	}
	return getAndPrint(base + "/api/v1/simulations/" + id + "/state")
}

func doMetrics(base string, args []string) error {
	id, err := requireID(args, "metrics")
	if err != nil {
		return err
	}
	return getAndPrint(base + "/api/v1/simulations/" + id + "/metrics")
}

func doSnapshot(base string, args []string) error {
	id, err := requireID(args, "snapshot")
	if err != nil {
		return err
	}
	return postAndPrint(fmt.Sprintf("%s/api/v1/simulations/%s/snapshot", base, id), nil)
}

func requireID(args []string, cmd string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("%s requires a simulation id", cmd)
	}
	return args[0], nil
}

func getAndPrint(url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func postAndPrint(url string, body []byte) error {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, url, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return printResponse(resp)
}

func printResponse(resp *http.Response) error {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s: %s", resp.Status, string(b))
	}
	fmt.Println(string(b))
	return nil
}
