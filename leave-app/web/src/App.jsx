import React, { useCallback, useEffect, useMemo, useState } from 'react';

// Tiny leave page: identity → Apply / My Requests / Approvals, live-polled
// from the leave service (which delegates the process to FlowForge).

const ME_KEY = 'leave-me';

const daysBetween = (from, to) => {
  if (!from || !to) return 0;
  const d = (new Date(to) - new Date(from)) / 86400000 + 1;
  return d > 0 ? Math.round(d) : 0;
};

function Pill({ verdict, live, escalated }) {
  const map = {
    approved: ['approved', 'ok'],
    rejected: ['rejected', 'no'],
    pending: live?.stage === 'Reporting Manager' ? ['manager review', 'wait']
      : live?.stage === 'HR Escalation' ? ['escalated 🔥', 'esc']
        : live?.stage === 'HR Final Approval' ? ['HR review', 'wait']
          : live?.status === 'failed' ? ['rejected', 'no']
            : live?.status === 'cancelled' ? ['rejected', 'no']
              : ['processing…', 'run'],
  };
  const [label, cls] = map[verdict ?? 'pending'] ?? ['?', 'run'];
  return <span className={`pill ${cls}`}>{escalated && verdict === 'pending' && live?.stage !== 'HR Escalation' ? 'escalated · ' : ''}{label}</span>;
}

