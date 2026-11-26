// Human wording for audit actions, shared by the dashboard feed and the audit table.
const EVENTS: Record<string, [string, string]> = {
  'auth.login': ['Signed in', 'info'], 'auth.register': ['Created account', 'ok'],
  'secret.create': ['Created secret', 'ok'], 'secret.update': ['Updated secret', 'warn'], 'secret.delete': ['Deleted secret', 'bad'],
  'secret.read': ['Revealed secret', 'info'], 'secret.rotate': ['Rotated secret', 'warn'], 'secret.rollback': ['Restored secret version', 'warn'],
  'secret.import': ['Imported secrets', 'ok'], 'secret.export': ['Exported secrets', 'info'], 'secret.fetch': ['Fetched secrets (API)', 'info'], 'secret.expired': ['Secret expired', 'bad'],
  'token.create': ['Created service token', 'ok'], 'token.revoke': ['Revoked service token', 'bad'], 'token.denied': ['Blocked token (IP not allowed)', 'bad'],
  'member.add': ['Added member', 'ok'], 'member.remove': ['Removed member', 'bad'], 'member.role_change': ['Changed role', 'warn'], 'member.invite': ['Invited member', 'ok'],
  'member.invite_revoke': ['Revoked invitation', 'bad'], 'member.join': ['Joined via invitation', 'ok'], 'member.env_roles': ['Changed environment access', 'warn'],
  'project.create': ['Created project', 'ok'], 'project.delete': ['Deleted project', 'bad'], 'environment.create': ['Added environment', 'ok'], 'environment.delete': ['Removed environment', 'bad'],
  'webhook.create': ['Added webhook', 'ok'], 'webhook.update': ['Updated webhook', 'warn'], 'webhook.delete': ['Deleted webhook', 'bad'], 'webhook.test': ['Sent test webhook', 'info'],
  'integration.create': ['Added integration', 'ok'], 'integration.delete': ['Removed integration', 'bad'], 'integration.sync': ['Synced integration', 'info'],
  'dynamic.create': ['Created dynamic secret', 'ok'], 'dynamic.delete': ['Deleted dynamic secret', 'bad'], 'dynamic.lease': ['Generated credentials', 'info'], 'dynamic.revoke': ['Revoked credentials', 'bad'],
  'org.backup': ['Created backup', 'info'], 'org.restore': ['Restored backup', 'warn'],
}

export function describeEvent(action: string) {
  const known = EVENTS[action]
  if (known) return { label: known[0], tone: known[1] }
  const text = action.replace(/[._]/g, ' ')
  return { label: text.charAt(0).toUpperCase() + text.slice(1), tone: 'idle' }
}
