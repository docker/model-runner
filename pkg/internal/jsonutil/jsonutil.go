package jsonutil

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReadFile parses the contents of a file as JSON.
func ReadFile[T any](path string, result T) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()
	dec := json.NewDecoder(f)
	if err := dec.Decode(&result); err != nil {
		return fmt.Errorf("parsing JSON: %w", err)
	}
	return nil
}
