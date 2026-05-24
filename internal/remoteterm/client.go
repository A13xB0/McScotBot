package remoteterm

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const MaxMessageLen = 155 // conservative limit leaving room for sender name prefix

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type channelResp struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type sendChannelReq struct {
	ChannelKey string `json:"channel_key"`
	Text       string `json:"text"`
}

type scopeOverrideReq struct {
	FloodScopeOverride string `json:"flood_scope_override"`
}

type createChannelReq struct {
	Name string `json:"name"`
}

func NewClient(baseURL string) *Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout:   15 * time.Second,
			Transport: transport,
		},
	}
}

// HashtagChannelKey computes the channel key for a hashtag channel name.
// Matches RemoteTerm: sha256(name.encode("utf-8"))[:16].hex().upper()
func HashtagChannelKey(name string) string {
	if !strings.HasPrefix(name, "#") {
		name = "#" + name
	}
	h := sha256.Sum256([]byte(name))
	return strings.ToUpper(hex.EncodeToString(h[:16]))
}

func (c *Client) HealthCheck() error {
	resp, err := c.httpClient.Get(c.baseURL + "/api/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) ListChannels() ([]channelResp, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/channels")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var channels []channelResp
	if err := json.Unmarshal(body, &channels); err != nil {
		return nil, fmt.Errorf("parsing channels response: %w", err)
	}
	return channels, nil
}

func (c *Client) CreateChannel(name string) error {
	payload, _ := json.Marshal(createChannelReq{Name: name})
	resp, err := c.httpClient.Post(c.baseURL+"/api/channels", "application/json", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create channel returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *Client) SetScopeOverride(channelKey, scope string) error {
	return c.postJSON(
		fmt.Sprintf("/api/channels/%s/flood-scope-override", channelKey),
		scopeOverrideReq{FloodScopeOverride: scope},
	)
}

func (c *Client) ClearScopeOverride(channelKey string) error {
	return c.postJSON(
		fmt.Sprintf("/api/channels/%s/flood-scope-override", channelKey),
		scopeOverrideReq{FloodScopeOverride: ""},
	)
}

func (c *Client) SendMessage(channelKey, text string) error {
	if len(text) > MaxMessageLen {
		text = text[:MaxMessageLen-1] + "…"
	}
	return c.postJSON("/api/messages/channel", sendChannelReq{
		ChannelKey: channelKey,
		Text:       text,
	})
}

func (c *Client) postJSON(path string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s returned %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}
