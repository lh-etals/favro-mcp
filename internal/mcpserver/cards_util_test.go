package mcpserver

import "testing"

func TestContentHashIgnoresPresignedAttachmentSignature(t *testing.T) {
	// Shaped after a real Favro response: an inline image's presigned S3 URL,
	// reissued with a fresh date/signature on every fetch even though the
	// underlying file (and the rest of the description) never changed.
	a := `<br>  ![screenshot.png](https://favro.s3.eu-central-1.amazonaws.com/10e66bb8.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Content-Sha256=UNSIGNED-PAYLOAD&X-Amz-Credential=AKIA%2F20260813%2Feu-central-1%2Fs3%2Faws4_request&X-Amz-Date=20260813T052225Z&X-Amz-Expires=86400&X-Amz-Signature=d4761b75b54b7d16&X-Amz-SignedHeaders=host&x-amz-checksum-mode=ENABLED&x-id=GetObject)`
	b := `<br>  ![screenshot.png](https://favro.s3.eu-central-1.amazonaws.com/10e66bb8.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Content-Sha256=UNSIGNED-PAYLOAD&X-Amz-Credential=AKIA%2F20260813%2Feu-central-1%2Fs3%2Faws4_request&X-Amz-Date=20260813T060102Z&X-Amz-Expires=86400&X-Amz-Signature=3c8efe7e629633dc&X-Amz-SignedHeaders=host&x-amz-checksum-mode=ENABLED&x-id=GetObject)`

	if contentHash(a) != contentHash(b) {
		t.Errorf("contentHash should be stable across a re-signed presigned URL for the same file:\na=%s\nb=%s", contentHash(a), contentHash(b))
	}

	// A genuinely different object path must still be detected as a change.
	c := `<br>  ![screenshot.png](https://favro.s3.eu-central-1.amazonaws.com/DIFFERENT-FILE.png?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Date=20260813T060102Z&X-Amz-Signature=3c8efe7e629633dc)`
	if contentHash(a) == contentHash(c) {
		t.Error("contentHash should still change when the referenced file actually changes")
	}

	// Plain text with no attachment is unaffected.
	if contentHash("just some text") == contentHash("different text") {
		t.Error("contentHash should differ for genuinely different plain text")
	}
}
