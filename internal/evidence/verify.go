package evidence

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"time"
)

const (
	PacketSchemaVersion = "null.select.evidence-packet.v1"
	AuditSchemaVersion  = "null.select.audit-export.v3"
	KeySetSchemaVersion = "null.select.evidence-signing-keys.v1"
	SigningAlgorithm    = "Ed25519"
	maximumArchiveBytes = 4 << 20
	maximumMemberBytes  = 3 << 20
)

var (
	packetIDPattern    = regexp.MustCompile(`^pkt_[a-f0-9]{24}$`)
	executionPattern   = regexp.MustCompile(`^exec_[a-f0-9]{24}$`)
	keyIDPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`)
	sha256Pattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	officialPublicKeys = map[string]string{
		"evidence-2026-10": "avXbx2nLOBlAyXlZVoMgrHdG-4CZ-76HKoaeqMxw0hA",
	}
)

type ManifestFile struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	SizeBytes int    `json:"size_bytes"`
}

type Manifest struct {
	SchemaVersion  string         `json:"schema_version"`
	PacketID       string         `json:"packet_id"`
	OrganizationID string         `json:"organization_id"`
	EnvironmentID  string         `json:"environment_id"`
	ExecutionID    string         `json:"execution_id"`
	GeneratedAt    time.Time      `json:"generated_at"`
	GeneratedBy    string         `json:"generated_by"`
	SigningKeyID   string         `json:"signing_key_id"`
	Algorithm      string         `json:"signing_algorithm"`
	Files          []ManifestFile `json:"files"`
	Signature      string         `json:"signature"`
}

type PublicKey struct {
	KeyID     string `json:"key_id"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key"`
}

type PublicKeySet struct {
	SchemaVersion string      `json:"schema_version"`
	Keys          []PublicKey `json:"keys"`
}

type AuditEvent struct {
	OrganizationID string         `json:"organization_id"`
	ExecutionID    string         `json:"execution_id"`
	Sequence       int64          `json:"sequence"`
	EventType      string         `json:"event_type"`
	OccurredAt     time.Time      `json:"occurred_at"`
	Payload        map[string]any `json:"payload"`
	PreviousHash   string         `json:"previous_hash"`
	EventHash      string         `json:"event_hash"`
}

type ActivationEvidence struct {
	ActivationID        int64     `json:"activation_id"`
	EnvironmentID       string    `json:"environment_id"`
	ReleaseID           string    `json:"release_id"`
	ActorID             string    `json:"actor_id"`
	EnvironmentRevision int64     `json:"environment_revision"`
	OccurredAt          time.Time `json:"occurred_at"`
}

type ReleaseMemberEvidence struct {
	Ordinal              int    `json:"ordinal"`
	PublicToolName       string `json:"public_tool_name"`
	UpstreamToolName     string `json:"upstream_tool_name"`
	SourceID             string `json:"source_id"`
	SourceDisplayName    string `json:"source_display_name"`
	SourceRevisionID     string `json:"source_revision_id"`
	SourceRevisionDigest string `json:"source_revision_digest"`
	CandidateAttemptID   string `json:"candidate_attempt_id"`
	CandidateDigest      string `json:"candidate_digest"`
	CaseSetDigest        string `json:"case_set_digest"`
}

type ReleaseEvidence struct {
	ReleaseID            string                  `json:"release_id"`
	ReleaseDigest        string                  `json:"release_digest"`
	SupersedesID         string                  `json:"supersedes_id,omitempty"`
	SourceID             string                  `json:"source_id,omitempty"`
	SourceDisplayName    string                  `json:"source_display_name,omitempty"`
	SourceRevisionID     string                  `json:"source_revision_id,omitempty"`
	SourceRevisionDigest string                  `json:"source_revision_digest,omitempty"`
	CandidateAttemptID   string                  `json:"candidate_attempt_id,omitempty"`
	CaseSetDigest        string                  `json:"case_set_digest,omitempty"`
	Members              []ReleaseMemberEvidence `json:"members"`
}

type AuditExport struct {
	SchemaVersion            string              `json:"schema_version"`
	EnvironmentID            string              `json:"environment_id"`
	GeneratedAt              time.Time           `json:"generated_at"`
	OrganizationID           string              `json:"organization_id"`
	ExecutionID              string              `json:"execution_id"`
	EventCount               int                 `json:"event_count"`
	AuditTip                 string              `json:"audit_tip"`
	ChainValid               bool                `json:"chain_valid"`
	ActivationEvidenceStatus string              `json:"activation_evidence_status"`
	ActivationEvidence       *ActivationEvidence `json:"activation_evidence,omitempty"`
	ReleaseEvidence          *ReleaseEvidence    `json:"release_evidence,omitempty"`
	Events                   []AuditEvent        `json:"events"`
}

