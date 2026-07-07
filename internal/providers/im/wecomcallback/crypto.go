// Package wecomcallback verifies and decrypts Enterprise WeChat application
// message callbacks.
package wecomcallback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	// #nosec G505 -- Enterprise WeChat callback signatures require SHA-1 by protocol.
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"
)

const (
	encodingAESKeyBytes = 32
	encodingAESKeyChars = 43
	maxEncryptedBytes   = 64 << 10
	maxPlaintextBytes   = 64 << 10
	weComPKCS7BlockSize = 32
)

// Config holds Enterprise WeChat callback verification material.
type Config struct {
	Token          string
	EncodingAESKey string
	ReceiveID      string
}

// Verifier verifies callback signatures and decrypts Enterprise WeChat XML.
type Verifier struct {
	token     string
	aesKey    []byte
	receiveID string
}

// Message is the sanitized subset of an Enterprise WeChat decrypted callback.
type Message struct {
	ToUserName   string
	FromUserName string
	CreateTime   int64
	MsgID        string
	MsgType      string
	Content      string
	Event        string
	EventKey     string
	AgentID      int64
}

type encryptedXML struct {
	XMLName xml.Name `xml:"xml"`
	Encrypt string   `xml:"Encrypt"`
}

type decryptedXML struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgID        string   `xml:"MsgId"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	Event        string   `xml:"Event"`
	EventKey     string   `xml:"EventKey"`
	AgentID      int64    `xml:"AgentID"`
}

// NewVerifier validates callback configuration.
func NewVerifier(cfg Config) (*Verifier, error) {
	token, err := normalizeToken(cfg.Token)
	if err != nil {
		return nil, err
	}
	aesKey, err := decodeEncodingAESKey(cfg.EncodingAESKey)
	if err != nil {
		return nil, err
	}
	receiveID, err := normalizeReceiveID(cfg.ReceiveID)
	if err != nil {
		return nil, err
	}
	return &Verifier{
		token:     token,
		aesKey:    aesKey,
		receiveID: receiveID,
	}, nil
}

// VerifyEcho verifies and decrypts the GET callback setup echo string.
func (v *Verifier) VerifyEcho(msgSignature, timestamp, nonce, echo string) (string, error) {
	if v == nil {
		return "", fmt.Errorf("wecom callback: verifier is nil")
	}
	encrypted, err := normalizeEncryptedValue(echo)
	if err != nil {
		return "", err
	}
	if err := v.verifySignature(msgSignature, timestamp, nonce, encrypted); err != nil {
		return "", err
	}
	plaintext, err := v.decrypt(encrypted)
	if err != nil {
		return "", err
	}
	if !validCallbackText(plaintext) {
		return "", fmt.Errorf("wecom callback: decrypted echo is invalid")
	}
	return string(plaintext), nil
}

// DecryptMessage verifies and decrypts one POST callback XML body.
func (v *Verifier) DecryptMessage(msgSignature, timestamp, nonce string, rawXML []byte) (Message, error) {
	if v == nil {
		return Message{}, fmt.Errorf("wecom callback: verifier is nil")
	}
	wrapper, err := parseEncryptedXML(rawXML)
	if err != nil {
		return Message{}, err
	}
	encrypted, err := normalizeEncryptedValue(wrapper.Encrypt)
	if err != nil {
		return Message{}, err
	}
	if err := v.verifySignature(msgSignature, timestamp, nonce, encrypted); err != nil {
		return Message{}, err
	}
	plaintext, err := v.decrypt(encrypted)
	if err != nil {
		return Message{}, err
	}
	return parseDecryptedMessage(plaintext)
}

func (v *Verifier) verifySignature(msgSignature, timestamp, nonce, encrypted string) error {
	signature, err := normalizeCallbackToken("msg_signature", msgSignature)
	if err != nil {
		return err
	}
	ts, err := normalizeCallbackToken("timestamp", timestamp)
	if err != nil {
		return err
	}
	normalizedNonce, err := normalizeCallbackToken("nonce", nonce)
	if err != nil {
		return err
	}
	want := callbackSignature(v.token, ts, normalizedNonce, encrypted)
	if subtle.ConstantTimeCompare([]byte(signature), []byte(want)) != 1 {
		return fmt.Errorf("wecom callback: signature is invalid")
	}
	return nil
}

func callbackSignature(token, timestamp, nonce, encrypted string) string {
	parts := []string{token, timestamp, nonce, encrypted}
	sort.Strings(parts)
	// #nosec G401 -- Enterprise WeChat callback signatures require SHA-1 by protocol.
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(sum[:])
}

func (v *Verifier) decrypt(encrypted string) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("wecom callback: decrypt message failed")
	}
	if len(ciphertext) == 0 ||
		len(ciphertext) > maxEncryptedBytes ||
		len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("wecom callback: encrypted message is invalid")
	}
	block, err := aes.NewCipher(v.aesKey)
	if err != nil {
		return nil, fmt.Errorf("wecom callback: initialize cipher: %w", err)
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, v.aesKey[:aes.BlockSize]).CryptBlocks(plaintext, ciphertext)
	unpadded, err := removeWeComPadding(plaintext)
	if err != nil {
		return nil, err
	}
	if len(unpadded) < 20 {
		return nil, fmt.Errorf("wecom callback: decrypted message is invalid")
	}
	messageLength := int(binary.BigEndian.Uint32(unpadded[16:20]))
	if messageLength < 0 || messageLength > maxPlaintextBytes || 20+messageLength > len(unpadded) {
		return nil, fmt.Errorf("wecom callback: decrypted message length is invalid")
	}
	message := unpadded[20 : 20+messageLength]
	receiveID := string(unpadded[20+messageLength:])
	if v.receiveID != "" && subtle.ConstantTimeCompare([]byte(receiveID), []byte(v.receiveID)) != 1 {
		return nil, fmt.Errorf("wecom callback: receive id is invalid")
	}
	return message, nil
}

func parseEncryptedXML(rawXML []byte) (encryptedXML, error) {
	if len(rawXML) == 0 || len(rawXML) > maxEncryptedBytes {
		return encryptedXML{}, fmt.Errorf("wecom callback: encrypted xml size is invalid")
	}
	decoder := xml.NewDecoder(bytes.NewReader(rawXML))
	decoder.Strict = true
	var wrapper encryptedXML
	if err := decoder.Decode(&wrapper); err != nil {
		return encryptedXML{}, fmt.Errorf("wecom callback: encrypted xml is invalid")
	}
	if wrapper.XMLName.Local != "xml" {
		return encryptedXML{}, fmt.Errorf("wecom callback: encrypted xml root is invalid")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return encryptedXML{}, fmt.Errorf("wecom callback: encrypted xml trailing data is invalid")
	}
	return wrapper, nil
}

func parseDecryptedMessage(plaintext []byte) (Message, error) {
	if len(plaintext) == 0 || len(plaintext) > maxPlaintextBytes {
		return Message{}, fmt.Errorf("wecom callback: decrypted xml size is invalid")
	}
	decoder := xml.NewDecoder(bytes.NewReader(plaintext))
	decoder.Strict = true
	var decoded decryptedXML
	if err := decoder.Decode(&decoded); err != nil {
		return Message{}, fmt.Errorf("wecom callback: decrypted xml is invalid")
	}
	if decoded.XMLName.Local != "xml" {
		return Message{}, fmt.Errorf("wecom callback: decrypted xml root is invalid")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Message{}, fmt.Errorf("wecom callback: decrypted xml trailing data is invalid")
	}
	message := Message{
		ToUserName:   strings.TrimSpace(decoded.ToUserName),
		FromUserName: strings.TrimSpace(decoded.FromUserName),
		CreateTime:   decoded.CreateTime,
		MsgID:        strings.TrimSpace(decoded.MsgID),
		MsgType:      strings.TrimSpace(decoded.MsgType),
		Content:      decoded.Content,
		Event:        strings.TrimSpace(decoded.Event),
		EventKey:     strings.TrimSpace(decoded.EventKey),
		AgentID:      decoded.AgentID,
	}
	if message.FromUserName == "" {
		return Message{}, fmt.Errorf("wecom callback: message sender is missing")
	}
	if message.MsgType == "" {
		return Message{}, fmt.Errorf("wecom callback: message type is missing")
	}
	return message, nil
}

func decodeEncodingAESKey(raw string) ([]byte, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, fmt.Errorf("wecom callback: encoding aes key must be non-empty")
	}
	if value != raw || len(value) != encodingAESKeyChars {
		return nil, fmt.Errorf("wecom callback: encoding aes key must be 43 characters")
	}
	key, err := base64.StdEncoding.DecodeString(value + "=")
	if err != nil || len(key) != encodingAESKeyBytes {
		return nil, fmt.Errorf("wecom callback: encoding aes key is invalid")
	}
	return key, nil
}

func normalizeToken(raw string) (string, error) {
	value, err := normalizeCallbackToken("token", raw)
	if err != nil {
		return "", err
	}
	return value, nil
}

func normalizeReceiveID(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value != raw {
		return "", fmt.Errorf("wecom callback: receive id must not contain leading or trailing whitespace")
	}
	if strings.ContainsFunc(value, unicode.IsControl) {
		return "", fmt.Errorf("wecom callback: receive id must not contain control characters")
	}
	return value, nil
}

func normalizeCallbackToken(label, raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("wecom callback: %s must be non-empty", label)
	}
	if value != raw || len([]byte(value)) > 512 {
		return "", fmt.Errorf("wecom callback: %s is invalid", label)
	}
	if strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) {
		return "", fmt.Errorf("wecom callback: %s is invalid", label)
	}
	return value, nil
}

func normalizeEncryptedValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("wecom callback: encrypted value must be non-empty")
	}
	if value != raw || len([]byte(value)) > maxEncryptedBytes {
		return "", fmt.Errorf("wecom callback: encrypted value is too large")
	}
	if strings.ContainsFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) {
		return "", fmt.Errorf("wecom callback: encrypted value is invalid")
	}
	return value, nil
}

func removeWeComPadding(plaintext []byte) ([]byte, error) {
	if len(plaintext) == 0 {
		return nil, fmt.Errorf("wecom callback: decrypted padding is invalid")
	}
	pad := int(plaintext[len(plaintext)-1])
	if pad < 1 || pad > weComPKCS7BlockSize || pad > len(plaintext) {
		return nil, fmt.Errorf("wecom callback: decrypted padding is invalid")
	}
	for _, value := range plaintext[len(plaintext)-pad:] {
		if int(value) != pad {
			return nil, fmt.Errorf("wecom callback: decrypted padding is invalid")
		}
	}
	return plaintext[:len(plaintext)-pad], nil
}

func validCallbackText(value []byte) bool {
	if len(value) == 0 || len(value) > maxPlaintextBytes {
		return false
	}
	return !bytes.ContainsFunc(value, unicode.IsControl)
}
