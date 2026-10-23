package adapter

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"frugal-llm/internal/model"
)

func TestCreateAdapterOpenAI(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("openai", "openai", "https://api.openai.com/v1", "", nil)
	if active {
		t.Errorf("Expected remote OpenAI without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("openai", "openai", "https://api.openai.com/v1", "sk-test-key", nil)
	if !active || ad == nil {
		t.Fatalf("Expected remote OpenAI with API key to be active")
	}
	if ad.Name() != "OpenAI" {
		t.Errorf("Expected adapter name 'OpenAI', got '%s'", ad.Name())
	}

	// Local provider without API key -> active
	localAd, localActive := CreateAdapter("local", "openai", "http://localhost:1234/v1", "", nil)
	if !localActive || localAd == nil {
		t.Errorf("Expected local generic provider to be active without API key")
	}
}

func TestCreateAdapterAnthropic(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("anthropic", "anthropic", "https://api.anthropic.com/v1", "", nil)
	if active {
		t.Errorf("Expected Anthropic without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("anthropic", "anthropic", "https://api.anthropic.com/v1", "sk-ant-test", nil)
	if !active || ad == nil {
		t.Fatalf("Expected Anthropic with API key to be active")
	}
	if ad.Name() != "Anthropic" {
		t.Errorf("Expected adapter name 'Anthropic', got '%s'", ad.Name())
	}
}

func TestCreateAdapterOllama(t *testing.T) {
	// Empty BaseURL -> inactive
	_, active := CreateAdapter("ollama", "ollama", "", "", nil)
	if active {
		t.Errorf("Expected Ollama with empty BaseURL to be inactive")
	}

	// Non-empty BaseURL -> active
	ad, active := CreateAdapter("ollama", "ollama", "http://localhost:11434", "", nil)
	if !active || ad == nil {
		t.Fatalf("Expected Ollama with BaseURL to be active")
	}
	if ad.Name() != "Ollama" {
		t.Errorf("Expected adapter name 'Ollama', got '%s'", ad.Name())
	}
}

func TestCreateAdapterHuggingFace(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("huggingface", "huggingface", "https://api-inference.huggingface.co/models", "", nil)
	if active {
		t.Errorf("Expected HuggingFace without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("huggingface", "huggingface", "https://api-inference.huggingface.co/models", "hf_test", nil)
	if !active || ad == nil {
		t.Fatalf("Expected HuggingFace with API key to be active")
	}
	if ad.Name() != "HuggingFace" {
		t.Errorf("Expected adapter name 'HuggingFace', got '%s'", ad.Name())
	}
}

func TestCreateAdapterAzure(t *testing.T) {
	// Remote without API key -> inactive
	_, active := CreateAdapter("azure", "azure", "https://my-resource.openai.azure.com/openai/v1", "", nil)
	if active {
		t.Errorf("Expected Azure without API key to be inactive")
	}

	// Remote with API key -> active
	ad, active := CreateAdapter("azure", "azure", "https://my-resource.openai.azure.com/openai/v1", "az-test-key", nil)
	if !active || ad == nil {
		t.Fatalf("Expected Azure with API key to be active")
	}
	if ad.Name() != "Azure" {
		t.Errorf("Expected adapter name 'Azure', got '%s'", ad.Name())
	}

	// BuildRequest verification
	req := &model.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []model.ChatMessage{
			{Role: "user", Content: "Hello Azure"},
		},
	}
	body, targetURL, headers, err := ad.BuildRequest(req)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}
	if !strings.HasSuffix(targetURL, "/chat/completions") {
		t.Errorf("Expected targetURL to end with /chat/completions, got '%s'", targetURL)
	}
	if headers["api-key"] != "az-test-key" {
		t.Errorf("Expected header api-key to be 'az-test-key', got '%s'", headers["api-key"])
	}
	if len(body) == 0 {
		t.Errorf("Expected non-empty body")
	}
}

func TestSignBedrockRequest(t *testing.T) {
	creds := AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "test-session-token",
	}

	endpoint := "https://bedrock-runtime.us-west-2.amazonaws.com/model/anthropic.claude-sonnet-5-5:0/converse"
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"Hello"}]}]}`)
	signTime := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	headers, err := SignBedrockRequest("POST", endpoint, body, creds, "us-west-2", signTime)
	if err != nil {
		t.Fatalf("SignBedrockRequest failed: %v", err)
	}

	if headers["X-Amz-Date"] != "20261019T120000Z" {
		t.Errorf("Expected X-Amz-Date '20261019T120000Z', got '%s'", headers["X-Amz-Date"])
	}
	if headers["X-Amz-Security-Token"] != "test-session-token" {
		t.Errorf("Expected X-Amz-Security-Token 'test-session-token', got '%s'", headers["X-Amz-Security-Token"])
	}
	auth := headers["Authorization"]
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20261019/us-west-2/bedrock/aws4_request") {
		t.Errorf("Authorization header malformed: %s", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token") {
		t.Errorf("Expected SignedHeaders to include session token, got: %s", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Errorf("Authorization header missing Signature: %s", auth)
	}
}

func TestDynamicAWSCredentialsFileWatching(t *testing.T) {
	// Create temporary directory simulating ~/.aws
	tmpDir, err := os.MkdirTemp("", "aws_test_*")
	if err != nil {
		t.Fatalf("Failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	credFile := filepath.Join(tmpDir, "credentials")
	configFile := filepath.Join(tmpDir, "config")

	initialCreds := `