type Result struct {
	SchemaVersion string    `json:"schema_version"`
	PacketID      string    `json:"packet_id"`
	ExecutionID   string    `json:"execution_id"`
	EnvironmentID string    `json:"environment_id"`
	SigningKeyID  string    `json:"signing_key_id"`
	GeneratedAt   time.Time `json:"generated_at"`
	EventCount    int       `json:"event_count"`
	AuditTip      string    `json:"audit_tip"`
	Verified      bool      `json:"verified"`
}

func Verify(path, keySetPath string) (Result, error) {
	keys, err := trustedKeys(keySetPath)
	if err != nil {
		return Result{}, err
	}
	return VerifyWithKeys(path, keys)
}

func VerifyWithKeys(path string, keys map[string]ed25519.PublicKey) (Result, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Result{}, errors.New("open evidence packet")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maximumArchiveBytes {
		return Result{}, errors.New("evidence packet must be a regular ZIP no larger than 4 MiB")
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return Result{}, errors.New("read evidence packet ZIP")
	}
	defer archive.Close()
	wantMembers := []string{"VERIFY.txt", "execution.json", "manifest.json", "summary.html"}
	if len(archive.File) != len(wantMembers) {
		return Result{}, errors.New("evidence packet has unexpected members")
	}
	members := make(map[string][]byte, len(wantMembers))
	var total uint64
	for _, file := range archive.File {
		if !slices.Contains(wantMembers, file.Name) || file.FileInfo().IsDir() || file.UncompressedSize64 > maximumMemberBytes {
			return Result{}, fmt.Errorf("evidence packet member %q is not allowed", file.Name)
		}
		if _, exists := members[file.Name]; exists {
			return Result{}, fmt.Errorf("evidence packet member %q is duplicated", file.Name)
		}
		total += file.UncompressedSize64
		if total > maximumArchiveBytes {
			return Result{}, errors.New("evidence packet expands beyond 4 MiB")
		}
		opened, err := file.Open()
		if err != nil {
			return Result{}, errors.New("open evidence packet member")
		}
		body, readErr := io.ReadAll(io.LimitReader(opened, maximumMemberBytes+1))
		closeErr := opened.Close()
		if readErr != nil || closeErr != nil || len(body) > maximumMemberBytes {
			return Result{}, errors.New("read bounded evidence packet member")
		}
		members[file.Name] = body
	}
	for _, name := range wantMembers {
		if _, ok := members[name]; !ok {
			return Result{}, fmt.Errorf("evidence packet member %q is missing", name)
		}
	}
	var manifest Manifest
	if err := decodeStrict(members["manifest.json"], &manifest); err != nil {
		return Result{}, errors.New("evidence manifest is invalid")
	}
	canonicalFull, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(members["manifest.json"], append(canonicalFull, '\n')) {
		return Result{}, errors.New("evidence manifest is not canonical")
	}
	if err := validateManifest(manifest); err != nil {
		return Result{}, err
	}
	publicKey, ok := keys[manifest.SigningKeyID]
	if !ok {
		return Result{}, fmt.Errorf("evidence signing key %q is not trusted; update nullselect or provide --keyring", manifest.SigningKeyID)
	}
	signature, err := base64.RawURLEncoding.DecodeString(manifest.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return Result{}, errors.New("evidence manifest signature is invalid")
	}
	signingPayload, err := canonicalManifest(manifest)
	if err != nil || !ed25519.Verify(publicKey, signingPayload, signature) {
		return Result{}, errors.New("evidence manifest signature verification failed")
	}
	for _, file := range manifest.Files {
		body, ok := members[file.Name]
		if !ok || len(body) != file.SizeBytes {
			return Result{}, fmt.Errorf("evidence member %q size does not match manifest", file.Name)
		}
		digest := sha256.Sum256(body)
		if file.SHA256 != "sha256:"+hex.EncodeToString(digest[:]) {
			return Result{}, fmt.Errorf("evidence member %q digest verification failed", file.Name)
		}
	}
	var exported AuditExport
	if err := decodeStrict(members["execution.json"], &exported); err != nil {
		return Result{}, errors.New("audit export is invalid")
	}
	if err := verifyAudit(manifest, exported); err != nil {
		return Result{}, err
	}
	return Result{
		SchemaVersion: PacketSchemaVersion, PacketID: manifest.PacketID,
		ExecutionID: manifest.ExecutionID, EnvironmentID: manifest.EnvironmentID,
		SigningKeyID: manifest.SigningKeyID, GeneratedAt: manifest.GeneratedAt,
		EventCount: exported.EventCount, AuditTip: exported.AuditTip, Verified: true,
	}, nil
}

