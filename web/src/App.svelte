<script>
  import Timeline from './Timeline.svelte'
  import CycleView from './CycleView.svelte'

  // ---------- 输入状态（示例数据预填，可直接求解） ----------
  let services = [
    { id: 'auth', minOffset: -8, maxOffset: 8, anchor: false },
    { id: 'db', minOffset: -8, maxOffset: 8, anchor: false },
    { id: 'gateway', minOffset: 0, maxOffset: 0, anchor: true },
  ]
  let spans = [
    { id: 's1', serviceId: 'gateway', parentId: '', start: 0, end: 60 },
    { id: 's2', serviceId: 'auth', parentId: 's1', start: 5, end: 30 },
    { id: 's3', serviceId: 'db', parentId: 's2', start: 2, end: 15 },
    { id: 's4', serviceId: 'db', parentId: 's1', start: 40, end: 55 },
  ]

  // ---------- 求解结果状态 ----------
  let result = null // 最近一次求解响应；任何输入编辑都会立即置空
  let error = ''
  let stale = false // 有未求解的输入修改
  let loading = false

  // 任何对偏移界或 span 的编辑都必须让旧结果立即失效。
  function invalidate() {
    result = null
    error = ''
    stale = true
  }

  const toInt = (v) => {
    const n = parseInt(v, 10)
    return Number.isFinite(n) ? n : 0
  }

  // ---------- 服务编辑 ----------
  function addService() {
    if (services.length >= 8) return
    let k = services.length
    while (services.some((s) => s.id === `svc${k}`)) k++
    services = [...services, { id: `svc${k}`, minOffset: -5, maxOffset: 5, anchor: false }]
    invalidate()
  }

  function removeService(id) {
    const removed = new Set(spans.filter((sp) => sp.serviceId === id).map((sp) => sp.id))
    spans = spans
      .filter((sp) => sp.serviceId !== id)
      .map((sp) => (removed.has(sp.parentId) ? { ...sp, parentId: '' } : sp))
    services = services.filter((s) => s.id !== id)
    invalidate()
  }

  function setAnchor(id) {
    services = services.map((s) =>
      s.id === id
        ? { ...s, anchor: true, minOffset: 0, maxOffset: 0 }
        : { ...s, anchor: false },
    )
    invalidate()
  }

  function patchService(id, patch) {
    services = services.map((s) => (s.id === id ? { ...s, ...patch } : s))
    invalidate()
  }

  // ---------- span 编辑 ----------
  function addSpan() {
    if (spans.length >= 100) return
    let k = spans.length + 1
    while (spans.some((sp) => sp.id === `s${k}`)) k++
    const svc = services[0] ? services[0].id : ''
    spans = [...spans, { id: `s${k}`, serviceId: svc, parentId: '', start: 0, end: 10 }]
    invalidate()
  }

  function removeSpan(id) {
    spans = spans
      .filter((sp) => sp.id !== id)
      .map((sp) => (sp.parentId === id ? { ...sp, parentId: '' } : sp))
    invalidate()
  }

  function patchSpan(id, patch) {
    spans = spans.map((sp) => (sp.id === id ? { ...sp, ...patch } : sp))
    invalidate()
  }

  // ---------- 求解 ----------
  async function solve() {
    loading = true
    try {
      const res = await fetch('/api/solve', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ services, spans }),
      })
      const data = await res.json()
      if (!res.ok) {
        error = data.error || '求解失败'
        result = null
      } else {
        result = data
        error = ''
        stale = false
      }
    } catch (e) {
      error = '请求失败：' + (e && e.message ? e.message : String(e))
      result = null
    } finally {
      loading = false
    }
  }

  $: correctedById = result && result.correctedSpans
    ? Object.fromEntries(result.correctedSpans.map((cs) => [cs.id, cs]))
    : {}
</script>

