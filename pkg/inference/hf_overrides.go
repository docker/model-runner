package inference

import (
	"fmt"
	"regexp"
)

// HFOverrides contains HuggingFace model configuration overrides.
// Uses map[string]interface{} for flexibility, with validation to prevent injection attacks.
// This matches vLLM's --hf-overrides which accepts "a JSON string parsed into a dictionary".
type HFOverrides map[string]interface{}

// validHFOverridesKeyRegex allows only alphanumeric characters and underscores,
// must start with a letter or underscore (not a number).
// This prevents command injection via keys like "--malicious-flag" or "key;rm -rf /".
var validHFOverridesKeyRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Validate ensures all keys and values in HFOverrides are safe.
// Keys must be alphanumeric with underscores only (no special characters that could be exploited).
// Values can be primitives (string, bool, number), arrays, or nested objects.
// Nested objects have their keys validated recursively.
func (h HFOverrides) Validate() error {
	for key, value := range h {
		// Validate key format
		if !validHFOverridesKeyRegex.MatchString(key) {
			return fmt.Errorf("invalid hf_overrides key %q: must contain only alphanumeric characters and underscores, and start with a letter or underscore", key)
		}
		// Validate value is a safe type
		if err := validateHFOverridesValue(key, value); err != nil {
			return err
		}
	}
	return nil
}

// validateHFOverridesValue ensures the value is a primitive, array, or nested object with valid keys.
// Validation is recursive for nested objects and arrays.
func validateHFOverridesValue(key string, value interface{}) error {
	switch v := value.(type) {
	case string, bool, float64, nil,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		// Primitives are OK (JSON numbers decode as float64, but programmatic construction may use int types)
		return nil
	case []interface{}:
		// Arrays are OK if all elements are valid (primitives, objects, or nested arrays)
		for i, elem := range v {
			if err := validateHFOverridesValue(fmt.Sprintf("%s[%d]", key, i), elem); err != nil {
				return err
			}
		}
		return nil
	case map[string]interface{}:
		// Nested objects are OK if all keys are valid and all values are valid recursively
		for nestedKey, nestedValue := range v {
			if !validHFOverridesKeyRegex.MatchString(nestedKey) {
				return fmt.Errorf("invalid hf_overrides nested key %q in %q: must contain only alphanumeric characters and underscores, and start with a letter or underscore", nestedKey, key)
			}
			if err := validateHFOverridesValue(key+"."+nestedKey, nestedValue); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("invalid hf_overrides value for key %q: unsupported type %T", key, value)
	}
}
