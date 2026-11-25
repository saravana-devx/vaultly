package webhook

const (
	MsgListed     = "Webhooks fetched"
	MsgCreated    = "Webhook created. Copy the signing secret now, it will not be shown again"
	MsgUpdated    = "Webhook updated"
	MsgDeleted    = "Webhook deleted"
	MsgTested     = "Test event sent"
	MsgDeliveries = "Deliveries fetched"

	MsgNotFound      = "Webhook not found"
	MsgInvalidURL    = "URL must start with http:// or https://"
	MsgPrivateURL    = "Private and loopback addresses are not allowed"
	MsgInvalidEvents = "Events look like secret.*, token.create or *"
)
