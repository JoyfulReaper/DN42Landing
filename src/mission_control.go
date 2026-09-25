package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultMissionControlURL = "http://127.0.0.1:5190/api/events"
	missionControlEventType  = "dn42landing.visit"
)

type missionControlEvent struct {
	EventID       string      `json:"eventId"`
	EventType     string      `json:"eventType"`
	SchemaVersion int         `json:"schemaVersion"`
	OccurredAt    time.Time   `json:"occurredAt"`
	CorrelationID *string     `json:"correlationId"`
	Payload       interface{} `json:"payload"`
}

type missionControlVisitPayload struct {
	Remote    string `json:"remote"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	UserAgent string `json:"userAgent"`
}

var missionControlHTTPClient = &http.Client{
	Timeout: time.Second,
}

func init() {
	if strings.TrimSpace(os.Getenv("MISSION_CONTROL_API_KEY")) == "" {
		log.Printf(
			"warning: MISSION_CONTROL_API_KEY is not set; Mission Control visit telemetry disabled",
		)
	}
}

func makeMissionControlVisitPayload(r *http.Request) missionControlVisitPayload {
	return missionControlVisitPayload{
		Remote:    remoteHost(r.RemoteAddr),
		Method:    r.Method,
		Path:      r.URL.Path,
		UserAgent: r.UserAgent(),
	}
}

func publishMissionControlVisit(payload missionControlVisitPayload) {
	apiKey := strings.TrimSpace(os.Getenv("MISSION_CONTROL_API_KEY"))
	if apiKey == "" {
		return
	}

	missionURL := envOrDefault(
		"MISSION_CONTROL_URL",
		defaultMissionControlURL,
	)

	eventID, err := newMissionControlEventID()
	if err != nil {
		log.Printf("Mission Control event ID generation failed: %v", err)
		return
	}

	event := missionControlEvent{
		EventID:       eventID,
		EventType:     missionControlEventType,
		SchemaVersion: 1,
		OccurredAt:    time.Now().UTC(),
		Payload:       payload,
	}

	body, err := json.Marshal(event)
	if err != nil {
		log.Printf("Mission Control telemetry marshal failed: %v", err)
		return
	}

	req, err := http.NewRequest(
		http.MethodPost,
		missionURL,
		bytes.NewReader(body),
	)
	if err != nil {
		log.Printf("Mission Control telemetry request failed: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mission-Control-Key", apiKey)

	resp, err := missionControlHTTPClient.Do(req)
	if err != nil {
		log.Printf("Mission Control telemetry publish failed: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf(
			"Mission Control telemetry rejected: HTTP %d",
			resp.StatusCode,
		)
	}
}

func newMissionControlEventID() (string, error) {
	var b [16]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	s := hex.EncodeToString(b[:])

	return s[0:8] + "-" +
		s[8:12] + "-" +
		s[12:16] + "-" +
		s[16:20] + "-" +
		s[20:32], nil
}
