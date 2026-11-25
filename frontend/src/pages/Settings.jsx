import { useEffect, useRef, useState } from 'react'
import { api, apiBlob, apiForm } from '../api.js'
import { useAuth } from '../auth.jsx'
import { Modal, Icon, ago, copy, useToast } from '../components/ui.jsx'

export default function Settings() {
  const { org, orgId, user } = useAuth()
  const [toast, toastEl] = useToast()
  return (
    <>
      <div className="head"><div><h1>Settings</h1><p>{user.email}</p></div></div>
      <PersonalTokens toast={toast} />
      {org?.role === 'admin' && <Backup orgId={orgId} orgName={org.name} toast={toast} />}
      {toastEl}
    </>
  )
}

function PersonalTokens({ toast }) {
  const [list, setList] = useState(null)
  const [open, setOpen] = useState(false)
  const [created, setCreated] = useState(null)
  const [f, setF] = useState({ name: '', expire_days: 90 })
  const [err, setErr] = useState('')
  const load = () => api('/me/tokens').then(setList)
  useEffect(() => { load() }, [])
  return (
    <section className="block">
      <div className="sect"><div><h2>Personal access tokens</h2><small>Call the API as yourself, with your roles and environment access. Use them for scripts and local tooling.</small></div>
        <button className="btn" onClick={() => { setErr(''); setOpen(true) }}><Icon name="plus" size={15} /> New token</button></div>
      <table className="table"><thead><tr><th>Name</th><th>Token</th><th>Expires</th><th>Last used</th><th /></tr></thead><tbody>
        {(list || []).map((t) => (
          <tr key={t.id}><td style={{ fontWeight: 500 }}>{t.name}</td><td className="mono muted">{t.prefix}…</td>
            <td className="muted">{t.expires_at ? new Date(t.expires_at).toLocaleDateString() : 'Never'}</td><td className="muted">{ago(t.last_used_at)}</td>
            <td style={{ textAlign: 'right' }}><button className="btn sm ghost danger" onClick={async () => { if (confirm(`Revoke ${t.name}?`)) { await api(`/me/tokens/${t.id}`, { method: 'DELETE' }); load() } }}>Revoke</button></td></tr>))}
        {list && !list.length && <tr><td colSpan="5" className="muted" style={{ textAlign: 'center', padding: 32 }}>No personal tokens.</td></tr>}
      </tbody></table>
      {open && (
        <Modal title="New personal access token" onClose={() => setOpen(false)}>
          <form onSubmit={async (e) => { e.preventDefault(); try { const d = await api('/me/tokens', { method: 'POST', body: { name: f.name, expire_days: +f.expire_days } }); setOpen(false); setCreated(d.token); load() } catch (x) { setErr(x.message) } }}>
            {err && <div className="alert">{err}</div>}
            <div className="field"><label>Name</label><input className="input" required autoFocus value={f.name} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder="laptop" /></div>
            <div className="field"><label>Expires in<span className="hint">days, 0 = never</span></label><input className="input" type="number" min="0" value={f.expire_days} onChange={(e) => setF({ ...f, expire_days: e.target.value })} /></div>
            <div className="modal-actions"><button type="button" className="btn" onClick={() => setOpen(false)}>Cancel</button><button className="btn primary">Create token</button></div>
          </form>
        </Modal>
      )}
      {created && (
        <Modal title="Token created" onClose={() => setCreated(null)}>
          <div className="alert warn">Copy this token now — it won’t be shown again.</div>
          <div className="tokenbox">{created}</div>
          <p className="muted mono" style={{ fontSize: 12.5 }}>curl -H "Authorization: Bearer {created.slice(0, 10)}…" /api/orgs</p>
          <div className="modal-actions"><button className="btn" onClick={async () => toast((await copy(created)) ? 'Copied' : 'Copy failed')}>Copy</button><button className="btn primary" onClick={() => setCreated(null)}>Done</button></div>
        </Modal>
      )}
    </section>
  )
}

function Backup({ orgId, orgName, toast }) {
  const [pass, setPass] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [rpass, setRpass] = useState('')
  const [result, setResult] = useState(null)
  const file = useRef()

  const download = async (e) => {
    e.preventDefault(); setErr(''); setBusy(true)
    try {
      const blob = await apiBlob(`/orgs/${orgId}/backup`, { passphrase: pass })
      const a = document.createElement('a'); a.href = URL.createObjectURL(blob); a.download = `vaultly-backup-${new Date().toISOString().slice(0, 10)}.vbk`; a.click()
      setPass(''); toast('Encrypted backup downloaded')
    } catch (x) { setErr(x.message) }
    setBusy(false)
  }
  const restore = async (e) => {
    e.preventDefault(); setErr(''); setResult(null)
    const fd = new FormData(); fd.append('file', file.current.files[0]); fd.append('passphrase', rpass)
    setBusy(true)
    try { setResult(await apiForm(`/orgs/${orgId}/restore`, fd)); setRpass(''); file.current.value = ''; window.dispatchEvent(new Event('vaultly:projects')) } catch (x) { setErr(x.message) }
    setBusy(false)
  }
  return (
    <section className="block">
      <div className="sect"><div><h2>Backup &amp; restore</h2><small>{orgName}</small></div></div>
      {err && <div className="alert">{err}</div>}
      <div className="cols2">
        <form onSubmit={download}>
          <h3 className="h3">Create a backup</h3>
          <p className="muted" style={{ fontSize: 13.5, margin: '4px 0 14px' }}>Every project, environment, secret and version as ciphertext, sealed again with your passphrase. Tokens, webhooks and integration credentials are not included.</p>
          <div className="field"><label>Passphrase<span className="hint">12+ characters — you’ll need it to restore</span></label><input className="input" type="password" minLength={12} required autoComplete="new-password" value={pass} onChange={(e) => setPass(e.target.value)} /></div>
          <button className="btn" disabled={busy}><Icon name="download" size={15} /> Download encrypted backup</button>
        </form>
        <form onSubmit={restore}>
          <h3 className="h3">Restore from a backup</h3>
          <p className="muted" style={{ fontSize: 13.5, margin: '4px 0 14px' }}>Projects keep their original IDs and are never overwritten. The server must use the same MASTER_KEY the backup was made with.</p>
          <div className="field"><label>Backup file</label><input className="input" type="file" accept=".vbk" required ref={file} style={{ paddingTop: 8 }} /></div>
          <div className="field"><label>Passphrase</label><input className="input" type="password" required autoComplete="off" value={rpass} onChange={(e) => setRpass(e.target.value)} /></div>
          <button className="btn" disabled={busy}><Icon name="upload" size={15} /> Restore</button>
        </form>
      </div>
      {result && (
        <div className="alert warn" style={{ marginTop: 18 }}>
          Restored {result.restored} project{result.restored === 1 ? '' : 's'} ({result.secrets} secrets).
          {result.skipped?.map((s) => <div key={s.project}>Skipped <b>{s.project}</b>: {s.reason}</div>)}
        </div>
      )}
    </section>
  )
}
