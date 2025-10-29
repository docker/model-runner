package formatter

import (
	"bytes"
	"encoding/json"
)

const standardIndentation = "    "

// ToStandardJSON return a string with the JSON representation of the interface{}
func ToStandardJSON(i interface{}) (string, error) {
	return ToJSON(i, "", standardIndentation)
}

// ToJSON return a string with the JSON representation of the interface{}
func ToJSON(i interface{}, prefix string, indentation string) (string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent(prefix, indentation)
	err := encoder.Encode(i)
	return buffer.String(), err
}