[default]
aws_access_key_id = AKIA_INITIAL_KEY
aws_secret_access_key = SECRET_INITIAL_KEY
aws_session_token = TOKEN_INITIAL
`
	if err := os.WriteFile(credFile, []byte(initialCreds), 0600); err != nil {
		t.Fatalf("Failed writing initial creds: %v", err)
	}

	initialConfig := `
[default]
region = us-west-2
`
	if err := os.WriteFile(configFile, []byte(initialConfig), 0600); err != nil {
		t.Fatalf("Failed writing initial config: %v", err)
	}

	provider := &DynamicAWSCredentialsProvider{
		credentialsPath: credFile,
		configPath:      configFile,
		profile:         "default",
	}

	// 1. Initial load
	creds, err := provider.GetCredentials()
	if err != nil {
		t.Fatalf("GetCredentials failed: %v", err)
	}
	if creds.AccessKeyID != "AKIA_INITIAL_KEY" {
		t.Errorf("Expected AccessKeyID 'AKIA_INITIAL_KEY', got '%s'", creds.AccessKeyID)
	}

	// 2. Simulate background thread / SSO refresh updating ~/.aws/credentials
	time.Sleep(10 * time.Millisecond) // Ensure mod time difference
	updatedCreds := `
[default]
aws_access_key_id = ASIA_REFRESHED_KEY
aws_secret_access_key = SECRET_REFRESHED_KEY
aws_session_token = TOKEN_REFRESHED
`
	if err := os.WriteFile(credFile, []byte(updatedCreds), 0600); err != nil {
		t.Fatalf("Failed writing updated creds: %v", err)
	}

	// 3. Hot-reload check: GetCredentials should immediately pick up refreshed credentials
	freshCreds, err := provider.GetCredentials()
	if err != nil {
		t.Fatalf("GetCredentials failed on refresh: %v", err)
	}
	if freshCreds.AccessKeyID != "ASIA_REFRESHED_KEY" {
		t.Errorf("Expected refreshed AccessKeyID 'ASIA_REFRESHED_KEY', got '%s'", freshCreds.AccessKeyID)
	}
	if freshCreds.SecretAccessKey != "SECRET_REFRESHED_KEY" {
		t.Errorf("Expected refreshed SecretAccessKey 'SECRET_REFRESHED_KEY', got '%s'", freshCreds.SecretAccessKey)
	}
	if freshCreds.SessionToken != "TOKEN_REFRESHED" {
		t.Errorf("Expected refreshed SessionToken 'TOKEN_REFRESHED', got '%s'", freshCreds.SessionToken)
	}
}

func TestBedrockCredentialsResolutionHierarchy(t *testing.T) {
	// A. Priority 1: Config options (must have region, access_key_id, secret_access_key)
	opts := &BedrockOptions{
		AccessKeyID:     "AKIA_OPTS_KEY",
		SecretAccessKey: "SECRET_OPTS_KEY",
		Region:          "us-west-2",
		SessionToken:    "TOKEN_OPTS",
	}

	provider := NewDynamicAWSCredentialsProvider(opts)
	creds, err := provider.GetCredentials()
	if err != nil {
		t.Fatalf("GetCredentials with opts failed: %v", err)
	}
	if creds.AccessKeyID != "AKIA_OPTS_KEY" || creds.SecretAccessKey != "SECRET_OPTS_KEY" || creds.SessionToken != "TOKEN_OPTS" {
		t.Errorf("Unexpected credentials from opts: %+v", creds)
	}

	// Priority 1 fails if region is missing and no profile config exists -> skips Bedrock
	optsMissingRegion := &BedrockOptions{
		AccessKeyID:     "AKIA_OPTS_KEY",
		SecretAccessKey: "SECRET_OPTS_KEY",
	}
	emptyProvider := &DynamicAWSCredentialsProvider{
		opts:            *optsMissingRegion,
		credentialsPath: "/tmp/non_existent_credentials_path_xyz",
		configPath:      "/tmp/non_existent_config_path_xyz",
		profile:         "default",
	}
	_, err = emptyProvider.GetCredentials()
	if err == nil {
		t.Fatalf("Expected GetCredentials to fail when region is missing, but it succeeded")
	}

	// B. Ambient environment variables must be ignored by adapter directly
	t.Setenv("AWS_ACCESS_KEY_ID", "UNPREFIXED_KEY")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "UNPREFIXED_SECRET")
	t.Setenv("FRUGAL_LLM_AWS_ACCESS_KEY_ID", "UNEXPANDED_KEY")

	noOptsProvider := &DynamicAWSCredentialsProvider{
		credentialsPath: "/tmp/non_existent_credentials_path_xyz",
		configPath:      "/tmp/non_existent_config_path_xyz",
		profile:         "default",
	}
	_, err = noOptsProvider.GetCredentials()
	if err == nil {
		t.Fatalf("Expected GetCredentials to ignore environment variables and fail, but it succeeded")
	}

	// C. Priority 2: Profile (credentials must have keys, config must have region)
	tmpDir, err := os.MkdirTemp("", "aws_hierarchy_*")
	if err != nil {
		t.Fatalf("Failed creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	credFile := filepath.Join(tmpDir, "credentials")
	configFile := filepath.Join(tmpDir, "config")

	_ = os.WriteFile(credFile, []byte("[default]\naws_access_key_id = AKIA_PROF_KEY\naws_secret_access_key = SECRET_PROF_KEY\n"), 0600)
	// Without config containing region, Priority 2 must fail
	profileProvider := &DynamicAWSCredentialsProvider{
		credentialsPath: credFile,
		configPath:      configFile,
		profile:         "default",
	}
	_, err = profileProvider.GetCredentials()
	if err == nil {
		t.Fatalf("Expected Priority 2 to fail when config missing region, but it succeeded")
	}

	// Add region to config -> Priority 2 succeeds
	_ = os.WriteFile(configFile, []byte("[default]\nregion = us-west-2\n"), 0600)
	profCreds, err := profileProvider.GetCredentials()
	if err != nil {
		t.Fatalf("Expected Priority 2 to succeed when config has region, failed: %v", err)
	}
	if profCreds.AccessKeyID != "AKIA_PROF_KEY" {
		t.Errorf("Expected AKIA_PROF_KEY, got '%s'", profCreds.AccessKeyID)
	}
}

func TestResolveBedrockRegion(t *testing.T) {
	// 1. Explicit option in BedrockOptions takes highest precedence
	opts := &BedrockOptions{Region: "ap-northeast-1"}
	if reg := ResolveBedrockRegion(opts); reg != "ap-northeast-1" {
		t.Errorf("Expected 'ap-northeast-1', got '%s'", reg)
	}

	// 2. Fallback when neither opts nor config is defined returns empty string (skips Bedrock)
	optsEmpty := &BedrockOptions{Profile: "non-existent-profile-xyz"}
	if reg := ResolveBedrockRegion(optsEmpty); reg != "" {
		t.Errorf("Expected empty string when region is undefined, got '%s'", reg)
	}
}

func TestBedrockConverseMapping(t *testing.T) {
	opts := &BedrockOptions{
		Region:          "us-west-2",
		AccessKeyID:     "TEST_KEY",
		SecretAccessKey: "TEST_SECRET",
	}

	ad, active := CreateAdapter("bedrock", "bedrock", "", "", opts)
	if !active || ad == nil {
		t.Fatalf("Expected Bedrock with options to be active")
	}

	req := &model.ChatCompletionRequest{
		Model: "anthropic.claude-sonnet-5-5:0",
		Messages: []model.ChatMessage{
			{Role: "system", Content: "You are an AI assistant."},
			{Role: "user", Content: "Write a poem."},
		},
		MaxTokens: 500,
	}

	body, targetURL, headers, err := ad.BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest failed: %v", err)
	}

	expectedURL := "https://bedrock-runtime.us-west-2.amazonaws.com/model/anthropic.claude-sonnet-5-5:0/converse"
	if targetURL != expectedURL {
		t.Errorf("Expected targetURL '%s', got '%s'", expectedURL, targetURL)
	}
	if headers["Authorization"] == "" || headers["X-Amz-Date"] == "" {
		t.Errorf("Missing SigV4 headers: %+v", headers)
	}
	if !strings.Contains(string(body), "You are an AI assistant.") {
		t.Errorf("Expected body to contain system prompt: %s", string(body))
	}
	if !strings.Contains(string(body), "Write a poem.") {
		t.Errorf("Expected body to contain user message: %s", string(body))
	}

	// Test TransformResponse
	mockRespJSON := `{
		"output": {
			"message": {
				"role": "assistant",
				"content": [{"text": "Roses are red, violets are blue."}]
			}
		},
		"stopReason": "end_turn",
		"usage": {
			"inputTokens": 10,
			"outputTokens": 8,
			"totalTokens": 18
		}
	}`

	httpResp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockRespJSON)),
	}

	chatResp, err := ad.TransformResponse(httpResp)
	if err != nil {
		t.Fatalf("TransformResponse failed: %v", err)
	}
	if len(chatResp.Choices) == 0 {
		t.Fatalf("Expected choices in response")
	}
	if chatResp.Choices[0].Message.Content != "Roses are red, violets are blue." {
		t.Errorf("Unexpected content: %s", chatResp.Choices[0].Message.Content)
	}
	if chatResp.Choices[0].FinishReason != "stop" {
		t.Errorf("Expected finish_reason 'stop', got '%s'", chatResp.Choices[0].FinishReason)
	}
	if chatResp.Usage.TotalTokens != 18 {
		t.Errorf("Expected total tokens 18, got %d", chatResp.Usage.TotalTokens)
	}
}

func TestBedrockEventStreamDecoder(t *testing.T) {
	// Create mock AWS EventStream frame
	payloadJSON := `{"contentBlockDelta":{"contentBlockIndex":0,"delta":{"text":"Hello Bedrock!"}}}`
	payloadBytes := []byte(payloadJSON)

	headersBytes := []byte{} // zero headers for test payload
	totalLen := uint32(16 + len(headersBytes) + len(payloadBytes))
	headersLen := uint32(len(headersBytes))

	var frame bytes.Buffer
	binary.Write(&frame, binary.BigEndian, totalLen)
	binary.Write(&frame, binary.BigEndian, headersLen)
	binary.Write(&frame, binary.BigEndian, uint32(0)) // Prelude CRC
	frame.Write(headersBytes)
	frame.Write(payloadBytes)
	binary.Write(&frame, binary.BigEndian, uint32(0)) // Message CRC

	rec := httptest.NewRecorder()
	err := StreamBedrockEventStreamToSSE(&frame, "claude-3-5", rec)
	if err != nil {
		t.Fatalf("StreamBedrockEventStreamToSSE failed: %v", err)
	}

	output := rec.Body.String()
	if !strings.Contains(output, "Hello Bedrock!") {
		t.Errorf("Expected output to contain delta text, got: %s", output)
	}
	if !strings.Contains(output, "data: [DONE]") {
		t.Errorf("Expected output to contain [DONE], got: %s", output)
	}
}
