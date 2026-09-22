package api

import (
	"encoding/json"

	"github.com/duck-driven-llm-proxy-service/internal/masking"
)

// encodeMappings сериализует соответствия токенов для зашифрованного хранения.
func encodeMappings(mappings []masking.TokenMapping) []byte {
	b, _ := json.Marshal(mappings)
	return b
}

// decodeMappings десериализует соответствия токенов из зашифрованного хранилища.
func decodeMappings(b []byte) ([]masking.TokenMapping, error) {
	var mappings []masking.TokenMapping
	if err := json.Unmarshal(b, &mappings); err != nil {
		return nil, err
	}
	return mappings, nil
}