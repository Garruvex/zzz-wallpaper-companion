(() => {
  const operations = globalThis.CompanionContract.operations;
  const active = new Set();
  function build(id, { path = {}, query = {}, headers = {}, body } = {}) {
    if (!Object.prototype.hasOwnProperty.call(operations, id)) throw Error(`Unknown operation: ${id}`);
    const op = operations[id], mapped = {}, search = [];
    let route = op.path;
    for (const [where, values] of [['path', path], ['query', query], ['header', headers]]) {
      for (const name of Object.keys(values)) if (!op.parameters.some(p => p.in === where && p.name === name)) throw Error(`${id}: unknown ${where} parameter ${name}`);
    }
    for (const p of op.parameters) {
      const value = (p.in === 'path' ? path : p.in === 'query' ? query : headers)[p.name];
      if (value === undefined || value === null) { if (p.required) throw Error(`${id}: missing ${p.name}`); continue; }
      if (!['string', 'number', 'boolean'].includes(typeof value)) throw Error(`${id}: expected scalar ${p.name}`);
      if (p.in === 'path') {
        if (['', '.', '..'].includes(String(value))) throw Error(`${id}: invalid path parameter ${p.name}`);
        route = route.replace(`{${p.name}}`, encodeURIComponent(value));
      } else if (p.in === 'query') search.push(`${encodeURIComponent(p.name)}=${encodeURIComponent(value)}`);
      else mapped[p.name] = String(value);
    }
    if (op.bodyRequired && body === undefined) throw Error(`${id}: missing request body`);
    if (!op.bodyType && body !== undefined) throw Error(`${id}: unexpected request body`);
    return { op, path: route + (search.length ? '?' + search.join('&') : ''), headers: mapped, body };
  }
  async function request(id, parameters = {}, { signal, timeout } = {}) {
    const built = build(id, parameters), op = built.op;
    if (op.responseType !== 'json') throw Error(`${id}: use a native binary/stream consumer`);
    const controller = new AbortController(); active.add(controller);
    let category = 'cancelled';
    const cancel = () => controller.abort();
    if (signal?.aborted) cancel(); else signal?.addEventListener('abort', cancel, { once: true });
    const timer = setTimeout(() => { category = 'timeout'; cancel(); }, timeout ?? op.timeout ?? 60000);
    try {
      if (controller.signal.aborted) throw Error('Request cancelled');
      const headers = { ...built.headers };
      if (built.body !== undefined) headers['Content-Type'] = op.bodyType;
      const response = await globalThis.fetch(built.path, { method: op.method, headers,
        body: op.bodyType === 'application/json' && built.body !== undefined ? JSON.stringify(built.body) : built.body,
        signal: controller.signal, cache: 'no-store' });
      const text = response.status === 204 ? '' : await response.text();
      let data = null, invalid = false;
      if (text) { try { data = JSON.parse(text); } catch { invalid = true; data = text; } }
      if (controller.signal.aborted) throw Error('Request cancelled');
      if (!response.ok) throw Object.assign(Error(data?.error || `Companion returned ${response.status}`), { status: response.status, payload: data, category: response.status === 426 ? 'incompatible' : 'api' });
      if (response.status !== 204 && (invalid || !text)) throw Object.assign(Error('Companion returned invalid JSON'), { status: response.status, payload: data, category: 'invalid-response' });
      return data;
    } catch (error) {
      error.operationId = id;
      if (controller.signal.aborted) { error.name = 'AbortError'; error.category = category; }
      else error.category ??= 'offline';
      throw error;
    } finally { clearTimeout(timer); signal?.removeEventListener('abort', cancel); active.delete(controller); }
  }
  function url(id, parameters = {}) {
    const built = build(id, parameters);
    if (built.op.method !== 'GET' || built.op.bodyType || Object.keys(built.headers).length) throw Error(`${id}: cannot build browser URL`);
    return built.path;
  }
  globalThis.addEventListener('pagehide', () => { for (const controller of active) controller.abort(); });
  globalThis.CompanionAPI = Object.freeze({ request, url });
})();
