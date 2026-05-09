// sign-craze · Routing Editor SPA
// Preact 10 + htm (ESM, без npm build)

import { h, render, Fragment } from './vendor/preact.module.js';
import { useState, useEffect, useCallback, useRef } from './vendor/preact-hooks.module.js';
import htm from './vendor/htm.module.js';

const html = htm.bind(h);

// ---------------------------------------------------------------------------
// API helper — все запросы идут с credentials:'include' (Basic Auth из браузера)
// ---------------------------------------------------------------------------
const api = {
  get: (path) =>
    fetch(path, { credentials: 'include' })
      .then(r => r.ok ? r.json() : Promise.reject(new Error(`${r.status} ${r.statusText}`))),

  post: (path, body) =>
    fetch(path, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      credentials: 'include',
    }).then(r => r.ok ? r.json() : Promise.reject(new Error(`${r.status} ${r.statusText}`))),

  put: (path, body) =>
    fetch(path, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      credentials: 'include',
    }).then(r => r.ok ? r.json() : Promise.reject(new Error(`${r.status} ${r.statusText}`))),

  del: (path) =>
    fetch(path, { method: 'DELETE', credentials: 'include' })
      .then(r => r.ok),

  getRaw: (path) =>
    fetch(path, { credentials: 'include' })
      .then(r => r.ok ? r.text() : Promise.reject(new Error(`${r.status} ${r.statusText}`))),
};

// ---------------------------------------------------------------------------
// Toast — лёгкие уведомления
// ---------------------------------------------------------------------------
function Toast({ message, kind, onDone }) {
  useEffect(() => {
    const t = setTimeout(onDone, 3000);
    return () => clearTimeout(t);
  }, [onDone]);

  return html`<div class="toast ${kind}">${message}</div>`;
}

// ---------------------------------------------------------------------------
// Modal — универсальная обёртка
// ---------------------------------------------------------------------------
function Modal({ title, onClose, children }) {
  // Закрытие по Escape
  useEffect(() => {
    const handler = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  return html`
    <div class="modal-overlay" onClick=${(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div class="modal-content">
        <h3>${title}</h3>
        ${children}
      </div>
    </div>
  `;
}

// ---------------------------------------------------------------------------
// ChipsInput — мульти-ввод значений (домены, IP-адреса, порты и т.д.)
// ---------------------------------------------------------------------------
function ChipsInput({ values, onChange, placeholder }) {
  const [input, setInput] = useState('');
  const inputRef = useRef(null);

  const addChip = useCallback((val) => {
    const v = val.trim();
    if (v && !values.includes(v)) {
      onChange([...values, v]);
    }
    setInput('');
  }, [values, onChange]);

  const removeChip = useCallback((idx) => {
    onChange(values.filter((_, i) => i !== idx));
  }, [values, onChange]);

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addChip(input);
    } else if (e.key === 'Backspace' && input === '' && values.length > 0) {
      onChange(values.slice(0, -1));
    }
  };

  return html`
    <div class="chips-container" onClick=${() => inputRef.current?.focus()}>
      ${values.map((v, i) => html`
        <span class="chip" key=${v + i}>
          ${v}
          <button type="button" onClick=${(e) => { e.stopPropagation(); removeChip(i); }}>×</button>
        </span>
      `)}
      <input
        ref=${inputRef}
        class="chips-input"
        value=${input}
        placeholder=${values.length === 0 ? placeholder : ''}
        onInput=${(e) => setInput(e.target.value)}
        onKeyDown=${handleKeyDown}
        onBlur=${() => { if (input.trim()) addChip(input); }}
      />
    </div>
    <small style="color:var(--pico-muted-color)">Enter или , для добавления</small>
  `;
}

// ---------------------------------------------------------------------------
// InboundsTab — таблица inbound-интерфейсов с CRUD
// ---------------------------------------------------------------------------
const INBOUND_TYPES = ['tun', 'tproxy', 'mixed', 'socks', 'http'];

