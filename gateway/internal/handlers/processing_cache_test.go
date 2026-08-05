package handlers

import "testing"

// TestProcessingFingerprintChangesWithVariant verifies processing-relevant parameters invalidate cache identity.
func TestProcessingFingerprintChangesWithVariant(t *testing.T) {
	contentHash := hashBytes([]byte("same document"))
	bullet := processingFingerprint(operationDistiller, contentHash, "bullet_points", ".pdf")
	executive := processingFingerprint(operationDistiller, contentHash, "executive", ".pdf")

	if bullet == executive {
		t.Fatal("distiller fingerprint must change when summary style changes")
	}
}

// TestProcessingFingerprintChangesWithSourceFormat verifies parser-relevant formats remain isolated.
func TestProcessingFingerprintChangesWithSourceFormat(t *testing.T) {
	contentHash := hashBytes([]byte("same bytes"))
	pdf := processingFingerprint(operationGleaner, contentHash, ".pdf")
	text := processingFingerprint(operationGleaner, contentHash, ".txt")

	if pdf == text {
		t.Fatal("processing fingerprint must change when source format changes")
	}
}

// TestProcessingFingerprintChangesWithContent verifies different source bytes never share cache identity.
func TestProcessingFingerprintChangesWithContent(t *testing.T) {
	first := processingFingerprint(operationGleaner, hashBytes([]byte("document one")), ".txt")
	second := processingFingerprint(operationGleaner, hashBytes([]byte("document two")), ".txt")

	if first == second {
		t.Fatal("processing fingerprint must change when source content changes")
	}
}

// TestProcessingFingerprintChangesWithOperation verifies separate pipelines never share cache entries.
func TestProcessingFingerprintChangesWithOperation(t *testing.T) {
	contentHash := hashBytes([]byte("same bytes"))
	scriber := processingFingerprint(operationScriber, contentHash, ".wav")
	gleaner := processingFingerprint(operationGleaner, contentHash, ".wav")

	if scriber == gleaner {
		t.Fatal("processing fingerprint must change between operations")
	}
}