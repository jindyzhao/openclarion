package wecomcallback

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

const (
	testToken     = "callback-token-1"
	testReceiveID = "wwcorpfixture"
)

func TestVerifierVerifyEcho(t *testing.T) {
	key := testEncodingAESKey()
	encrypted := testEncryptMessage(t, key, []byte("echo-token-1"), testReceiveID)
	signature := callbackSignature(testToken, "1700000000", "nonce-1", encrypted)

	verifier, err := NewVerifier(Config{
		Token:          testToken,
		EncodingAESKey: key,
		ReceiveID:      testReceiveID,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	echo, err := verifier.VerifyEcho(signature, "1700000000", "nonce-1", encrypted)
	if err != nil {
		t.Fatalf("VerifyEcho: %v", err)
	}
	if echo != "echo-token-1" {
		t.Fatalf("echo = %q, want echo-token-1", echo)
	}
}

func TestVerifierDecryptMessageExtractsSenderAndContent(t *testing.T) {
	key := testEncodingAESKey()
	plaintext := []byte(`<xml>
<ToUserName><![CDATA[wwcorpfixture]]></ToUserName>
<FromUserName><![CDATA[operator-1]]></FromUserName>
<CreateTime>1700000000</CreateTime>
<MsgType><![CDATA[text]]></MsgType>
<Content><![CDATA[Need current database capacity status]]></Content>
<MsgId>10001</MsgId>
<AgentID>1000002</AgentID>
</xml>`)
	encrypted := testEncryptMessage(t, key, plaintext, testReceiveID)
	signature := callbackSignature(testToken, "1700000001", "nonce-2", encrypted)
	rawXML := []byte(fmt.Sprintf(`<xml><Encrypt><![CDATA[%s]]></Encrypt></xml>`, encrypted))

	verifier, err := NewVerifier(Config{
		Token:          testToken,
		EncodingAESKey: key,
		ReceiveID:      testReceiveID,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	message, err := verifier.DecryptMessage(signature, "1700000001", "nonce-2", rawXML)
	if err != nil {
		t.Fatalf("DecryptMessage: %v", err)
	}
	if message.FromUserName != "operator-1" ||
		message.ToUserName != "wwcorpfixture" ||
		message.MsgID != "10001" ||
		message.MsgType != "text" ||
		message.Content != "Need current database capacity status" ||
		message.AgentID != 1000002 {
		t.Fatalf("message = %+v", message)
	}
}

func TestVerifierRejectsInvalidSignatureWithoutLeakingPayload(t *testing.T) {
	key := testEncodingAESKey()
	plaintext := []byte(`<xml><FromUserName>operator-1</FromUserName><MsgType>text</MsgType><Content>secret-body</Content></xml>`)
	encrypted := testEncryptMessage(t, key, plaintext, testReceiveID)
	rawXML := []byte(fmt.Sprintf(`<xml><Encrypt>%s</Encrypt></xml>`, encrypted))

	verifier, err := NewVerifier(Config{
		Token:          testToken,
		EncodingAESKey: key,
		ReceiveID:      testReceiveID,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	_, err = verifier.DecryptMessage("bad-signature", "1700000001", "nonce-2", rawXML)
	if err == nil {
		t.Fatal("DecryptMessage error = nil, want invalid signature")
	}
	for _, leaked := range []string{encrypted, "secret-body", testToken, key} {
		if strings.Contains(err.Error(), leaked) {
			t.Fatalf("error leaked sensitive value %q: %v", leaked, err)
		}
	}
}

func TestVerifierRejectsReceiveIDMismatch(t *testing.T) {
	key := testEncodingAESKey()
	plaintext := []byte(`<xml><FromUserName>operator-1</FromUserName><MsgType>text</MsgType></xml>`)
	encrypted := testEncryptMessage(t, key, plaintext, "other-receiver")
	signature := callbackSignature(testToken, "1700000001", "nonce-2", encrypted)
	rawXML := []byte(fmt.Sprintf(`<xml><Encrypt>%s</Encrypt></xml>`, encrypted))

	verifier, err := NewVerifier(Config{
		Token:          testToken,
		EncodingAESKey: key,
		ReceiveID:      testReceiveID,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	_, err = verifier.DecryptMessage(signature, "1700000001", "nonce-2", rawXML)
	if err == nil {
		t.Fatal("DecryptMessage error = nil, want receive id mismatch")
	}
	if strings.Contains(err.Error(), "other-receiver") {
		t.Fatalf("error leaked receive id: %v", err)
	}
}

func TestVerifierRejectsInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{name: "missing token", cfg: Config{EncodingAESKey: testEncodingAESKey()}},
		{name: "spaced token", cfg: Config{Token: " callback-token ", EncodingAESKey: testEncodingAESKey()}},
		{name: "short aes key", cfg: Config{Token: testToken, EncodingAESKey: "short"}},
		{name: "spaced receive id", cfg: Config{Token: testToken, EncodingAESKey: testEncodingAESKey(), ReceiveID: " corp "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewVerifier(tc.cfg); err == nil {
				t.Fatal("NewVerifier error = nil, want invalid config")
			}
		})
	}
}

func TestVerifierRejectsMalformedMessage(t *testing.T) {
	key := testEncodingAESKey()
	plaintext := []byte(`<xml><FromUserName></FromUserName><MsgType>text</MsgType></xml>`)
	encrypted := testEncryptMessage(t, key, plaintext, testReceiveID)
	signature := callbackSignature(testToken, "1700000001", "nonce-2", encrypted)
	rawXML := []byte(fmt.Sprintf(`<xml><Encrypt>%s</Encrypt></xml>`, encrypted))

	verifier, err := NewVerifier(Config{
		Token:          testToken,
		EncodingAESKey: key,
		ReceiveID:      testReceiveID,
	})
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	_, err = verifier.DecryptMessage(signature, "1700000001", "nonce-2", rawXML)
	if err == nil {
		t.Fatal("DecryptMessage error = nil, want malformed message")
	}
}

func testEncodingAESKey() string {
	key := []byte("0123456789abcdef0123456789abcdef")
	return strings.TrimRight(base64.StdEncoding.EncodeToString(key), "=")
}

func testEncryptMessage(t *testing.T, rawKey string, plaintext []byte, receiveID string) string {
	t.Helper()
	key, err := base64.StdEncoding.DecodeString(rawKey + "=")
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}
	payload := bytes.Repeat([]byte{0x41}, 16)
	length := make([]byte, 4)
	// #nosec G115 -- test plaintext fixtures are far smaller than uint32.
	binary.BigEndian.PutUint32(length, uint32(len(plaintext)))
	payload = append(payload, length...)
	payload = append(payload, plaintext...)
	payload = append(payload, receiveID...)
	pad := weComPKCS7BlockSize - len(payload)%weComPKCS7BlockSize
	if pad == 0 {
		pad = weComPKCS7BlockSize
	}
	payload = append(payload, bytes.Repeat([]byte{byte(pad)}, pad)...)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	ciphertext := make([]byte, len(payload))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(ciphertext, payload)
	return base64.StdEncoding.EncodeToString(ciphertext)
}
