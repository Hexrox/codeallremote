# Authentication and device pairing

## Threat model

CAR is accessed over the Internet through a VPS gateway. A stolen phone, intercepted pairing link or leaked token must not grant durable agent-control access. The gateway is transport infrastructure; CAR remains the final authorization authority.

## MVP identity model

CAR has one owner account and explicitly paired devices. Each device receives a revocable device record and short-lived access tokens; refresh credentials are encrypted at rest on the device using platform-protected storage.

## Pairing flow

1. The owner opens the local CAR administration interface on the homelab.
2. CAR creates a single-use, short-lived pairing challenge.
3. Android scans a QR code or enters the challenge, then proves possession of a newly generated device key.
4. The owner confirms the device name and fingerprint on the trusted local interface.
5. CAR stores the public key/device record and issues the first token set.

The QR code contains no long-lived secret and expires within minutes. A pairing challenge can be used once only.

## Token rules

- Access tokens are audience-bound to CAR and expire quickly.
- Refresh credentials are bound to one device and may be revoked independently.
- Sensitive actions—approval decisions and pairing—require recent device unlock; Android uses biometric or device credential where available.
- A server restart invalidates only ephemeral sessions, not paired-device records.

## Revocation

The owner can revoke a device from the local administration interface or any already trusted client. Revocation invalidates refresh credentials immediately and adds its key/token identifier to the server deny list until token expiry.

## Notification privacy

Push notifications contain an approval/session identifier and minimal generic text such as “Action requires approval.” Full command text, filenames, repository names and secrets are fetched only after authenticated app unlock.