function InboundModal({ item, onSave, onClose }) {
  const isNew = !item;
  const [form, setForm] = useState(item || { tag: '', type: 'tun', settings: '{}' });
  const [err, setErr] = useState('');

  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));

  const handleSubmit = (e) => {
    e.preventDefault();
    let settings;
    try {
      settings = JSON.parse(form.settings || '{}');
    } catch {
      setErr('settings: некорректный JSON');
      return;
    }
    onSave({ ...form, settings });
  };

  return html`
    <${Modal} title=${isNew ? 'Добавить Inbound' : `Редактировать: ${item.tag}`} onClose=${onClose}>
      <form onSubmit=${handleSubmit}>
        <label>
          Tag
          <input required value=${form.tag} onInput=${(e) => set('tag', e.target.value)} />
        </label>
        <label>
          Тип
          <select value=${form.type} onChange=${(e) => set('type', e.target.value)}>
            ${INBOUND_TYPES.map(t => html`<option key=${t} value=${t}>${t}</option>`)}
          </select>
        </label>
        <label>
          Settings (JSON)
          <textarea
            rows="4"
            value=${form.settings || '{}'}
            onInput=${(e) => set('settings', e.target.value)}
            style="font-family:var(--pico-font-family-monospace);font-size:0.8rem"
          />
        </label>
        ${err && html`<p style="color:var(--pico-del-color)">${err}</p>`}
        <div class="modal-footer">
          <button type="button" class="secondary" onClick=${onClose}>Отмена</button>
          <button type="submit">Сохранить</button>
        </div>
      </form>
    </${Modal}>
  `;
}

