package adapter

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Environment variable names for AWS Bedrock integration with FRUGAL_LLM_ prefix
const (
	EnvBedrockAccessKeyID     = "FRUGAL_LLM_AWS_ACCESS_KEY_ID"
	EnvBedrockSecretAccessKey = "FRUGAL_LLM_AWS_SECRET_ACCESS_KEY"
	EnvBedrockSessionToken    = "FRUGAL_LLM_AWS_SESSION_TOKEN"
	EnvBedrockRegion          = "FRUGAL_LLM_AWS_REGION"
	EnvBedrockProfile         = "FRUGAL_LLM_AWS_PROFILE"
)

// AWSCredentials contains resolved AWS authentication credentials.
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

// BedrockOptions configures AWS Bedrock provider settings in YAML.
type BedrockOptions struct {
	Region          string `yaml:"region,omitempty"`
	AccessKeyID     string `yaml:"access_key_id,omitempty"`
	SecretAccessKey string `yaml:"secret_access_key,omitempty"`
	SessionToken    string `yaml:"session_token,omitempty"`
	Profile         string `yaml:"profile,omitempty"`
}

// AWSCredentialsProvider defines an interface for retrieving AWS credentials.
type AWSCredentialsProvider interface {
	GetCredentials() (AWSCredentials, error)
}

// DynamicAWSCredentialsProvider dynamically resolves credentials following strict priority:
// Priority 1: Config options (fed via YAML or expanded from environment variables)
//             Must have region, access_key_id, and secret_access_key; session_token is optional.
// Priority 2: Profile (profile in credentials file must contain access_key_id and secret_access_key; config file must contain region)
// Priority 3: Skip Bedrock if neither is defined.
type DynamicAWSCredentialsProvider struct {
	opts            BedrockOptions
	credentialsPath string
	configPath      string
	profile         string

	mu          sync.RWMutex
	cachedCreds AWSCredentials
	lastModTime time.Time
}

// NewDynamicAWSCredentialsProvider creates a provider watching the specified options and standard AWS credential files.
func NewDynamicAWSCredentialsProvider(opts *BedrockOptions) *DynamicAWSCredentialsProvider {
	var bo BedrockOptions
	if opts != nil {
		bo = *opts
	}

	profile := strings.TrimSpace(bo.Profile)
	if profile == "" {
		profile = "default"
	}

	homeDir, _ := os.UserHomeDir()
	credPath := filepath.Join(homeDir, ".aws", "credentials")
	confPath := filepath.Join(homeDir, ".aws", "config")

	return &DynamicAWSCredentialsProvider{
		opts:            bo,
		credentialsPath: credPath,
		configPath:      confPath,
		profile:         profile,
	}
}

// GetCredentials retrieves credentials adhering to the priority order.
func (p *DynamicAWSCredentialsProvider) GetCredentials() (AWSCredentials, error) {
	// Priority 1: Config contains credentials (must have region, access_key_id, and secret_access_key; session_token is optional)
	keyID := strings.TrimSpace(p.opts.AccessKeyID)
	secret := strings.TrimSpace(p.opts.SecretAccessKey)
	region := strings.TrimSpace(p.opts.Region)
	token := strings.TrimSpace(p.opts.SessionToken)

	if keyID != "" && secret != "" && region != "" {
		return AWSCredentials{
			AccessKeyID:     keyID,
			SecretAccessKey: secret,
			SessionToken:    token,
		}, nil
	}

	// Priority 2: Fall back to profile
	// (profile must contain access_key_id and secret_access_key; config must contain region)
	confRegion, err := parseAWSConfigRegion(p.configPath, p.profile)
	if err != nil || strings.TrimSpace(confRegion) == "" {
		return AWSCredentials{}, fmt.Errorf("bedrock skipped: config credentials incomplete and AWS config '%s' profile '%s' missing region", p.configPath, p.profile)
	}

	p.mu.RLock()
	cached := p.cachedCreds
	lastMod := p.lastModTime
	p.mu.RUnlock()

	fi, err := os.Stat(p.credentialsPath)
	if err != nil {
		if cached.AccessKeyID != "" && cached.SecretAccessKey != "" {
			return cached, nil
		}
		return AWSCredentials{}, fmt.Errorf("bedrock skipped: config credentials incomplete and AWS credentials '%s' not found: %w", p.credentialsPath, err)
	}

	modTime := fi.ModTime()
	if !modTime.Equal(lastMod) || cached.AccessKeyID == "" {
		p.mu.Lock()
		defer p.mu.Unlock()

		// Re-check under write lock
		fi, err := os.Stat(p.credentialsPath)
		if err == nil {
			loadedCreds, err := parseAWSCredentialsFile(p.credentialsPath, p.profile)
			if err == nil && loadedCreds.AccessKeyID != "" && loadedCreds.SecretAccessKey != "" {
				p.cachedCreds = loadedCreds
				p.lastModTime = fi.ModTime()
				return loadedCreds, nil
			}
		}

		if p.cachedCreds.AccessKeyID != "" && p.cachedCreds.SecretAccessKey != "" {
			return p.cachedCreds, nil
		}
		return AWSCredentials{}, fmt.Errorf("bedrock skipped: AWS profile '%s' in '%s' missing access_key_id or secret_access_key", p.profile, p.credentialsPath)
	}

	return cached, nil
}

