package agents

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"

	"campfire/internal/shared/agentidentity"
)

func (h *Hub) renew(w http.ResponseWriter, r *http.Request) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.PeerCertificates) == 0 {
		http.Error(w, "verified Spark certificate required", http.StatusUnauthorized)
		return
	}

	leaf := r.TLS.PeerCertificates[0]

	id, hash := leaf.Subject.CommonName, agentidentity.Fingerprint(leaf.Raw)
	if !h.store.Authorized(r.Context(), id, hash) {
		http.Error(w, "unknown or disabled Spark", http.StatusUnauthorized)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 16384)

	var input struct {
		CSR []byte `json:"csr"`
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "invalid renewal request", http.StatusBadRequest)
		return
	}

	block, _ := pem.Decode(input.CSR)
	if block == nil {
		http.Error(w, "invalid CSR", http.StatusBadRequest)
		return
	}

	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil || !bytes.Equal(csr.RawSubjectPublicKeyInfo, leaf.RawSubjectPublicKeyInfo) {
		http.Error(w, "renewal CSR must prove possession of the enrolled key", http.StatusBadRequest)
		return
	}

	certificate, err := h.store.Renew(r.Context(), id, hash, leaf.NotAfter, func() ([]byte, string, error) {
		signed, err := h.authority.Sign(id, input.CSR)
		if err != nil {
			return nil, "", err
		}

		block, _ := pem.Decode(signed)

		return signed, agentidentity.Fingerprint(block.Bytes), nil
	})
	if err != nil {
		http.Error(w, "certificate renewal unavailable; check Spark authorization and signing authority expiry", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Certificate []byte `json:"certificate"`
		ServerCA    []byte `json:"server_ca,omitempty"`
	}{certificate, h.serverTrust()})
}
