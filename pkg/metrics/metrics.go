package metrics

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/docker/model-distribution/types"
	"github.com/docker/model-runner/pkg/logging"
	"github.com/sirupsen/logrus"
)

type Tracker struct {
	doNotTrack bool
	httpClient *http.Client
	log        logging.Logger
}

func NewTracker(httpClient *http.Client, log logging.Logger) *Tracker {
	client := *httpClient
	client.Timeout = 5 * time.Second

	if os.Getenv("DEBUG") == "1" {
		if logger, ok := log.(*logrus.Logger); ok {
			logger.SetLevel(logrus.DebugLevel)
		} else if entry, ok := log.(*logrus.Entry); ok {
			entry.Logger.SetLevel(logrus.DebugLevel)
		}
	}

	return &Tracker{
		doNotTrack: os.Getenv("DO_NOT_TRACK") == "1",
		httpClient: &client,
		log:        log,
	}
}

func (t *Tracker) TrackModel(model types.Model) {
	if t.doNotTrack {
		return
	}

	go t.trackModel(model)
}

func (t *Tracker) trackModel(model types.Model) {
	id, err := model.ID()
	if err != nil {
		t.log.Errorf("failed to get model ID for %v: %v", model.Tags(), err)
		return
	}

	tags := model.Tags()
	t.log.Debugln("Tracking model:", tags, id)
	if len(tags) == 0 {
		return
	}

	url := fmt.Sprintf("https://hub.docker.com/layers/%s/latest/images/%s", tags[0], id)
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		t.log.Errorf("failed to create request for %s: %v", url, err)
		return
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		t.log.Debugf("failed to make HEAD request to %s: %v", url, err)
		return
	}
	defer resp.Body.Close()

	t.log.Debugf("Tracking %s returned status: %d", url, resp.StatusCode)
}
