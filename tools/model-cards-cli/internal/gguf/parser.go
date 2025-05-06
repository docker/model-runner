package gguf

import (
	"context"
	"fmt"
	"github.com/docker/model-cards/tools/build-tables/types"

	parser "github.com/gpustack/gguf-parser-go"
)

// Parser implements the GGUFParser interface
type Parser struct{}

// NewParser creates a new GGUF parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseRemote parses a remote GGUF file
func (p *Parser) ParseRemote(ctx context.Context, url, token string) (types.ModelDescriptor, error) {
	gf, err := parser.ParseGGUFFileRemote(ctx, url, parser.UseBearerAuth(token))
	if err != nil {
		return nil, fmt.Errorf("failed to parse GGUF: %w", err)
	}

	return &File{
		file: gf,
	}, nil
}
