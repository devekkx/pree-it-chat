package userclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("user-client")

type UserProfile struct {
	ID          uuid.UUID `json:"id"`
	Username    string    `json:"username"`
	DisplayName *string   `json:"display_name"`
	AvatarURL   *string   `json:"avatar_url"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *Client) BatchGetProfiles(ctx context.Context, userIDs []uuid.UUID) (map[uuid.UUID]UserProfile, error) {
	ctx, span := tracer.Start(ctx, "userclient.BatchGetProfiles")
	defer span.End()

	span.SetAttributes(attribute.Int("user_count", len(userIDs)))

	if len(userIDs) == 0 {
		return map[uuid.UUID]UserProfile{}, nil
	}

	type batchReq struct {
		UserIDs []uuid.UUID `json:"user_ids"`
	}

	body, err := json.Marshal(batchReq{UserIDs: userIDs})
	if err != nil {
		return nil, fmt.Errorf("marshal batch request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/users/batch", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch request returned %d", resp.StatusCode)
	}

	type batchResp struct {
		Profiles []UserProfile `json:"profiles"`
	}

	var result batchResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode batch response: %w", err)
	}

	profiles := make(map[uuid.UUID]UserProfile, len(result.Profiles))
	for _, p := range result.Profiles {
		profiles[p.ID] = p
	}

	return profiles, nil
}
