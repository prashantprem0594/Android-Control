package release

import (
	"crypto/ed25519"
	"crypto/x509"
	"time"
)

// Document is an immutable, operator-provisioned public metadata snapshot.
// The hosting process revalidates its lifetime for every response.
type Document struct {
	raw                 []byte
	key                 ed25519.PublicKey
	origin, certificate string
	minimum             int
}

func NewDocument(raw, publicDER []byte, origin, certificate string, minimum int) (*Document, error) {
	if len(publicDER) != 44 {
		return nil, ErrInvalid
	}
	parsed, err := x509.ParsePKIXPublicKey(publicDER)
	if err != nil {
		return nil, ErrInvalid
	}
	key, ok := parsed.(ed25519.PublicKey)
	if !ok {
		return nil, ErrInvalid
	}
	manifest, err := verifiedPayload(raw, key)
	// Verify authority/structure at load time, freshness at response time. An
	// expired but authentic update must not prevent control-plane startup.
	if err != nil || manifest.Validate(origin, certificate, minimum, manifest.IssuedAt) != nil {
		return nil, ErrInvalid
	}
	return &Document{raw: append([]byte(nil), raw...), key: append(ed25519.PublicKey(nil), key...), origin: origin, certificate: certificate, minimum: minimum}, nil
}
func (document *Document) Bytes(now time.Time) ([]byte, error) {
	if document == nil {
		return nil, ErrInvalid
	}
	if _, err := Verify(document.raw, document.key, document.origin, document.certificate, document.minimum, now); err != nil {
		return nil, ErrInvalid
	}
	return append([]byte(nil), document.raw...), nil
}
