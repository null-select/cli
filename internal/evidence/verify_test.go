package evidence

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestVerifyWithKeysAcceptsOfflinePacket(t *testing.T) {
	path, publicKey := testPacket(t)
	result, err := VerifyWithKeys(path, map[string]ed25519.PublicKey{"evidence-test": publicKey})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Verified || result.PacketID != "pkt_0123456789abcdef01234567" || result.EventCount != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestOfficialKeyringContainsProductionKey(t *testing.T) {
	ids := OfficialKeyIDs()
	if len(ids) != 1 || ids[0] != "evidence-2026-10" {
		t.Fatalf("official key identifiers = %v", ids)
	}
}

func TestVerifyRejectsEveryMutatedMember(t *testing.T) {
	path, publicKey := testPacket(t)
	for _, name := range []string{"VERIFY.txt", "execution.json", "summary.html"} {
		t.Run(name, func(t *testing.T) {
			mutated := rewritePacket(t, path, func(member string, body []byte) []byte {
				if member == name {
					body[len(body)/2] ^= 1
				}
				return body
			})
			if _, err := VerifyWithKeys(mutated, map[string]ed25519.PublicKey{"evidence-test": publicKey}); err == nil {
				t.Fatalf("mutation of %s was accepted", name)
			}
		})
	}
}

func TestVerifyRejectsManifestAndSignatureCorruption(t *testing.T) {
	path, publicKey := testPacket(t)
	for _, mutate := range []func(string, []byte) []byte{
		func(name string, body []byte) []byte {
			if name == "manifest.json" {
				body[len(body)/2] ^= 1
			}
			return body
		},
		func(name string, body []byte) []byte {
			if name != "manifest.json" {
				return body
			}
			var manifest Manifest
			if err := json.Unmarshal(body, &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Signature = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, ed25519.SignatureSize))
			rewritten, err := json.Marshal(manifest)
			if err != nil {
				t.Fatal(err)
			}
			return append(rewritten, '\n')
		},
	} {
		mutated := rewritePacket(t, path, mutate)
		if _, err := VerifyWithKeys(mutated, map[string]ed25519.PublicKey{"evidence-test": publicKey}); err == nil {
			t.Fatal("manifest corruption was accepted")
		}
	}
}

func TestVerifyRejectsUntrustedKeyAndExtraMember(t *testing.T) {
	path, _ := testPacket(t)
	if _, err := VerifyWithKeys(path, map[string]ed25519.PublicKey{}); err == nil || !strings.Contains(err.Error(), "not trusted") {
		t.Fatalf("untrusted key error = %v", err)
	}
	extra := rewritePacket(t, path, func(name string, body []byte) []byte { return body }, "extra.txt")
	if _, err := VerifyWithKeys(extra, map[string]ed25519.PublicKey{"evidence-test": bytes.Repeat([]byte{7}, ed25519.PublicKeySize)}); err == nil {
		t.Fatal("extra ZIP member was accepted")
	}
}

func testPacket(t *testing.T) (string, ed25519.PublicKey) {
	t.Helper()
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	generatedAt := time.Date(2026, 10, 16, 9, 30, 0, 0, time.UTC)
	event := AuditEvent{
		OrganizationID: "org_example", ExecutionID: "exec_0123456789abcdef01234567", Sequence: 1,
		EventType: "request.received", OccurredAt: generatedAt,
		Payload: map[string]any{"environment_id": "env_production", "tool": "docs.search"},
	}
	event.EventHash, _ = hashAuditEvent(event)
	exported := AuditExport{
		SchemaVersion: AuditSchemaVersion, EnvironmentID: "env_production", GeneratedAt: generatedAt,
		OrganizationID: "org_example", ExecutionID: event.ExecutionID, EventCount: 1,
		AuditTip: event.EventHash, ChainValid: true, ActivationEvidenceStatus: "not_reached", Events: []AuditEvent{event},
	}
	executionJSON, err := json.MarshalIndent(exported, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	executionJSON = append(executionJSON, '\n')
	members := map[string][]byte{
		"VERIFY.txt": []byte("verify offline\n"), "execution.json": executionJSON,
		"summary.html": []byte("<!doctype html><title>Evidence</title>\n"),
	}
	manifest := Manifest{
		SchemaVersion: PacketSchemaVersion, PacketID: "pkt_0123456789abcdef01234567",
		OrganizationID: "org_example", EnvironmentID: "env_production", ExecutionID: event.ExecutionID,
		GeneratedAt: generatedAt, GeneratedBy: "member_owner", SigningKeyID: "evidence-test", Algorithm: SigningAlgorithm,
	}
	for _, item := range []struct{ name, media string }{
		{"VERIFY.txt", "text/plain; charset=utf-8"}, {"execution.json", "application/json"}, {"summary.html", "text/html; charset=utf-8"},
	} {
		digest := sha256.Sum256(members[item.name])
		manifest.Files = append(manifest.Files, ManifestFile{Name: item.name, MediaType: item.media, SHA256: "sha256:" + hex.EncodeToString(digest[:]), SizeBytes: len(members[item.name])})
	}
	payload, _ := canonicalManifest(manifest)
	manifest.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	manifestJSON, _ := json.Marshal(manifest)
	members["manifest.json"] = append(manifestJSON, '\n')
	path := filepath.Join(t.TempDir(), "packet.zip")
	writePacket(t, path, members, "")
	return path, publicKey
}

func rewritePacket(t *testing.T, source string, mutate func(string, []byte) []byte, extras ...string) string {
	t.Helper()
	archive, err := zip.OpenReader(source)
	if err != nil {
		t.Fatal(err)
	}
	members := map[string][]byte{}
	for _, file := range archive.File {
		opened, _ := file.Open()
		var buffer bytes.Buffer
		_, _ = buffer.ReadFrom(opened)
		opened.Close()
		members[file.Name] = mutate(file.Name, buffer.Bytes())
	}
	archive.Close()
	path := filepath.Join(t.TempDir(), "mutated.zip")
	extra := ""
	if len(extras) > 0 {
		extra = extras[0]
	}
	writePacket(t, path, members, extra)
	return path
}

func writePacket(t *testing.T, path string, members map[string][]byte, extra string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"manifest.json", "VERIFY.txt", "execution.json", "summary.html"} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if extra != "" {
		entry, _ := writer.Create(extra)
		_, _ = entry.Write([]byte("unexpected"))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
