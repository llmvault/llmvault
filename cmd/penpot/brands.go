package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

func brandsCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("brands subcommand is required")
	}
	switch args[0] {
	case "list":
		return brandsListCommand(args[1:])
	case "view", "get":
		return brandsViewCommand(args[1:])
	case "create":
		return brandsCreateCommand(args[1:])
	case "update", "patch":
		return brandsUpdateCommand(args[1:])
	default:
		return fmt.Errorf("unknown brands subcommand %q", args[0])
	}
}

func brandsListCommand(args []string) error {
	fs := flag.NewFlagSet("brands list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("brands list does not accept arguments")
	}
	var result map[string]any
	if err := getControlPlane(agentCanvasPath("brands"), &result); err != nil {
		return err
	}
	return printJSON(result)
}

func brandsViewCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("brand id is required")
	}
	var result map[string]any
	if err := getControlPlane(agentCanvasPath("brands/"+strings.TrimSpace(args[0])), &result); err != nil {
		return err
	}
	return printJSON(result)
}

func brandsCreateCommand(args []string) error {
	fs := flag.NewFlagSet("brands create", flag.ContinueOnError)
	rawJSON := fs.String("json", "{}", "brand payload JSON or @path")
	name := fs.String("name", "", "brand name")
	slug := fs.String("slug", "", "brand slug")
	description := fs.String("description", "", "brand description")
	isDefault := fs.Bool("default", false, "mark brand as default")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("brands create does not accept arguments")
	}
	body, err := brandJSONObject(*rawJSON)
	if err != nil {
		return err
	}
	mergeStringFlag(fs, body, "name", *name)
	mergeStringFlag(fs, body, "slug", *slug)
	mergeStringFlag(fs, body, "description", *description)
	if flagWasSet(fs, "default") {
		body["is_default"] = *isDefault
	}
	if value, ok := body["name"].(string); !ok || strings.TrimSpace(value) == "" {
		return errors.New("--name is required unless --json includes name")
	}
	var result map[string]any
	if err := postControlPlane(agentCanvasPath("brands"), body, &result); err != nil {
		return err
	}
	return printJSON(result)
}

func brandsUpdateCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("brand id is required")
	}
	brandID := strings.TrimSpace(args[0])
	if brandID == "" {
		return errors.New("brand id is required")
	}
	fs := flag.NewFlagSet("brands update", flag.ContinueOnError)
	rawJSON := fs.String("json", "{}", "brand patch JSON or @path")
	name := fs.String("name", "", "brand name")
	slug := fs.String("slug", "", "brand slug")
	description := fs.String("description", "", "brand description")
	isDefault := fs.Bool("default", false, "mark brand as default")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("brands update does not accept extra arguments")
	}
	body, err := brandJSONObject(*rawJSON)
	if err != nil {
		return err
	}
	mergeStringFlag(fs, body, "name", *name)
	mergeStringFlag(fs, body, "slug", *slug)
	mergeStringFlag(fs, body, "description", *description)
	if flagWasSet(fs, "default") {
		body["is_default"] = *isDefault
	}
	if len(body) == 0 {
		return errors.New("no fields to update")
	}
	var result map[string]any
	if err := patchControlPlane(agentCanvasPath("brands/"+brandID), body, &result); err != nil {
		return err
	}
	return printJSON(result)
}

func patchControlPlane(path string, payload any, out any) error {
	base := strings.TrimRight(mustEnv(envControlPlaneURL), "/")
	return requestJSON(http.MethodPatch, base+path, mustEnv(envRuntimeSecret), payload, out)
}

func agentCanvasPath(resource string) string {
	return "/internal/agents/" + mustEnv(envAgentID) + "/canvas/" + strings.TrimLeft(resource, "/")
}

func brandJSONObject(raw string) (map[string]any, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return map[string]any{}, nil
	}
	if strings.HasPrefix(value, "@") {
		data, err := os.ReadFile(strings.TrimPrefix(value, "@"))
		if err != nil {
			return nil, fmt.Errorf("read --json file: %w", err)
		}
		value = string(data)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil, fmt.Errorf("invalid --json: %w", err)
	}
	if out == nil {
		return nil, errors.New("--json must be an object")
	}
	return out, nil
}

func mergeStringFlag(fs *flag.FlagSet, body map[string]any, key, value string) {
	if flagWasSet(fs, key) {
		body[key] = value
	}
}

func flagWasSet(fs *flag.FlagSet, name string) bool {
	wasSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			wasSet = true
		}
	})
	return wasSet
}