<main>
  <header>
    <h1>同步调用链 · 时钟偏移校正复核</h1>
    <p class="sub">
      2~8 个服务、至多 100 个 span；后端将父子包含关系化为差分约束，
      求按服务 ID 字典序最小的可行偏移向量。无解时仅展示由真实输入约束构成的矛盾环。
    </p>
  </header>

  <section class="panel">
    <div class="panel-head">
      <h2>服务与允许偏移区间</h2>
      <button on:click={addService} disabled={services.length >= 8}>+ 服务</button>
    </div>
    <table>
      <thead>
        <tr><th>服务 ID</th><th>偏移下界</th><th>偏移上界</th><th>锚定为 0</th><th></th></tr>
      </thead>
      <tbody>
        {#each services as s (s.id)}
          <tr>
            <td>
              <input
                class="id"
                value={s.id}
                on:input={(e) => patchService(s.id, { id: e.currentTarget.value })}
              />
            </td>
            <td>
              <input
                type="number"
                value={s.minOffset}
                disabled={s.anchor}
                on:input={(e) => patchService(s.id, { minOffset: toInt(e.currentTarget.value) })}
              />
            </td>
            <td>
              <input
                type="number"
                value={s.maxOffset}
                disabled={s.anchor}
                on:input={(e) => patchService(s.id, { maxOffset: toInt(e.currentTarget.value) })}
              />
            </td>
            <td>
              <input
                type="radio"
                name="anchor"
                checked={s.anchor}
                on:change={() => setAnchor(s.id)}
              />
            </td>
            <td>
              <button class="danger" on:click={() => removeService(s.id)} disabled={services.length <= 2}>
                删除
              </button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>

  <section class="panel">
    <div class="panel-head">
      <h2>Span（本地时钟整数时刻）</h2>
      <button on:click={addSpan} disabled={spans.length >= 100}>+ span</button>
    </div>
    <table>
      <thead>
        <tr><th>ID</th><th>服务</th><th>父 span</th><th>开始</th><th>结束</th><th></th></tr>
      </thead>
      <tbody>
        {#each spans as sp (sp.id)}
          <tr>
            <td>
              <input
                class="id"
                value={sp.id}
                on:input={(e) => patchSpan(sp.id, { id: e.currentTarget.value })}
              />
            </td>
            <td>
              <select
                value={sp.serviceId}
                on:change={(e) => patchSpan(sp.id, { serviceId: e.currentTarget.value })}
              >
                {#each services as s (s.id)}
                  <option value={s.id}>{s.id}</option>
                {/each}
              </select>
            </td>
            <td>
              <select
                value={sp.parentId}
                on:change={(e) => patchSpan(sp.id, { parentId: e.currentTarget.value })}
              >
                <option value="">（根）</option>
                {#each spans.filter((o) => o.id !== sp.id) as o (o.id)}
                  <option value={o.id}>{o.id}</option>
                {/each}
              </select>
            </td>
            <td>
              <input
                type="number"
                value={sp.start}
                on:input={(e) => patchSpan(sp.id, { start: toInt(e.currentTarget.value) })}
              />
            </td>
            <td>
              <input
                type="number"
                value={sp.end}
                on:input={(e) => patchSpan(sp.id, { end: toInt(e.currentTarget.value) })}
              />
            </td>
            <td><button class="danger" on:click={() => removeSpan(sp.id)}>删除</button></td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>

  <div class="actions">
    <button class="primary" on:click={solve} disabled={loading}>
      {loading ? '求解中…' : '求解'}
    </button>
    {#if stale && !result}
      <span class="notice">输入已修改，旧求解结果已失效，请重新求解。</span>
    {/if}
    {#if error}
      <span class="error">⚠ {error}</span>
    {/if}
  </div>

  {#if result}
    {#if result.feasible}
      <section class="panel">
        <h2>校正结果（按服务 ID 字典序最小）</h2>
        <div class="chips">
          {#each result.offsets as o (o.serviceId)}
            <span class="chip">
              {o.serviceId}：{o.offset >= 0 ? '+' : ''}{o.offset}
            </span>
          {/each}
        </div>
        <Timeline {result} />
      </section>

      <section class="panel">
        <h2>父子约束余量</h2>
        {#if result.margins && result.margins.length}
          <table>
            <thead>
              <tr>
                <th>子 span</th><th>父 span</th>
                <th>子校正区间</th><th>父校正区间</th>
                <th>起点余量</th><th>终点余量</th>
              </tr>
            </thead>
            <tbody>
              {#each result.margins as m (m.spanId)}
                {@const c = correctedById[m.spanId]}
                {@const p = correctedById[m.parentSpanId]}
                <tr>
                  <td>{m.spanId}</td>
                  <td>{m.parentSpanId}</td>
                  <td>{c ? `[${c.start}, ${c.end}]` : '—'}</td>
                  <td>{p ? `[${p.start}, ${p.end}]` : '—'}</td>
                  <td class:zero={m.startMargin === 0}>{m.startMargin}</td>
                  <td class:zero={m.endMargin === 0}>{m.endMargin}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {:else}
          <p class="dim">没有父子约束（全部为根 span）。</p>
        {/if}
      </section>
    {:else}
      <section class="panel infeasible">
        <h2>无可行偏移向量</h2>
        <p class="warn">
          约束系统矛盾，<strong>未绘制校正时间线</strong>（任何时间线都会是伪造的）。
          以下矛盾环完全由输入中的真实约束构成：
        </p>
        <CycleView cycle={result.cycle} />
      </section>
    {/if}
  {/if}
</main>

<style>
  :global(body) {
    margin: 0;
    font-family: 'PingFang SC', 'Microsoft YaHei', system-ui, sans-serif;
    background: #f4f6f8;
    color: #1f2933;
  }
  main {
    max-width: 1080px;
    margin: 0 auto;
    padding: 24px 16px 64px;
  }
  header h1 {
    font-size: 22px;
    margin: 0 0 6px;
  }
  .sub {
    color: #6b7280;
    font-size: 13px;
    margin: 0 0 20px;
  }
  .panel {
    background: #fff;
    border: 1px solid #e5e7eb;
    border-radius: 8px;
    padding: 16px;
    margin-bottom: 16px;
  }
  .panel h2 {
    font-size: 15px;
    margin: 0 0 12px;
  }
  .panel-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
  }
  .panel-head h2 {
    margin: 0;
  }
  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 13px;
  }
  th, td {
    text-align: left;
    padding: 6px 8px;
    border-bottom: 1px solid #eef0f2;
  }
  th {
    color: #6b7280;
    font-weight: 600;
  }
  input, select {
    font: inherit;
    padding: 4px 6px;
    border: 1px solid #d1d5db;
    border-radius: 4px;
    width: 90px;
    box-sizing: border-box;
  }
  input.id {
    width: 110px;
    font-family: ui-monospace, monospace;
  }
  input[type='radio'] {
    width: auto;
  }
  input:disabled {
    background: #f3f4f6;
    color: #9ca3af;
  }
  button {
    font: inherit;
    padding: 5px 12px;
    border: 1px solid #d1d5db;
    border-radius: 5px;
    background: #fff;
    cursor: pointer;
  }
  button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }
  button.primary {
    background: #2563eb;
    border-color: #2563eb;
    color: #fff;
    font-weight: 600;
    padding: 8px 28px;
  }
  button.danger {
    color: #b91c1c;
    border-color: #fecaca;
    background: #fff;
    padding: 3px 8px;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 14px;
    margin: 4px 0 18px;
  }
  .notice {
    color: #92400e;
    background: #fef3c7;
    border: 1px solid #fde68a;
    border-radius: 5px;
    padding: 5px 10px;
    font-size: 13px;
  }
  .error {
    color: #b91c1c;
    font-size: 13px;
  }
  .chips {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }
  .chip {
    font-family: ui-monospace, monospace;
    font-size: 13px;
    background: #eef2ff;
    border: 1px solid #c7d2fe;
    color: #3730a3;
    border-radius: 999px;
    padding: 3px 12px;
  }
  .zero {
    color: #b45309;
    font-weight: 700;
  }
  .dim {
    color: #9ca3af;
    font-size: 13px;
  }
  .infeasible {
    border-color: #fca5a5;
  }
  .warn {
    color: #991b1b;
    font-size: 13px;
  }
</style>
