package arsenal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ovh/cds/sdk"
	"github.com/ovh/cds/sdk/cdsclient"
)

const (
	arsenalDeploymentTokenHeader = "X-Arsenal-Deployment-Token"
	arsenalFollowupTokenHeader   = "X-Arsenal-Followup-Token"

	arsenalGatewaySourceHeader      = "X-Ovh-Gateway-Source"
	arsenalGatewayTokenHeader       = "X-Ovh-Gateway-Token"
	arsenalGatewayServiceNameHeader = "X-Ovh-Gateway-Service-Name"
)

// Alternative represents an alternative to a deployment.
type Alternative struct {
	Name    string                 `json:"name"`
	From    string                 `json:"from,omitempty"`
	Config  map[string]interface{} `json:"config"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// DeployRequest represents a deploy request to arsenal.
type DeployRequest struct {
	Version     string            `json:"version"`
	Alternative string            `json:"alternative,omitempty"`
	Metadata    map[string]string `json:"metadata"`
}

type DeployResponse struct {
	FollowUpToken    string `json:"followup_token"`
	DeploymentName   string `json:"deployment_name"`
	DeploymentID     string `json:"deployment_id"`
	DeploymentFamily string `json:"deployment_family"`
	StackName        string `json:"stack_name"`
	StackID          string `json:"stack_id"`
	StackPlatform    string `json:"stack_platform"`
	Namespace        string `json:"namespace"`
}

// String returns a string representation of a deploy request. Omits metadata.
func (r *DeployRequest) String() string {
	s := "Version: " + r.Version
	if r.Alternative != "" {
		s += "; Alternative: " + r.Alternative
	}
	return s
}

// FollowupState is the followup status of a deploy request.
type FollowupState struct {
	Done     bool    `json:"done"`
	Progress float64 `json:"progress"`
}

// Client is a helper client to call arsenal public API.
type Client struct {
	client *http.Client
	conf   Conf
}

// RequestError represents an error from a HTTP 4XX status
type RequestError struct {
	msg string
}

func (r *RequestError) Error() string {
	return r.msg
}

type Conf struct {
	Host            string
	DeploymentToken string
	GWServiceName   string
	GWTokenSource   string
	GWTokenSecret   string
}

// NewClient creates a new client to call Arsenal public routes with a given host and deploymentToken.
func NewClient(conf Conf) *Client {
	return &Client{
		client: cdsclient.NewHTTPClient(60*time.Second, false),
		conf:   conf,
	}
}

func (ac *Client) addHeaders(req *http.Request) {
	req.Header.Add(arsenalDeploymentTokenHeader, ac.conf.DeploymentToken)
	if ac.conf.GWTokenSource != "" {
		req.Header.Add(arsenalGatewaySourceHeader, ac.conf.GWTokenSource)
	}
	if ac.conf.GWTokenSecret != "" {
		req.Header.Add(arsenalGatewayTokenHeader, ac.conf.GWTokenSecret)
	}
	if ac.conf.GWServiceName != "" {
		req.Header.Add(arsenalGatewayServiceNameHeader, "arsenal")
	}
}

// Deploy makes a deploy request and returns a followup token if successful.
func (ac *Client) Deploy(deployRequest *DeployRequest) (*DeployResponse, error) {
	req, err := ac.newRequest(http.MethodPost, "/deploy", deployRequest)
	if err != nil {
		return nil, err
	}
	ac.addHeaders(req)

	var (
		deployResult DeployResponse
		nbRetry      int
		lastErr      error
	)
	for ; nbRetry < 5; nbRetry++ {
		lastErr = nil
		statusCode, rawBody, err := ac.doRequest(req, &deployResult)
		if err != nil {
			return nil, err
		}
		if statusCode == http.StatusOK {
			return &deployResult, lastErr
		}
		if statusCode == http.StatusMethodNotAllowed {
			lastErr = &RequestError{fmt.Sprintf("deploy request failed (HTTP status %d): %s", statusCode, rawBody)}
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if statusCode == http.StatusBadRequest {
			return nil, &RequestError{fmt.Sprintf("deploy request failed (HTTP status %d): %s", statusCode, rawBody)}
		}
		if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
			return nil, fmt.Errorf("deploy request failed (HTTP status %d): %s", statusCode, rawBody)
		}
		return nil, fmt.Errorf("cannot reach Arsenal service (HTTP status %d)", statusCode)
	}

	return &deployResult, lastErr
}

// Follow makes a followup request with a followup token.
func (ac *Client) Follow(followupToken string) (*FollowupState, error) {
	req, err := ac.newRequest(http.MethodGet, "/follow", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Follow request: %w", err)
	}
	req.Header.Add(arsenalFollowupTokenHeader, followupToken)
	ac.addHeaders(req)

	state := &FollowupState{}
	statusCode, rawBody, err := ac.doRequest(req, state)
	if err != nil {
		return nil, err
	}

	if statusCode != http.StatusOK {
		if statusCode == http.StatusServiceUnavailable {
			return nil, nil
		}
		return nil, fmt.Errorf("failed Follow request (HTTP status %d): %s", statusCode, rawBody)
	}
	return state, nil
}

// UpsertAlternative creates or updates an alternative.
func (ac *Client) UpsertAlternative(altConfig *Alternative) error {
	req, err := ac.newRequest(http.MethodPost, "/alternative", altConfig)
	if err != nil {
		return fmt.Errorf("failed to create upsert alternative request: %w", err)
	}
	ac.addHeaders(req)

	statusCode, rawBody, err := ac.doRequest(req, nil)
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
			return fmt.Errorf("failed upsert alternative request (HTTP status %d): %s", statusCode, rawBody)
		}
		return fmt.Errorf("cannot reach Arsenal service (HTTP status %d)", statusCode)
	}
	return nil
}

// DeleteAlternative deletes an existing alternative.
func (ac *Client) DeleteAlternative(altName string) error {
	req, err := ac.newRequest(http.MethodDelete, "/alternative/"+url.PathEscape(altName), nil)
	if err != nil {
		return fmt.Errorf("failed to create delete alternative request: %w", err)
	}
	ac.addHeaders(req)

	statusCode, rawBody, err := ac.doRequest(req, nil)
	if err != nil {
		return err
	}
	if statusCode != http.StatusOK {
		if statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError {
			return fmt.Errorf("failed delete alternative request (HTTP status %d): %s", statusCode, rawBody)
		}
		return fmt.Errorf("cannot reach Arsenal service (HTTP status %d)", statusCode)
	}
	return nil
}

func (ac *Client) newRequest(method, uri string, obj interface{}) (*http.Request, error) {
	var body io.ReadCloser
	if obj != nil {
		objData, err := json.Marshal(obj)
		if err != nil {
			return nil, fmt.Errorf("unable to encode request body: %w", err)
		}
		body = io.NopCloser(bytes.NewReader(objData))
	}

	req, err := http.NewRequest(method, ac.conf.Host+uri, body)
	if err != nil {
		return nil, fmt.Errorf("unable to prepare request on %s %s: %v", method, uri, err)
	}
	return req, nil
}

func (ac *Client) doRequest(req *http.Request, respObject interface{}) (int, []byte, error) {
	resp, err := ac.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("%s %s failed: %w", req.Method, req.URL, err)
	}
	defer resp.Body.Close()

	rawBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, fmt.Errorf("failed to read body from %s %s: %w", req.Method, req.URL, err)
	}

	if resp.StatusCode == http.StatusOK && respObject != nil {
		err = sdk.JSONUnmarshal(rawBody, respObject)
		if err != nil {
			return resp.StatusCode, nil, fmt.Errorf("failed to decode body from %s %s: %w", req.Method, req.URL, err)
		}
	}

	return resp.StatusCode, rawBody, nil
}