// ResolveBedrockRegion determines the active AWS region for Bedrock:
// Priority 1: Config option (opts.Region)
// Priority 2: AWS Profile (config must contain region for profile)
// Priority 3: Returns empty string if neither is defined (Bedrock skipped).
func ResolveBedrockRegion(opts *BedrockOptions) string {
	// Priority 1: Explicit option from config
	if opts != nil && strings.TrimSpace(opts.Region) != "" {
		return strings.TrimSpace(opts.Region)
	}

	// Priority 2: AWS Profile config
	profile := ""
	if opts != nil {
		profile = strings.TrimSpace(opts.Profile)
	}
	if profile == "" {
		profile = "default"
	}

	homeDir, _ := os.UserHomeDir()
	confPath := filepath.Join(homeDir, ".aws", "config")
	if configRegion, err := parseAWSConfigRegion(confPath, profile); err == nil && strings.TrimSpace(configRegion) != "" {
		return strings.TrimSpace(configRegion)
	}

	// Priority 3: Neither defined -> return empty string (no fallback default)
	return ""
}

// parseAWSCredentialsFile parses an INI-style AWS credentials file for a specific profile section.
func parseAWSCredentialsFile(path, targetProfile string) (AWSCredentials, error) {
	file, err := os.Open(path)
	if err != nil {
		return AWSCredentials{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentProfile := ""
	var creds AWSCredentials

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			rawSection := strings.TrimSpace(line[1 : len(line)-1])
			// Section in credentials file is usually "[default]" or "[my-profile]"
			currentProfile = rawSection
			continue
		}

		if strings.EqualFold(currentProfile, targetProfile) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				val := strings.TrimSpace(parts[1])
				switch key {
				case "aws_access_key_id":
					creds.AccessKeyID = val
				case "aws_secret_access_key":
					creds.SecretAccessKey = val
				case "aws_session_token":
					creds.SessionToken = val
				}
			}
		}
	}

	if creds.AccessKeyID == "" || creds.SecretAccessKey == "" {
		return AWSCredentials{}, fmt.Errorf("profile '%s' missing aws_access_key_id or aws_secret_access_key", targetProfile)
	}

	return creds, scanner.Err()
}

// parseAWSConfigRegion reads target profile from ~/.aws/config to discover region.
func parseAWSConfigRegion(path, targetProfile string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentProfile := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			rawSection := strings.TrimSpace(line[1 : len(line)-1])
			// In ~/.aws/config, sections are often "[default]" or "[profile my-profile]"
			if strings.HasPrefix(rawSection, "profile ") {
				currentProfile = strings.TrimSpace(strings.TrimPrefix(rawSection, "profile "))
			} else {
				currentProfile = rawSection
			}
			continue
		}

		if strings.EqualFold(currentProfile, targetProfile) {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.ToLower(strings.TrimSpace(parts[0]))
				val := strings.TrimSpace(parts[1])
				if key == "region" {
					return val, nil
				}
			}
		}
	}
	return "", scanner.Err()
}
