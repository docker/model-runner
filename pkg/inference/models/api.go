package models

import (
	"fmt"

	"github.com/docker/model-distribution/types"
)

// ModelCreateRequest represents a model create request. It is designed to
// follow Docker Engine API conventions, most closely following the request
// associated with POST /images/create. At the moment is only designed to
// facilitate pulls, though in the future it may facilitate model building and
// refinement (such as fine tuning, quantization, or distillation).
type ModelCreateRequest struct {
	// From is the name of the model to pull.
	From string `json:"from"`
}

// ToOpenAIList converts the model list to its OpenAI API representation. This function never
// returns a nil slice (though it may return an empty slice).
func ToOpenAIList(l []types.Model) (*OpenAIModelList, error) {
	// Convert the constituent models.
	models := make([]*OpenAIModel, len(l))
	for i, model := range l {
		openAI, err := ToOpenAI(model)
		if err != nil {
			return nil, fmt.Errorf("convert model: %w", err)
		}
		models[i] = openAI
	}

	// Create the OpenAI model list.
	return &OpenAIModelList{
		Object: "list",
		Data:   models,
	}, nil
}

// ToOpenAI converts a types.Model to its OpenAI API representation.
func ToOpenAI(m types.Model) (*OpenAIModel, error) {
	desc, err := m.Descriptor()
	if err != nil {
		return nil, fmt.Errorf("get descriptor: %w", err)
	}

	created := int64(0)
	if desc.Created != nil {
		created = desc.Created.Unix()
	}

	id, err := m.ID()
	if err != nil {
		return nil, fmt.Errorf("get model ID: %w", err)
	}
	if tags := m.Tags(); len(tags) > 0 {
		id = tags[0]
	}

	return &OpenAIModel{
		ID:      id,
		Object:  "model",
		Created: created,
		OwnedBy: "docker",
	}, nil
}

// OpenAIModel represents a locally stored model using OpenAI conventions.
type OpenAIModel struct {
	// ID is the model tag.
	ID string `json:"id"`
	// Object is the object type. For OpenAIModel, it is always "model".
	Object string `json:"object"`
	// Created is the Unix epoch timestamp corresponding to the model creation.
	Created int64 `json:"created"`
	// OwnedBy is the model owner. At the moment, it is always "docker".
	OwnedBy string `json:"owned_by"`
}

// OpenAIModelList represents a list of models using OpenAI conventions.
type OpenAIModelList struct {
	// Object is the object type. For OpenAIModelList, it is always "list".
	Object string `json:"object"`
	// Data is the list of models.
	Data []*OpenAIModel `json:"data"`
}

type Model struct {
	// ID is the globally unique model identifier.
	ID string `json:"id"`
	// Tags are the list of tags associated with the model.
	Tags []string `json:"tags,omitempty"`
	// Created is the Unix epoch timestamp corresponding to the model creation.
	Created int64 `json:"created"`
	// Config describes the model.
	Config types.Config `json:"config"`
}

func ToModel(m types.Model) (*Model, error) {
	desc, err := m.Descriptor()
	if err != nil {
		return nil, fmt.Errorf("get descriptor: %w", err)
	}

	id, err := m.ID()
	if err != nil {
		return nil, fmt.Errorf("get id: %w", err)
	}

	cfg, err := m.Config()
	if err != nil {
		return nil, fmt.Errorf("get config: %w", err)
	}

	created := int64(0)
	if desc.Created != nil {
		created = desc.Created.Unix()
	}

	return &Model{
		ID:      id,
		Tags:    m.Tags(),
		Created: created,
		Config:  cfg,
	}, nil
}
