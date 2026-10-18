package license

import (
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"sort"
	"time"
)

// VerifyLocalFile reads and verifies the local signed license file
// using the embedded public key. Returns the license info if valid.
func VerifyLocalFile(filePath, publicKeyPEM string) (*LicenseInfo, error) {
	if publicKeyPEM == "" {
		return nil, fmt.Errorf("public key is not configured")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read license file: %w", err)
	}

	var signed SignedServerLicense
	if err := json.Unmarshal(data, &signed); err != nil {
		return nil, fmt.Errorf("failed to parse license file: %w", err)
	}

	if !signed.Signed || signed.License == nil {
		return nil, fmt.Errorf("invalid license file format")
	}

	if err := verifySignature(publicKeyPEM, signed.License, signed.Signature); err != nil {
		return nil, fmt.Errorf("license signature verification failed: %w", err)
	}

	expiresAt, err := time.Parse(time.RFC3339, signed.License.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("invalid expiration date: %w", err)
	}
	if time.Now().After(expiresAt) {
		return nil, fmt.Errorf("license has expired at %s", signed.License.ExpiresAt)
	}
	// 试用许可证的 TrialEndsAt 是硬截止，常早于 ExpiresAt；不强制则试用期形同虚设。
	if signed.License.IsTrial && signed.License.TrialEndsAt != "" {
		trialEnds, terr := time.Parse(time.RFC3339, signed.License.TrialEndsAt)
		if terr != nil {
			// fail-closed：试用证 TrialEndsAt 解析失败不能放行，否则构造畸形日期即可绕过试用期。
			return nil, fmt.Errorf("invalid trial end date %q: %w", signed.License.TrialEndsAt, terr)
		}
		if time.Now().After(trialEnds) {
			return nil, fmt.Errorf("trial license ended at %s", signed.License.TrialEndsAt)
		}
	}

	info := &LicenseInfo{
		LicenseID:      signed.License.LicenseID,
		VendorID:       signed.License.VendorID,
		ProductLine:    signed.License.ProductLine,
		ProductCode:    signed.License.ProductCode,
		Edition:        signed.License.Edition,
		ExpiresAt:      signed.License.ExpiresAt,
		Features:       signed.License.Features,
		Limits:         signed.License.Limits,
		MaxNodes:       signed.License.MaxNodes,
		MaxUsers:       signed.License.MaxUsers,
		MaxServers:     signed.License.MaxServers,
		BoundDomains:   signed.License.BoundDomains,
		ServerURL:      signed.License.ServerURL,
		MaxEndUsers:    signed.License.MaxEndUsers,
		MaxTenants:     signed.License.MaxTenants,
		BoundServerIDs: signed.License.BoundServerIDs,
		BoundHostnames: signed.License.BoundHostnames,
		BoundIPs:       signed.License.BoundIPs,
		IsTrial:        signed.License.IsTrial,
		TrialEndsAt:    signed.License.TrialEndsAt,
		LicenseSerial:  signed.License.LicenseSerial,
		VerifiedAt:     time.Now(),
	}

	return info, nil
}

// UpdateLocalFile writes the latest license info back to the local file
func UpdateLocalFile(filePath string, signedData []byte) error {
	var check SignedServerLicense
	if err := json.Unmarshal(signedData, &check); err != nil {
		return fmt.Errorf("invalid signed license data: %w", err)
	}
	if !check.Signed || check.License == nil {
		return fmt.Errorf("invalid signed license format")
	}

	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, signedData, 0600); err != nil {
		return fmt.Errorf("failed to write license file: %w", err)
	}
	if err := os.Rename(tmpPath, filePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to finalize license file: %w", err)
	}
	return nil
}

func verifySignature(publicKeyPEM string, data interface{}, signatureBase64 string) error {
	block, _ := pem.Decode([]byte(publicKeyPEM))
	if block == nil {
		return fmt.Errorf("failed to decode public key PEM")
	}

	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("public key is not RSA")
	}

	dataBytes, err := canonicalJSON(data)
	if err != nil {
		return fmt.Errorf("failed to serialize data: %w", err)
	}

	signature, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	hash := sha256.Sum256(dataBytes)
	return rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, hash[:], signature)
}

func canonicalJSON(v interface{}) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}

	return orderedMarshal(parsed)
}

func orderedMarshal(v interface{}) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}

	switch val := v.(type) {
	case map[string]interface{}:
		return orderedMarshalMap(val)
	case []interface{}:
		return orderedMarshalArray(val)
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
		b := buf.Bytes()
		if len(b) > 0 && b[len(b)-1] == '\n' {
			b = b[:len(b)-1]
		}
		return b, nil
	}
}

func orderedMarshalMap(m map[string]interface{}) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}

	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := orderedMarshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		valueJSON, err := orderedMarshal(m[k])
		if err != nil {
			return nil, err
		}
		buf.Write(valueJSON)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func orderedMarshalArray(arr []interface{}) ([]byte, error) {
	if len(arr) == 0 {
		return []byte("[]"), nil
	}

	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, item := range arr {
		if i > 0 {
			buf.WriteByte(',')
		}
		itemJSON, err := orderedMarshal(item)
		if err != nil {
			return nil, err
		}
		buf.Write(itemJSON)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}