function InboundsTab({ inbounds, onReload, showToast }) {
  const [modal, setModal] = useState(null); // null | 'add' | {item}

  const handleSave = async (data) => {
    try {
      if (modal === 'add') {
        await api.post('/api/inbounds', data);
        showToast('Inbound добавлен', 'success');
      } else {
        await api.put(`/api/inbounds/${modal.tag}`, data);
        showToast('Inbound обновлён', 'success');
      }
      setModal(null);
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  const handleDelete = async (tag) => {
    if (!confirm(`Удалить inbound "${tag}"?`)) return;
    try {
      await api.del(`/api/inbounds/${tag}`);
      showToast('Inbound удалён', 'success');
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  return html`
    <div>
      <div style="display:flex;justify-content:flex-end;margin-bottom:0.75rem">
        <button onClick=${() => setModal('add')}>+ Добавить</button>
      </div>
      ${inbounds.length === 0
        ? html`<p class="empty-state">Нет inbound-интерфейсов</p>`
        : html`
          <table>
            <thead>
              <tr><th>Tag</th><th>Тип</th><th>Действия</th></tr>
            </thead>
            <tbody>
              ${inbounds.map(ib => html`
                <tr key=${ib.tag}>
                  <td>${ib.tag}</td>
                  <td><code>${ib.type}</code></td>
                  <td>
                    <div class="action-cell">
                      <button class="secondary" onClick=${() => setModal({ ...ib, settings: JSON.stringify(ib.settings || {}, null, 2) })}>Изменить</button>
                      <button class="contrast" onClick=${() => handleDelete(ib.tag)}>Удалить</button>
                    </div>
                  </td>
                </tr>
              `)}
            </tbody>
          </table>
        `}
      ${modal && html`
        <${InboundModal}
          item=${modal === 'add' ? null : modal}
          onSave=${handleSave}
          onClose=${() => setModal(null)}
        />
      `}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// OutboundsTab — таблица outbound-соединений с CRUD
// ---------------------------------------------------------------------------
const OUTBOUND_TYPES = ['direct', 'block', 'vless', 'vmess', 'trojan', 'shadowsocks', 'http', 'socks'];
const PROXY_TYPES = new Set(['vless', 'vmess', 'trojan', 'shadowsocks', 'http', 'socks']);

function OutboundModal({ item, onSave, onClose }) {
  const isNew = !item;
  const [form, setForm] = useState(item || { tag: '', type: 'direct', server: '', server_port: '', uuid: '', password: '' });
  const [err, setErr] = useState('');

  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));
  const isProxy = PROXY_TYPES.has(form.type);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (isProxy && !form.server) { setErr('server обязателен для прокси-типов'); return; }
    const portNum = parseInt(form.server_port, 10);
    if (isProxy && (!portNum || portNum < 1 || portNum > 65535)) { setErr('port должен быть от 1 до 65535'); return; }

    const data = { tag: form.tag, type: form.type };
    if (isProxy) {
      data.server = form.server;
      data.server_port = portNum;
      if (form.uuid) data.uuid = form.uuid;
      if (form.password) data.password = form.password;
    }
    onSave(data);
  };

  return html`
    <${Modal} title=${isNew ? 'Добавить Outbound' : `Редактировать: ${item.tag}`} onClose=${onClose}>
      <form onSubmit=${handleSubmit}>
        <label>
          Tag
          <input required value=${form.tag} onInput=${(e) => set('tag', e.target.value)} />
        </label>
        <label>
          Тип
          <select value=${form.type} onChange=${(e) => set('type', e.target.value)}>
            ${OUTBOUND_TYPES.map(t => html`<option key=${t} value=${t}>${t}</option>`)}
          </select>
        </label>
        ${isProxy && html`
          <label>
            Server
            <input required value=${form.server} onInput=${(e) => set('server', e.target.value)} placeholder="example.com" />
          </label>
          <label>
            Port
            <input required type="number" min="1" max="65535" value=${form.server_port} onInput=${(e) => set('server_port', e.target.value)} />
          </label>
          ${(form.type === 'vless' || form.type === 'vmess') && html`
            <label>
              UUID
              <input value=${form.uuid} onInput=${(e) => set('uuid', e.target.value)} placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx" />
            </label>
          `}
          ${(form.type === 'trojan' || form.type === 'shadowsocks' || form.type === 'http' || form.type === 'socks') && html`
            <label>
              Password
              <input type="password" value=${form.password} onInput=${(e) => set('password', e.target.value)} />
            </label>
          `}
        `}
        ${err && html`<p style="color:var(--pico-del-color)">${err}</p>`}
        <div class="modal-footer">
          <button type="button" class="secondary" onClick=${onClose}>Отмена</button>
          <button type="submit">Сохранить</button>
        </div>
      </form>
    </${Modal}>
  `;
}

function OutboundsTab({ outbounds, onReload, showToast }) {
  const [modal, setModal] = useState(null);

  const handleSave = async (data) => {
    try {
      if (modal === 'add') {
        await api.post('/api/outbounds', data);
        showToast('Outbound добавлен', 'success');
      } else {
        await api.put(`/api/outbounds/${modal.tag}`, data);
        showToast('Outbound обновлён', 'success');
      }
      setModal(null);
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  const handleDelete = async (tag) => {
    if (!confirm(`Удалить outbound "${tag}"?`)) return;
    try {
      await api.del(`/api/outbounds/${tag}`);
      showToast('Outbound удалён', 'success');
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  return html`
    <div>
      <div style="display:flex;justify-content:flex-end;margin-bottom:0.75rem">
        <button onClick=${() => setModal('add')}>+ Добавить</button>
      </div>
      ${outbounds.length === 0
        ? html`<p class="empty-state">Нет outbound-соединений</p>`
        : html`
          <table>
            <thead>
              <tr><th>Tag</th><th>Тип</th><th>Server</th><th>Действия</th></tr>
            </thead>
            <tbody>
              ${outbounds.map(ob => html`
                <tr key=${ob.tag}>
                  <td>${ob.tag}</td>
                  <td><code>${ob.type}</code></td>
                  <td>${ob.server ? `${ob.server}:${ob.server_port}` : '—'}</td>
                  <td>
                    <div class="action-cell">
                      <button class="secondary" onClick=${() => setModal({ ...ob, server_port: ob.server_port ? String(ob.server_port) : '' })}>Изменить</button>
                      <button class="contrast" onClick=${() => handleDelete(ob.tag)}>Удалить</button>
                    </div>
                  </td>
                </tr>
              `)}
            </tbody>
          </table>
        `}
      ${modal && html`
        <${OutboundModal}
          item=${modal === 'add' ? null : modal}
          onSave=${handleSave}
          onClose=${() => setModal(null)}
        />
      `}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// RulesTab — список правил маршрутизации с drag-and-drop reorder
// ---------------------------------------------------------------------------
const RULE_ACTIONS = ['route', 'block', 'sniff', 'resolve', 'hijack-dns'];

function RuleModal({ item, index, outbounds, onSave, onClose }) {
  const isNew = index === null;
  const defaultForm = {
    action: 'route',
    outbound: '',
    domain: [],
    ip_cidr: [],
    rule_set: [],
    port: [],
    invert: false,
    network: '',
    protocol: '',
  };
  const [form, setForm] = useState(item ? {
    ...defaultForm,
    ...item,
    domain: item.domain || [],
    ip_cidr: item.ip_cidr || [],
    rule_set: item.rule_set || [],
    port: (item.port || []).map(String),
  } : defaultForm);

  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));

  const handleSubmit = (e) => {
    e.preventDefault();
    const data = { ...form };
    // Очищаем пустые массивы
    if (data.domain.length === 0) delete data.domain;
    if (data.ip_cidr.length === 0) delete data.ip_cidr;
    if (data.rule_set.length === 0) delete data.rule_set;
    if (data.port.length === 0) {
      delete data.port;
    } else {
      data.port = data.port.map(Number);
    }
    if (!data.network) delete data.network;
    if (!data.protocol) delete data.protocol;
    if (!data.outbound || data.action !== 'route') delete data.outbound;
    onSave(data);
  };

  const proxyTags = outbounds.filter(o => o.type !== 'direct' && o.type !== 'block').map(o => o.tag);

  return html`
    <${Modal} title=${isNew ? 'Добавить правило' : `Редактировать правило #${index}`} onClose=${onClose}>
      <form onSubmit=${handleSubmit}>
        <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
          <label>
            Action
            <select value=${form.action} onChange=${(e) => set('action', e.target.value)}>
              ${RULE_ACTIONS.map(a => html`<option key=${a} value=${a}>${a}</option>`)}
            </select>
          </label>
          ${form.action === 'route' && html`
            <label>
              Outbound
              <select value=${form.outbound} onChange=${(e) => set('outbound', e.target.value)}>
                <option value="">— не задан —</option>
                ${proxyTags.map(t => html`<option key=${t} value=${t}>${t}</option>`)}
              </select>
            </label>
          `}
        </div>

        <label>
          Domain
          <${ChipsInput} values=${form.domain} onChange=${(v) => set('domain', v)} placeholder="example.com" />
        </label>

        <label>
          IP CIDR
          <${ChipsInput} values=${form.ip_cidr} onChange=${(v) => set('ip_cidr', v)} placeholder="192.168.0.0/16" />
        </label>

        <label>
          Rule Set tags
          <${ChipsInput} values=${form.rule_set} onChange=${(v) => set('rule_set', v)} placeholder="geoip-ru" />
        </label>

        <label>
          Ports
          <${ChipsInput} values=${form.port} onChange=${(v) => set('port', v)} placeholder="80" />
        </label>

        <div style="display:grid;grid-template-columns:1fr 1fr;gap:0.75rem">
          <label>
            Network
            <select value=${form.network} onChange=${(e) => set('network', e.target.value)}>
              <option value="">любой</option>
              <option value="tcp">tcp</option>
              <option value="udp">udp</option>
            </select>
          </label>
          <label>
            Protocol
            <input value=${form.protocol} onInput=${(e) => set('protocol', e.target.value)} placeholder="dns, quic, ..." />
          </label>
        </div>

        <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer">
          <input type="checkbox" checked=${form.invert} onChange=${(e) => set('invert', e.target.checked)} style="width:auto;margin:0" />
          Invert (инвертировать условие)
        </label>

        <div class="modal-footer">
          <button type="button" class="secondary" onClick=${onClose}>Отмена</button>
          <button type="submit">Сохранить</button>
        </div>
      </form>
    </${Modal}>
  `;
}

function RulesTab({ rules, outbounds, onReload, showToast }) {
  const [modal, setModal] = useState(null); // null | 'add' | {item, index}
  const [dragging, setDragging] = useState(null);
  const [dragOver, setDragOver] = useState(null);

  const handleSave = async (data) => {
    try {
      if (modal === 'add') {
        await api.post('/api/rules', data);
        showToast('Правило добавлено', 'success');
      } else {
        await api.put(`/api/rules/${modal.index}`, data);
        showToast('Правило обновлено', 'success');
      }
      setModal(null);
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  const handleDelete = async (idx) => {
    if (!confirm(`Удалить правило #${idx}?`)) return;
    try {
      await api.del(`/api/rules/${idx}`);
      showToast('Правило удалено', 'success');
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  // Drag-and-drop через HTML5 DnD API
  const handleDragStart = (e, idx) => {
    setDragging(idx);
    e.dataTransfer.effectAllowed = 'move';
  };

  const handleDragOver = (e, idx) => {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    setDragOver(idx);
  };

  const handleDrop = async (e, idx) => {
    e.preventDefault();
    setDragOver(null);
    if (dragging === null || dragging === idx) { setDragging(null); return; }
    try {
      await api.post('/api/rules/reorder', { from: dragging, to: idx });
      onReload();
    } catch (e) {
      showToast(`Ошибка перестановки: ${e.message}`, 'error');
    }
    setDragging(null);
  };

  const handleDragEnd = () => { setDragging(null); setDragOver(null); };

  const summarise = (rule) => {
    const parts = [];
    if (rule.domain?.length)   parts.push(`domain:${rule.domain.length}`);
    if (rule.ip_cidr?.length)  parts.push(`cidr:${rule.ip_cidr.length}`);
    if (rule.rule_set?.length) parts.push(`set:${rule.rule_set.join(',')}`);
    if (rule.port?.length)     parts.push(`port:${rule.port.join(',')}`);
    if (rule.network)          parts.push(`net:${rule.network}`);
    if (rule.protocol)         parts.push(`proto:${rule.protocol}`);
    if (parts.length === 0) return '(нет условий)';
    return parts.join(' ');
  };

  return html`
    <div>
      <div style="display:flex;justify-content:flex-end;margin-bottom:0.75rem">
        <button onClick=${() => setModal('add')}>+ Добавить</button>
      </div>
      ${rules.length === 0
        ? html`<p class="empty-state">Нет правил маршрутизации</p>`
        : html`
          <table>
            <thead>
              <tr><th style="width:2rem"></th><th>#</th><th>Action</th><th>Условия</th><th>Outbound</th><th>Действия</th></tr>
            </thead>
            <tbody>
              ${rules.map((rule, i) => html`
                <tr
                  key=${i}
                  class=${dragOver === i ? 'drag-over' : ''}
                  draggable="true"
                  onDragStart=${(e) => handleDragStart(e, i)}
                  onDragOver=${(e) => handleDragOver(e, i)}
                  onDrop=${(e) => handleDrop(e, i)}
                  onDragEnd=${handleDragEnd}
                >
                  <td><span class="handle" title="Перетащить для изменения порядка">⠿</span></td>
                  <td>${i}</td>
                  <td><code>${rule.action}</code>${rule.invert ? html` <sup title="invert">!</sup>` : ''}</td>
                  <td style="font-size:0.8rem">${summarise(rule)}</td>
                  <td>${rule.outbound || '—'}</td>
                  <td>
                    <div class="action-cell">
                      <button class="secondary" onClick=${() => setModal({ item: rule, index: i })}>Изм.</button>
                      <button class="contrast" onClick=${() => handleDelete(i)}>Удал.</button>
                    </div>
                  </td>
                </tr>
              `)}
            </tbody>
          </table>
        `}
      ${modal && html`
        <${RuleModal}
          item=${modal === 'add' ? null : modal.item}
          index=${modal === 'add' ? null : modal.index}
          outbounds=${outbounds}
          onSave=${handleSave}
          onClose=${() => setModal(null)}
        />
      `}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// RuleSetsTab — таблица rule_sets (geo-данные, списки)
// ---------------------------------------------------------------------------
const RULESET_TYPES = ['remote', 'local', 'inline'];

function RuleSetModal({ onSave, onClose }) {
  const [form, setForm] = useState({ tag: '', type: 'remote', url: '', path: '' });
  const set = (k, v) => setForm(f => ({ ...f, [k]: v }));

  const handleSubmit = (e) => {
    e.preventDefault();
    onSave(form);
  };

  return html`
    <${Modal} title="Добавить Rule Set" onClose=${onClose}>
      <form onSubmit=${handleSubmit}>
        <label>
          Tag
          <input required value=${form.tag} onInput=${(e) => set('tag', e.target.value)} />
        </label>
        <label>
          Тип
          <select value=${form.type} onChange=${(e) => set('type', e.target.value)}>
            ${RULESET_TYPES.map(t => html`<option key=${t} value=${t}>${t}</option>`)}
          </select>
        </label>
        ${form.type === 'remote' && html`
          <label>
            URL
            <input type="url" value=${form.url} onInput=${(e) => set('url', e.target.value)} placeholder="https://..." />
          </label>
        `}
        ${form.type === 'local' && html`
          <label>
            Path
            <input value=${form.path} onInput=${(e) => set('path', e.target.value)} placeholder="/opt/etc/sign-craze/rules.srs" />
          </label>
        `}
        <div class="modal-footer">
          <button type="button" class="secondary" onClick=${onClose}>Отмена</button>
          <button type="submit">Сохранить</button>
        </div>
      </form>
    </${Modal}>
  `;
}

function RuleSetsTab({ ruleSets, onReload, showToast }) {
  const [showAdd, setShowAdd] = useState(false);

  const handleSave = async (data) => {
    try {
      await api.post('/api/rule_sets', data);
      showToast('Rule Set добавлен', 'success');
      setShowAdd(false);
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  const handleDelete = async (tag) => {
    if (!confirm(`Удалить rule_set "${tag}"?`)) return;
    try {
      await api.del(`/api/rule_sets/${tag}`);
      showToast('Rule Set удалён', 'success');
      onReload();
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  return html`
    <div>
      <div style="display:flex;justify-content:flex-end;margin-bottom:0.75rem">
        <button onClick=${() => setShowAdd(true)}>+ Добавить</button>
      </div>
      ${ruleSets.length === 0
        ? html`<p class="empty-state">Нет rule sets</p>`
        : html`
          <table>
            <thead>
              <tr><th>Tag</th><th>Тип</th><th>URL / Path</th><th>Действия</th></tr>
            </thead>
            <tbody>
              ${ruleSets.map(rs => html`
                <tr key=${rs.tag}>
                  <td>${rs.tag}</td>
                  <td><code>${rs.type}</code></td>
                  <td style="font-size:0.8rem;word-break:break-all">${rs.url || rs.path || '—'}</td>
                  <td>
                    <button class="contrast" onClick=${() => handleDelete(rs.tag)}>Удалить</button>
                  </td>
                </tr>
              `)}
            </tbody>
          </table>
        `}
      ${showAdd && html`
        <${RuleSetModal} onSave=${handleSave} onClose=${() => setShowAdd(false)} />
      `}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// PresetsDropdown — выбор и применение пресетов
// ---------------------------------------------------------------------------
function PresetsDropdown({ onReload, showToast }) {
  const [presets, setPresets] = useState([]);
  // visible управляет видимостью выпадающего списка
  const [visible, setVisible] = useState(false);
  const containerRef = useRef(null);

  useEffect(() => {
    api.get('/api/presets').then(setPresets).catch(() => {});
  }, []);

  // Закрытие по клику вне компонента
  useEffect(() => {
    if (!visible) return;
    const handler = (e) => {
      if (containerRef.current && !containerRef.current.contains(e.target)) {
        setVisible(false);
      }
    };
    document.addEventListener('mousedown', handler);
    return () => document.removeEventListener('mousedown', handler);
  }, [visible]);

  // apply делается fire-and-forget: сначала запускаем запрос, потом скрываем
  // dropdown. Если скрыть сначала, preact unmount'ит preset-item между
  // mousedown и обработчиком и клик теряется (родитель уже re-render'ится).
  const apply = (name) => {
    api.post(`/api/presets/${name}/apply`, {})
      .then(() => {
        showToast(`Пресет "${name}" применён`, 'success');
        onReload();
      })
      .catch(e => showToast(`Ошибка применения пресета: ${e.message}`, 'error'))
      .finally(() => setVisible(false));
  };

  if (presets.length === 0) return null;

  return html`
    <div
      ref=${containerRef}
      style="position:relative"
    >
      <button
        class="secondary"
        onClick=${() => setVisible(v => !v)}
      >Пресеты ▾</button>
      ${visible && html`
        <div style="
          position:absolute;top:100%;right:0;
          background:var(--pico-background-color);
          border:1px solid var(--pico-muted-border-color);
          border-radius:0.25rem;
          min-width:200px;z-index:200;
          box-shadow:0 4px 12px rgba(0,0,0,0.15)
        ">
          ${presets.map(p => html`
            <div
              key=${p.name}
              class="preset-item"
              onClick=${() => apply(p.name)}
              title=${p.description || ''}
            >
              <strong>${p.name}</strong>
              ${p.description && html`<br/><small style="color:var(--pico-muted-color)">${p.description}</small>`}
            </div>
          `)}
        </div>
      `}
    </div>
  `;
}

// ---------------------------------------------------------------------------
// Toolbar — Validate / Preview / Apply (всегда виден)
// ---------------------------------------------------------------------------
function Toolbar({ onReload, showToast }) {
  const [previewModal, setPreviewModal] = useState(false);
  const [previewText, setPreviewText] = useState('');
  const [restartModal, setRestartModal] = useState(false);

  const handleValidate = async () => {
    try {
      const res = await fetch('/api/validate', { method: 'POST', credentials: 'include' })
        .then(r => r.ok ? r.json() : Promise.reject(new Error(`${r.status} ${r.statusText}`)));
      if (res.ok) {
        showToast('Конфигурация валидна', 'success');
      } else {
        alert('Ошибки валидации:\n' + (res.errors || []).join('\n'));
      }
    } catch (e) {
      showToast(`Ошибка: ${e.message}`, 'error');
    }
  };

  const handlePreview = async () => {
    try {
      const text = await api.getRaw('/api/preview');
      setPreviewText(text);
      setPreviewModal(true);
    } catch (e) {
      showToast(`Ошибка preview: ${e.message}`, 'error');
    }
  };

  const handleApply = async () => {
    try {
      const res = await api.post('/api/apply', {});
      if (res.needs_restart) {
        setRestartModal(true);
      } else {
        showToast('Изменения применены', 'success');
        onReload();
      }
    } catch (e) {
      showToast(`Ошибка apply: ${e.message}`, 'error');
    }
  };

  return html`
    <div class="toolbar">
      <h1>sign-craze · Routing Editor</h1>
      <${PresetsDropdown} onReload=${onReload} showToast=${showToast} />
      <button class="secondary" onClick=${handleValidate}>Validate</button>
      <button class="secondary" onClick=${handlePreview}>Preview</button>
      <button onClick=${handleApply}>Apply</button>
    </div>

    ${previewModal && html`
      <${Modal} title="Preview: config.json" onClose=${() => setPreviewModal(false)}>
        <pre class="config-preview"><code>${previewText}</code></pre>
        <div class="modal-footer">
          <button onClick=${() => setPreviewModal(false)}>Закрыть</button>
        </div>
      </${Modal}>
    `}

    ${restartModal && html`
      <${Modal} title="Изменения сохранены" onClose=${() => setRestartModal(false)}>
        <p>Изменения сохранены. Запустите <code>sign-craze --restart</code> чтобы применить.</p>
        <div class="modal-footer">
          <button onClick=${() => setRestartModal(false)}>OK</button>
        </div>
      </${Modal}>
    `}
  `;
}

// ---------------------------------------------------------------------------
// Footer — ссылки на связанные проекты на GitHub
// ---------------------------------------------------------------------------
function Footer() {
  return html`
    <footer style="text-align:center;padding:1rem;font-size:0.85em;color:var(--pico-muted-color)">
      <a href="https://github.com/kittylabassistant/sign-craze" target="_blank" rel="noopener noreferrer">
        sign-craze on GitHub
      </a>
    </footer>
  `;
}

// ---------------------------------------------------------------------------
// App — корневой компонент
// ---------------------------------------------------------------------------
const TABS = [
  { id: 'inbounds',  label: 'Inbounds'  },
  { id: 'outbounds', label: 'Outbounds' },
  { id: 'routing',   label: 'Routing'   },
  { id: 'rulesets',  label: 'Rule Sets' },
];

function App() {
  const [tab, setTab] = useState('inbounds');
  const [state, setState] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [toast, setToast] = useState(null); // {message, kind}

  const showToast = useCallback((message, kind = 'success') => {
    setToast({ message, kind });
  }, []);

  const loadState = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      const data = await api.get('/api/state');
      setState(data);
    } catch (e) {
      setError(`Ошибка загрузки конфигурации: ${e.message}`);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => { loadState(); }, [loadState]);

  const cfg = state?.config || {};
  const inbounds  = cfg.inbounds  || [];
  const outbounds = cfg.outbounds || [];
  const rules     = cfg.rules     || [];
  const ruleSets  = cfg.rule_sets || [];

  return html`
    <${Fragment}>
      <${Toolbar} onReload=${loadState} showToast=${showToast} />

      ${loading && html`<p aria-busy="true">Загрузка…</p>`}

      ${error && html`
        <article style="color:var(--pico-del-color)">
          ${error}
          <button class="secondary" style="margin-left:1rem" onClick=${loadState}>Повторить</button>
        </article>
      `}

      ${!loading && !error && html`
        <div class="tabs" role="tablist">
          ${TABS.map(t => html`
            <button
              key=${t.id}
              role="tab"
              class=${tab === t.id ? 'active' : ''}
              aria-selected=${tab === t.id}
              onClick=${() => setTab(t.id)}
            >
              ${t.label}
            </button>
          `)}
        </div>

        ${tab === 'inbounds' && html`
          <${InboundsTab} inbounds=${inbounds} onReload=${loadState} showToast=${showToast} />
        `}
        ${tab === 'outbounds' && html`
          <${OutboundsTab} outbounds=${outbounds} onReload=${loadState} showToast=${showToast} />
        `}
        ${tab === 'routing' && html`
          <${RulesTab} rules=${rules} outbounds=${outbounds} onReload=${loadState} showToast=${showToast} />
        `}
        ${tab === 'rulesets' && html`
          <${RuleSetsTab} ruleSets=${ruleSets} onReload=${loadState} showToast=${showToast} />
        `}
      `}

      ${toast && html`
        <${Toast}
          message=${toast.message}
          kind=${toast.kind}
          onDone=${() => setToast(null)}
        />
      `}

      <${Footer} />
    </${Fragment}>
  `;
}

// Монтируем приложение в DOM
render(html`<${App} />`, document.getElementById('app'));
