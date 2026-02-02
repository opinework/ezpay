package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"

	"github.com/shopspring/decimal"
)

// Base58 alphabet used by Bitcoin and Tron
const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

var base58Lookup = make(map[byte]int64)

func init() {
	// Initialize base58 lookup table
	for i, c := range base58Alphabet {
		base58Lookup[byte(c)] = int64(i)
	}
}

// base58Encode encodes bytes to base58 string
func base58Encode(input []byte) string {
	if len(input) == 0 {
		return ""
	}

	// Count leading zeros
	var numZeros int
	for _, b := range input {
		if b != 0 {
			break
		}
		numZeros++
	}

	// Convert to big integer
	num := new(big.Int).SetBytes(input)

	// Encode
	var encoded []byte
	base := big.NewInt(58)
	zero := big.NewInt(0)
	mod := new(big.Int)

	for num.Cmp(zero) > 0 {
		num.DivMod(num, base, mod)
		encoded = append(encoded, base58Alphabet[mod.Int64()])
	}

	// Add leading '1's for leading zeros
	for i := 0; i < numZeros; i++ {
		encoded = append(encoded, base58Alphabet[0])
	}

	// Reverse
	for i, j := 0, len(encoded)-1; i < j; i, j = i+1, j-1 {
		encoded[i], encoded[j] = encoded[j], encoded[i]
	}

	return string(encoded)
}

// base58Decode decodes base58 string to bytes
func base58Decode(input string) ([]byte, error) {
	if len(input) == 0 {
		return nil, errors.New("empty input")
	}

	// Count leading '1's
	var numZeros int
	for _, c := range input {
		if c != '1' {
			break
		}
		numZeros++
	}

	// Decode
	num := big.NewInt(0)
	base := big.NewInt(58)

	for _, c := range input {
		val, ok := base58Lookup[byte(c)]
		if !ok {
			return nil, errors.New("invalid base58 character")
		}
		num.Mul(num, base)
		num.Add(num, big.NewInt(val))
	}

	// Convert to bytes
	decoded := num.Bytes()

	// Add leading zeros
	result := make([]byte, numZeros+len(decoded))
	copy(result[numZeros:], decoded)

	return result, nil
}

// doubleSHA256 performs double SHA256 hash
func doubleSHA256(data []byte) []byte {
	hash1 := sha256.Sum256(data)
	hash2 := sha256.Sum256(hash1[:])
	return hash2[:]
}

// hexToBase58 converts Tron hex address (starting with 41) to base58 format (starting with T)
func hexToBase58(hexAddr string) string {
	// Remove 0x prefix if present
	hexAddr = strings.TrimPrefix(hexAddr, "0x")
	hexAddr = strings.TrimPrefix(hexAddr, "0X")

	// If already base58 format (starts with T), return as is
	if strings.HasPrefix(hexAddr, "T") {
		return hexAddr
	}

	// Ensure it starts with 41 (Tron mainnet)
	if !strings.HasPrefix(hexAddr, "41") {
		// Invalid Tron address
		return hexAddr
	}

	// Decode hex string to bytes
	addrBytes, err := hex.DecodeString(hexAddr)
	if err != nil {
		return hexAddr
	}

	// Calculate checksum: first 4 bytes of double SHA256
	checksum := doubleSHA256(addrBytes)[:4]

	// Append checksum to address bytes
	addrWithChecksum := append(addrBytes, checksum...)

	// Encode to base58
	return base58Encode(addrWithChecksum)
}

// base58ToHex converts Tron base58 address (starting with T) to hex format (starting with 41)
func base58ToHex(base58Addr string) (string, error) {
	if !strings.HasPrefix(base58Addr, "T") {
		// If already hex format
		if strings.HasPrefix(base58Addr, "41") {
			return base58Addr, nil
		}
		return "", errors.New("invalid Tron address")
	}

	// Decode base58
	decoded, err := base58Decode(base58Addr)
	if err != nil {
		return "", err
	}

	if len(decoded) != 25 {
		return "", errors.New("invalid address length")
	}

	// Split address and checksum
	addrBytes := decoded[:21]
	checksum := decoded[21:]

	// Verify checksum
	expectedChecksum := doubleSHA256(addrBytes)[:4]
	for i := 0; i < 4; i++ {
		if checksum[i] != expectedChecksum[i] {
			return "", errors.New("invalid checksum")
		}
	}

	return hex.EncodeToString(addrBytes), nil
}

// normalizeAddress normalizes address to lowercase and handles different formats
func normalizeAddress(addr string, chain string) string {
	addr = strings.TrimSpace(addr)

	// For Tron chains, ensure base58 format
	if chain == "trx" || chain == "trc20" {
		// If it's hex format, convert to base58
		if strings.HasPrefix(addr, "41") {
			addr = hexToBase58(addr)
		}
	}

	return strings.ToLower(addr)
}

// parseTokenAmount parses token amount string with decimals
func parseTokenAmount(value string, decimals int) decimal.Decimal {
	// Remove leading zeros
	value = strings.TrimLeft(value, "0")
	if value == "" {
		value = "0"
	}

	amount, err := decimal.NewFromString(value)
	if err != nil {
		return decimal.Zero
	}

	// Divide by 10^decimals
	divisor := decimal.NewFromInt(1)
	for i := 0; i < decimals; i++ {
		divisor = divisor.Mul(decimal.NewFromInt(10))
	}

	return amount.Div(divisor)
}

// parseHexAmount parses hex amount string
func parseHexAmount(hexValue string, decimals int) decimal.Decimal {
	hexValue = strings.TrimPrefix(hexValue, "0x")
	if hexValue == "" {
		return decimal.Zero
	}

	// Convert hex to big.Int
	value := new(big.Int)
	value.SetString(hexValue, 16)

	// Convert to decimal string
	decimalStr := value.String()

	return parseTokenAmount(decimalStr, decimals)
}

// parseHexUint64 parses hex string to uint64
func parseHexUint64(hexValue string) uint64 {
	hexValue = strings.TrimPrefix(hexValue, "0x")
	if hexValue == "" {
		return 0
	}

	value := new(big.Int)
	value.SetString(hexValue, 16)
	return value.Uint64()
}
