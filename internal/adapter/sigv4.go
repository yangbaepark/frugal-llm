package adapter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// SignBedrockRequest creates the AWS SigV4 signed headers for a Bedrock request.
func SignBedrockRequest(method, endpointURL string, body []byte, creds AWSCredentials, region string, signTime time.Time) (map[string]string, error) {
	parsedURL, err := url.Parse(endpointURL)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint URL: %w", err)
	}

	amzDate := signTime.UTC().Format("20060102T150405Z")
	dateStamp := signTime.UTC().Format("20060102")

	payloadHashBytes := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])

	// Canonical headers map (keys must be lowercase)
	canonicalURI := parsedURL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}

	host := parsedURL.Host
	if host == "" {
		return nil, fmt.Errorf("endpoint URL missing host")
	}

	canonicalQuery := parsedURL.RawQuery

	// Prepare signed headers in alphabetical order
	var signedHeadersList []string
	var canonicalHeadersBuilder strings.Builder

	canonicalHeadersBuilder.WriteString(fmt.Sprintf("content-type:application/json\nhost:%s\nx-amz-date:%s\n", host, amzDate))
	signedHeadersList = append(signedHeadersList, "content-type", "host", "x-amz-date")

	if creds.SessionToken != "" {
		canonicalHeadersBuilder.WriteString(fmt.Sprintf("x-amz-security-token:%s\n", creds.SessionToken))
		signedHeadersList = append(signedHeadersList, "x-amz-security-token")
	}

	signedHeaders := strings.Join(signedHeadersList, ";")

	// 1. Create Canonical Request
	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		method,
		canonicalURI,
		canonicalQuery,
		canonicalHeadersBuilder.String(),
		signedHeaders,
		payloadHash,
	)

	canonicalRequestHashBytes := sha256.Sum256([]byte(canonicalRequest))
	canonicalRequestHash := hex.EncodeToString(canonicalRequestHashBytes[:])

	// 2. Create String-to-Sign
	credentialScope := fmt.Sprintf("%s/%s/bedrock/aws4_request", dateStamp, region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		canonicalRequestHash,
	)

	// 3. Calculate Signature
	signingKey := getSignatureKey(creds.SecretAccessKey, dateStamp, region, "bedrock")
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// 4. Construct Authorization Header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID,
		credentialScope,
		signedHeaders,
		signature,
	)

	headers := map[string]string{
		"Content-Type":         "application/json",
		"X-Amz-Date":           amzDate,
		"x-amz-content-sha256": payloadHash,
		"Authorization":        authHeader,
	}

	if creds.SessionToken != "" {
		headers["X-Amz-Security-Token"] = creds.SessionToken
	}

	return headers, nil
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func getSignatureKey(key, dateStamp, regionName, serviceName string) []byte {
	kSecret := []byte("AWS4" + key)
	kDate := hmacSHA256(kSecret, []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(regionName))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	return kSigning
}