export default function App() {
  const [org, setOrg] = useState([]);
  const [me, setMe] = useState(localStorage.getItem(ME_KEY) ?? 'E-4003');
  const [view, setView] = useState(null);
  const [tab, setTab] = useState('apply');
  const [form, setForm] = useState({ leave_type: 'annual', from: '', to: '', reason: '' });
  const [msg, setMsg] = useState(null);
  const [openSteps, setOpenSteps] = useState(null); // request id with the timeline expanded
  const [steps, setSteps] = useState({}); // id -> step timeline

  useEffect(() => { fetch('/api/org').then((r) => r.json()).then(setOrg).catch(() => {}); }, []);
  useEffect(() => { localStorage.setItem(ME_KEY, me); }, [me]);

  const refresh = useCallback(() => {
    fetch(`/api/view/${me}`).then((r) => r.json()).then(setView).catch(() => {});
  }, [me]);
  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 2500);
    return () => clearInterval(t);
  }, [refresh]);

  const days = useMemo(() => daysBetween(form.from, form.to), [form.from, form.to]);

  const submit = async () => {
    setMsg(null);
    const r = await fetch('/api/leave', {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ employee_no: me, ...form, days }),
    });
    const b = await r.json();
    if (!r.ok) return setMsg({ kind: 'err', text: b.error });
    setMsg({ kind: 'ok', text: `${b.id} submitted — routing to your manager.` });
    setForm({ leave_type: 'annual', from: '', to: '', reason: '' });
    setTab('mine');
    refresh();
  };

  const act = async (id, action) => {
    const r = await fetch(`/api/leave/${id}/${action}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ approver: view?.me?.name ?? me }),
    });
    if (r.ok) refresh();
  };

  const markRead = () => { fetch('/api/leave/read', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ emp: me }) }).then(refresh).catch(() => {}); };

  const toggleSteps = async (id) => {
    if (openSteps === id) return setOpenSteps(null);
    setOpenSteps(id);
    if (!steps[id]) {
      const r = await fetch(`/api/leave/${id}/steps`);
      const b = await r.json();
      if (r.ok) setSteps((s) => ({ ...s, [id]: b.steps ?? [] }));
    }
  };

  const badge = view?.approvals?.length ?? 0;
  const notifCount = view?.unread ?? 0;

  return (
    <div className="wrap">
      <header>
        <div className="brand">🌴 Meridian <b>Leave</b></div>
        <div className="who">
          <select value={me} onChange={(e) => setMe(e.target.value)}>
            {org.map((e) => <option key={e.no} value={e.no}>{e.no} · {e.name}{e.role === 'hr' ? ' (HR)' : ''}</option>)}
          </select>
          <span className="balance" title="leave balance · days pending">{view?.balance ?? '–'}d{view?.pendingDays ? <span className="pend"> · {view.pendingDays}d pending</span> : null}</span>
          <button className={`bell ${notifCount ? 'on' : ''}`} title="notifications" onClick={() => { setTab('mine'); markRead(); }}>🔔{notifCount ? ` ${notifCount}` : ''}</button>
        </div>
      </header>

      <nav>
        {['apply', 'mine', 'approvals'].map((t) => (
          <button key={t} className={tab === t ? 'on' : ''} onClick={() => setTab(t)}>
            {t === 'apply' ? 'Apply' : t === 'mine' ? 'My Requests' : 'Approvals'}
            {t === 'approvals' && badge ? <span className="count">{badge}</span> : null}
          </button>
        ))}
      </nav>

      {tab === 'apply' && (
        <section className="card">
          <h2>New leave request</h2>
          <div className="grid">
            <label>Type
              <select value={form.leave_type} onChange={(e) => setForm({ ...form, leave_type: e.target.value })}>
                {['annual', 'sick', 'casual', 'unpaid'].map((t) => <option key={t}>{t}</option>)}
              </select>
            </label>
            <label>From <input type="date" value={form.from} onChange={(e) => setForm({ ...form, from: e.target.value })} /></label>
            <label>To <input type="date" value={form.to} onChange={(e) => setForm({ ...form, to: e.target.value })} /></label>
            <label>Days <input readOnly value={days || ''} placeholder="—" /></label>
          </div>
          <label className="full">Reason <input value={form.reason} onChange={(e) => setForm({ ...form, reason: e.target.value })} placeholder="optional" /></label>
          {msg && <div className={`msg ${msg.kind}`}>{msg.text}</div>}
          <button className="primary" disabled={!days} onClick={submit}>Submit request</button>
          <p className="hint">Balance is validated live before final approval — and only approved requests deduct days.</p>
        </section>
      )}

      {tab === 'mine' && (
        <section className="card">
          <h2>My requests <span className="hint inline">· click a request for its workflow timeline</span></h2>
          {view?.notifications?.length > 0 && (
            <ul className="notifs">
              {view.notifications.slice(0, 4).map((n) => <li key={n.id}>🔔 {n.text}</li>)}
            </ul>
          )}
          {view?.requests?.length === 0 && <p className="empty">No requests yet — apply above.</p>}
          {view?.requests?.map((r) => (
            <div key={r.id} className="reqBlock">
              <div className="row clickable" onClick={() => toggleSteps(r.id)}>
                <div>
                  <b>{r.id}</b> · {r.leave_type} · {r.from} → {r.to} ({r.days}d)
                  {r.reason ? <span className="reason"> “{r.reason}”</span> : null}
                  {r.escalated && r.verdict == null ? <div className="escNote">escalated — manager missed the SLA, HR holds the card</div> : null}
                  {r.live?.error ? <div className="err">{r.live.error}</div> : null}
                </div>
                <Pill verdict={r.verdict} live={r.live} escalated={r.escalated} />
              </div>
              {openSteps === r.id && (
                <div className="timeline">
                  {(steps[r.id] ?? []).map((s) => (
                    <div key={s.stepId} className="tl">
                      <span className={`dot ${s.status}`} /> {s.name}
                      {s.note ? <span className="tlNote"> — {s.note}</span> : s.output ? <span className="tlNote"> — {s.output}</span> : null}
                    </div>
                  ))}
                  {(steps[r.id] ?? []).length === 0 && <span className="hint">loading timeline…</span>}
                </div>
              )}
            </div>
          ))}
        </section>
      )}

      {tab === 'approvals' && (
        <section className="card">
          <h2>Waiting for you</h2>
          {view?.approvals?.length === 0 && <p className="empty">Nothing to approve. 🎉</p>}
          {view?.approvals?.map((r) => (
            <div key={r.id} className="row approve">
              <div>
                <b>{r.id}</b> · {org.find((o) => o.no === r.employee_no)?.name ?? r.employee_no} · {r.leave_type}
                {' '}{r.from} → {r.to} ({r.days}d)
                <div className="hint inline">{r.live?.stage === 'HR Escalation'
                  ? '🔥 ESCALATED — manager missed the SLA; you hold the card'
                  : r.live?.stage === 'HR Final Approval' ? 'final approval (balance validated ✓)' : 'manager approval'}</div>
              </div>
              <div className="acts">
                <button className="ok" onClick={() => act(r.id, 'approve')}>✓ Approve</button>
                <button className="no" onClick={() => act(r.id, 'reject')}>✕ Reject</button>
              </div>
            </div>
          ))}
        </section>
      )}

      <footer>process engine: <a href="http://localhost:8081" target="_blank" rel="noreferrer">FlowForge</a> · leave-request workflow · audit on every step</footer>
    </div>
  );
}
