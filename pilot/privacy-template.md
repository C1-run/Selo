# Pilot data handling — written template (fill before any payment)

| Field | Agreed value |
|---|---|
| Where the agent runs | _client-controlled environment_ |
| Where receipts are stored | _client repository / local disk_ |
| What C1 receives | only what the client deliberately shares |
| Signing key | generated fresh inside the run sandbox; never a client production key |
| Retention and deletion | controlled by the client; no C1-side copy by default |
| Third parties | none; the pilot uses local git + local Selo CLI only |
| Sign-off (both sides, date) | _fill_ |

## Facts to disclose, not gloss over

- A Selo receipt is a **record**, not an upload. It contains the receipt id,
  verdict, integrity state, findings, a diff summary, file paths, the
  signer's public key and signature. The client should review the diff
  summary before sharing a receipt externally — it is evidence, not a redaction
  service.
- The pilot's signing key is generated in a throwaway sandbox and shares a
  trust domain with the runner. It proves receipt integrity, not independent
  identity attestation. For stronger separation, the main CLI supports a
  separate signer command / keychain (see ADR-008 and SECURITY.md); that is
  outside this pilot.
- **No CLI telemetry.** Selo does not phone home; there is no silent
  measurement of client runs. Diagnostics for C1's own kill criteria are
  limited to the public landing/payment route counters, never to client code.
- Hosted retention, if wanted later, is a separate agreement with Vault —
  it is explicitly not part of this pilot.
- Deletion: the client may delete the sandbox and receipts at any time; C1
  retains only the written scope and acceptance record needed for the
  engagement, unless agreed otherwise in writing.
