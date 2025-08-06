package metrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/docker/model-runner/pkg/inference"
	"github.com/docker/model-runner/pkg/inference/models"
	"github.com/docker/model-runner/pkg/logging"
)

// maximumRecordsPerModel is the maximum number of records that will be stored
// per model.
const maximumRecordsPerModel = 10

type responseRecorder struct {
	http.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (rr *responseRecorder) Write(b []byte) (int, error) {
	rr.body.Write(b)
	return rr.ResponseWriter.Write(b)
}

func (rr *responseRecorder) WriteHeader(statusCode int) {
	rr.statusCode = statusCode
	rr.ResponseWriter.WriteHeader(statusCode)
}

func (rr *responseRecorder) Flush() {
	if flusher, ok := rr.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

type RequestResponsePair struct {
	ID         string    `json:"id"`
	Model      string    `json:"model"`
	Method     string    `json:"method"`
	URL        string    `json:"url"`
	Request    string    `json:"request"`
	Response   string    `json:"response,omitempty"`
	Error      string    `json:"error,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	StatusCode int       `json:"status_code"`
	UserAgent  string    `json:"user_agent,omitempty"`
}

type ModelData struct {
	Config  inference.BackendConfiguration `json:"config"`
	Records []*RequestResponsePair         `json:"records"`
}

type OpenAIRecorder struct {
	log          logging.Logger
	records      map[string]*ModelData // key is model ID
	modelManager *models.Manager       // for resolving model tags to IDs
	m            sync.RWMutex
}

func NewOpenAIRecorder(log logging.Logger, modelManager *models.Manager) *OpenAIRecorder {
	return &OpenAIRecorder{
		log:          log,
		modelManager: modelManager,
		records:      make(map[string]*ModelData),
	}
}

func (r *OpenAIRecorder) SetConfigForModel(model string, config *inference.BackendConfiguration) {
	if config == nil {
		r.log.Warnf("SetConfigForModel called with nil config for model %s", model)
		return
	}

	modelID := r.modelManager.ResolveModelID(model)

	r.m.Lock()
	defer r.m.Unlock()

	if r.records[modelID] == nil {
		r.records[modelID] = &ModelData{
			Records: make([]*RequestResponsePair, 0, 10),
			Config:  inference.BackendConfiguration{},
		}
	}

	r.records[modelID].Config = *config
}

func (r *OpenAIRecorder) RecordRequest(model string, req *http.Request, body []byte) string {
	modelID := r.modelManager.ResolveModelID(model)

	r.m.Lock()
	defer r.m.Unlock()

	recordID := fmt.Sprintf("%s_%d", modelID, time.Now().UnixNano())

	record := &RequestResponsePair{
		ID:        recordID,
		Model:     model,
		Method:    req.Method,
		URL:       req.URL.Path,
		Request:   string(body),
		Timestamp: time.Now(),
		UserAgent: req.UserAgent(),
	}

	modelData := r.records[modelID]
	if modelData == nil {
		modelData = &ModelData{
			Records: make([]*RequestResponsePair, 0, maximumRecordsPerModel),
			Config:  inference.BackendConfiguration{},
		}
		r.records[modelID] = modelData
	}

	// Ideally we would use a ring buffer or a linked list for storing records,
	// but we want this data returnable as JSON, so we have to live with this
	// slightly inefficieny memory shuffle. Note that truncating the front of
	// the slice and continually appending would cause the slice's capacity to
	// grow unbounded.
	if len(modelData.Records) == maximumRecordsPerModel {
		copy(
			modelData.Records[:maximumRecordsPerModel-1],
			modelData.Records[1:],
		)
		modelData.Records[maximumRecordsPerModel-1] = record
	} else {
		modelData.Records = append(modelData.Records, record)
	}

	return recordID
}

func (r *OpenAIRecorder) NewResponseRecorder(w http.ResponseWriter) http.ResponseWriter {
	rc := &responseRecorder{
		ResponseWriter: w,
		body:           &bytes.Buffer{},
		statusCode:     http.StatusOK,
	}
	return rc
}

func (r *OpenAIRecorder) RecordResponse(id, model string, rw http.ResponseWriter) {
	rr := rw.(*responseRecorder)

	responseBody := rr.body.String()
	statusCode := rr.statusCode

	var response string
	if strings.Contains(responseBody, "data: ") {
		response = r.convertStreamingResponse(responseBody)
	} else {
		response = responseBody
	}

	modelID := r.modelManager.ResolveModelID(model)

	r.m.Lock()
	defer r.m.Unlock()

	if modelData, exists := r.records[modelID]; exists {
		for _, record := range modelData.Records {
			if record.ID == id {
				record.StatusCode = statusCode
				// Populate either Response or Error field based on status code
				if statusCode >= 400 {
					record.Error = response
					record.Response = "" // Ensure Response is empty for errors
				} else {
					record.Response = response
					record.Error = "" // Ensure Error is empty for successful responses
				}
				return
			}
		}
		r.log.Errorf("Matching request (id=%s) not found for model %s - %d\n%s", id, modelID, statusCode, response)
	} else {
		r.log.Errorf("Model %s not found in records - %d\n%s", modelID, statusCode, response)
	}
}

func (r *OpenAIRecorder) convertStreamingResponse(streamingBody string) string {
	lines := strings.Split(streamingBody, "\n")
	var contentBuilder strings.Builder
	var lastChunk map[string]interface{}

	for _, line := range lines {
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break
			}

			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}

			lastChunk = chunk

			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if content, ok := delta["content"].(string); ok {
							contentBuilder.WriteString(content)
						}
					}
				}
			}
		}
	}

	if lastChunk == nil {
		return streamingBody
	}

	finalResponse := make(map[string]interface{})

	for key, value := range lastChunk {
		finalResponse[key] = value
	}

	if choices, ok := finalResponse["choices"].([]interface{}); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]interface{}); ok {
			choice["message"] = map[string]interface{}{
				"role":    "assistant",
				"content": contentBuilder.String(),
			}
			delete(choice, "delta")

			if _, ok := choice["finish_reason"]; !ok {
				choice["finish_reason"] = "stop"
			}
		}
	}

	finalResponse["object"] = "chat.completion"

	jsonResult, err := json.Marshal(finalResponse)
	if err != nil {
		return streamingBody
	}

	return string(jsonResult)
}

func (r *OpenAIRecorder) GetRecordsByModelHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		model := req.URL.Query().Get("model")

		if model == "" {
			http.Error(w, "A 'model' query parameter is required", http.StatusBadRequest)
		} else {
			// Retrieve records for the specified model.
			records := r.GetRecordsByModel(model)
			if records == nil {
				// No records found for the specified model.
				http.Error(w, fmt.Sprintf("No records found for model '%s'", model), http.StatusNotFound)
				return
			}

			modelID := r.modelManager.ResolveModelID(model)
			if err := json.NewEncoder(w).Encode(map[string]interface{}{
				"model":   model,
				"records": records,
				"count":   len(records),
				"config":  r.records[modelID].Config,
			}); err != nil {
				http.Error(w, fmt.Sprintf("Failed to encode records for model '%s': %v", model, err),
					http.StatusInternalServerError)
				return
			}
		}
	}
}

func (r *OpenAIRecorder) GetRecordsByModel(model string) []*RequestResponsePair {
	modelID := r.modelManager.ResolveModelID(model)

	r.m.RLock()
	defer r.m.RUnlock()

	if modelData, exists := r.records[modelID]; exists {
		result := make([]*RequestResponsePair, len(modelData.Records))
		copy(result, modelData.Records)
		return result
	}

	return nil
}

func (r *OpenAIRecorder) RemoveModel(model string) {
	modelID := r.modelManager.ResolveModelID(model)

	r.m.Lock()
	defer r.m.Unlock()

	if _, exists := r.records[modelID]; exists {
		delete(r.records, modelID)
		r.log.Infof("Removed records for model: %s", modelID)
	} else {
		r.log.Warnf("No records found for model: %s", modelID)
	}
}