func trustedKeys(keySetPath string) (map[string]ed25519.PublicKey, error) {
	encoded := make(map[string]string, len(officialPublicKeys))
	for keyID, value := range officialPublicKeys {
		encoded[keyID] = value
	}
	if keySetPath != "" {
		raw, err := os.ReadFile(keySetPath)
		if err != nil {
			return nil, errors.New("read evidence keyring")
		}
		var keySet PublicKeySet
		if err := decodeStrict(raw, &keySet); err != nil || keySet.SchemaVersion != KeySetSchemaVersion || len(keySet.Keys) == 0 || len(keySet.Keys) > 32 {
			return nil, errors.New("evidence keyring is invalid")
		}
		for _, key := range keySet.Keys {
			if !keyIDPattern.MatchString(key.KeyID) || key.Algorithm != SigningAlgorithm {
				return nil, errors.New("evidence keyring contains an invalid key")
			}
			if previous, exists := encoded[key.KeyID]; exists && previous != key.PublicKey {
				return nil, fmt.Errorf("evidence key %q conflicts with a trusted key", key.KeyID)
			}
			encoded[key.KeyID] = key.PublicKey
		}
	}
	keys := make(map[string]ed25519.PublicKey, len(encoded))
	for keyID, value := range encoded {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("evidence key %q is malformed", keyID)
		}
		keys[keyID] = ed25519.PublicKey(raw)
	}
	return keys, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.SchemaVersion != PacketSchemaVersion || !packetIDPattern.MatchString(manifest.PacketID) ||
		manifest.OrganizationID == "" || manifest.EnvironmentID == "" || !executionPattern.MatchString(manifest.ExecutionID) ||
		manifest.GeneratedAt.IsZero() || manifest.GeneratedBy == "" || !keyIDPattern.MatchString(manifest.SigningKeyID) ||
		manifest.Algorithm != SigningAlgorithm || manifest.Signature == "" {
		return errors.New("evidence manifest fields are invalid")
	}
	want := []ManifestFile{
		{Name: "VERIFY.txt", MediaType: "text/plain; charset=utf-8"},
		{Name: "execution.json", MediaType: "application/json"},
		{Name: "summary.html", MediaType: "text/html; charset=utf-8"},
	}
	if len(manifest.Files) != len(want) {
		return errors.New("evidence manifest file list is invalid")
	}
	for index, file := range manifest.Files {
		if file.Name != want[index].Name || file.MediaType != want[index].MediaType ||
			!sha256Pattern.MatchString(file.SHA256) || file.SizeBytes <= 0 || file.SizeBytes > maximumMemberBytes {
			return errors.New("evidence manifest file entry is invalid")
		}
	}
	return nil
}

func canonicalManifest(manifest Manifest) ([]byte, error) {
	manifest.Signature = ""
	type unsignedManifest Manifest
	return json.Marshal(struct {
		unsignedManifest
		Signature *string `json:"signature,omitempty"`
	}{unsignedManifest: unsignedManifest(manifest)})
}

func verifyAudit(manifest Manifest, exported AuditExport) error {
	if exported.SchemaVersion != AuditSchemaVersion || !exported.ChainValid || exported.EventCount != len(exported.Events) ||
		exported.EventCount < 1 || exported.EventCount > 1000 || exported.OrganizationID != manifest.OrganizationID ||
		exported.EnvironmentID != manifest.EnvironmentID || exported.ExecutionID != manifest.ExecutionID ||
		!exported.GeneratedAt.Equal(manifest.GeneratedAt) {
		return errors.New("audit export does not match the signed manifest")
	}
	previous := ""
	for index, event := range exported.Events {
		if event.OrganizationID != exported.OrganizationID || event.ExecutionID != exported.ExecutionID ||
			event.Sequence != int64(index+1) || event.PreviousHash != previous || !sha256Pattern.MatchString("sha256:"+event.EventHash) {
			return fmt.Errorf("audit chain is invalid at sequence %d", event.Sequence)
		}
		expected, err := hashAuditEvent(event)
		if err != nil || expected != event.EventHash {
			return fmt.Errorf("audit chain digest is invalid at sequence %d", event.Sequence)
		}
		previous = event.EventHash
	}
	if exported.AuditTip != previous {
		return errors.New("audit tip does not match the final event")
	}
	if err := verifyReleaseEvidence(exported); err != nil {
		return err
	}
	if err := verifyActivationEvidence(exported); err != nil {
		return err
	}
	return nil
}

func hashAuditEvent(event AuditEvent) (string, error) {
	payload := struct {
		OrganizationID string         `json:"organization_id"`
		ExecutionID    string         `json:"execution_id"`
		Sequence       int64          `json:"sequence"`
		EventType      string         `json:"event_type"`
		OccurredAt     string         `json:"occurred_at"`
		Payload        map[string]any `json:"payload"`
		PreviousHash   string         `json:"previous_hash"`
	}{event.OrganizationID, event.ExecutionID, event.Sequence, event.EventType,
		event.OccurredAt.UTC().Format("2006-01-02T15:04:05.000000Z07:00"), event.Payload, event.PreviousHash}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return "", err
	}
	canonical, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func verifyReleaseEvidence(exported AuditExport) error {
	var releaseID, releaseDigest, toolName string
	for _, event := range exported.Events {
		if event.EventType == "request.received" && toolName == "" {
			toolName, _ = event.Payload["tool"].(string)
		}
		if event.EventType == "release.resolved" && releaseID == "" {
			releaseID, _ = event.Payload["release_id"].(string)
			releaseDigest, _ = event.Payload["release_digest"].(string)
		}
	}
	if releaseID == "" && releaseDigest == "" {
		if exported.ReleaseEvidence != nil {
			return errors.New("release evidence has no matching audit event")
		}
		return nil
	}
	if exported.ReleaseEvidence == nil || exported.ReleaseEvidence.ReleaseID != releaseID || exported.ReleaseEvidence.ReleaseDigest != releaseDigest {
		return errors.New("release evidence does not match the audit event")
	}
	if len(exported.ReleaseEvidence.Members) > 0 {
		found := false
		for index, member := range exported.ReleaseEvidence.Members {
			if member.Ordinal != index+1 || member.PublicToolName == "" {
				return errors.New("release member evidence is invalid")
			}
			if member.PublicToolName == toolName {
				found = true
			}
		}
		if !found {
			return errors.New("release evidence does not contain the executed tool")
		}
	}
	return nil
}

func verifyActivationEvidence(exported AuditExport) error {
	if !slices.Contains([]string{"verified", "unavailable_legacy", "not_reached"}, exported.ActivationEvidenceStatus) {
		return errors.New("activation evidence status is invalid")
	}
	if exported.ActivationEvidenceStatus != "verified" {
		if exported.ActivationEvidence != nil {
			return errors.New("activation evidence conflicts with its status")
		}
		return nil
	}
	if exported.ActivationEvidence == nil {
		return errors.New("verified activation evidence is missing")
	}
	var decision *AuditEvent
	var releaseID string
	for index := range exported.Events {
		event := &exported.Events[index]
		if event.EventType == "release.resolved" && releaseID == "" {
			releaseID, _ = event.Payload["release_id"].(string)
		}
		if decision == nil && (event.EventType == "policy.allowed" || event.EventType == "policy.denied") {
			decision = event
		}
	}
	if decision == nil {
		return errors.New("verified activation has no policy decision")
	}
	activationID, idOK := exactInt64(decision.Payload["activation_id"])
	revision, revisionOK := exactInt64(decision.Payload["activation_environment_revision"])
	actor, actorOK := decision.Payload["activation_actor_id"].(string)
	occurredText, timeOK := decision.Payload["activation_occurred_at"].(string)
	occurredAt, timeErr := time.Parse(time.RFC3339Nano, occurredText)
	evidence := exported.ActivationEvidence
	if !idOK || !revisionOK || !actorOK || !timeOK || timeErr != nil || activationID != evidence.ActivationID ||
		revision != evidence.EnvironmentRevision || actor != evidence.ActorID || !occurredAt.Equal(evidence.OccurredAt) ||
		evidence.EnvironmentID != exported.EnvironmentID || evidence.ReleaseID != releaseID {
		return errors.New("activation evidence does not match the policy decision")
	}
	return nil
}

func exactInt64(value any) (int64, bool) {
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Int64()
		return parsed, err == nil
	case float64:
		if number != float64(int64(number)) {
			return 0, false
		}
		return int64(number), true
	default:
		return 0, false
	}
}

func decodeStrict(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON content")
	}
	return nil
}

func OfficialKeyIDs() []string {
	result := make([]string, 0, len(officialPublicKeys))
	for keyID := range officialPublicKeys {
		result = append(result, keyID)
	}
	slices.Sort(result)
	return result
}
