<script>
  // 校正时间线：完全由求解响应（offsets + correctedSpans）绘制，
  // 不读取页面当前输入，保证“所见即所解”。
  export let result

  const WIDTH = 1000
  const PAD_L = 120
  const PAD_R = 24
  const PAD_T = 30
  const LANE_H = 24
  const LANE_GAP = 6
  const GROUP_GAP = 14
  const AXIS_H = 26

  const PALETTE = ['#2563eb', '#059669', '#d97706', '#dc2626', '#7c3aed', '#0891b2', '#db2777', '#65a30d']

  $: offsets = result.offsets || []
  $: spans = result.correctedSpans || []
  $: colorOf = Object.fromEntries(offsets.map((o, i) => [o.serviceId, PALETTE[i % PALETTE.length]]))

  // 每个服务内部按起点贪心分配泳道，避免同服务条块重叠。
  $: groups = offsets.map((o) => {
    const mine = spans
      .filter((sp) => sp.serviceId === o.serviceId)
      .slice()
      .sort((a, b) => a.start - b.start || a.end - b.end)
    const laneEnds = []
    const placed = mine.map((sp) => {
      let lane = laneEnds.findIndex((end) => end <= sp.start)
      if (lane === -1) {
        lane = laneEnds.length
        laneEnds.push(-Infinity)
      }
      laneEnds[lane] = sp.end
      return { ...sp, lane }
    })
    return { serviceId: o.serviceId, spans: placed, lanes: Math.max(1, laneEnds.length) }
  })

  $: minT = spans.length ? Math.min(...spans.map((s) => s.start)) : 0
  $: maxT = spans.length ? Math.max(...spans.map((s) => s.end)) : 1
  $: range = Math.max(1, maxT - minT)
  $: x = (t) => PAD_L + ((t - minT) / range) * (WIDTH - PAD_L - PAD_R)

  // 刻度：1/2/5 阶梯，目标约 10 个刻度。
  $: tickStep = niceStep(range / 10)
  $: ticks = (() => {
    const out = []
    for (let t = Math.ceil(minT / tickStep) * tickStep; t <= maxT; t += tickStep) out.push(t)
    return out
  })()

  function niceStep(raw) {
    const pow = Math.pow(10, Math.floor(Math.log10(Math.max(raw, 1e-9))))
    for (const m of [1, 2, 5, 10]) {
      if (m * pow >= raw) return m * pow
    }
    return 10 * pow
  }

  // 各服务分组的纵向位置。
  $: rows = (() => {
    let y = PAD_T
    return groups.map((g) => {
      const h = g.lanes * (LANE_H + LANE_GAP) - LANE_GAP
      const row = { ...g, y, h }
      y += h + GROUP_GAP
      return row
    })
  })()
  $: height = PAD_T + rows.reduce((acc, r) => acc + r.h + GROUP_GAP, 0) + AXIS_H
  $: rowOf = Object.fromEntries(rows.map((r) => [r.serviceId, r]))
  $: barOf = (() => {
    const m = {}
    for (const r of rows) {
      for (const sp of r.spans) {
        m[sp.id] = {
          x1: x(sp.start),
          x2: x(sp.end),
          y: r.y + sp.lane * (LANE_H + LANE_GAP),
          parentId: sp.parentId,
        }
      }
    }
    return m
  })()
</script>

<svg viewBox="0 0 {WIDTH} {height}" width="100%" role="img" aria-label="校正时间线">
  <defs>
    <marker id="arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
      <path d="M0,0 L8,4 L0,8 z" fill="#9ca3af" />
    </marker>
  </defs>

  <!-- 网格与刻度 -->
  {#each ticks as t}
    <line x1={x(t)} y1={PAD_T - 8} x2={x(t)} y2={height - AXIS_H} stroke="#eef0f2" />
    <text x={x(t)} y={height - 8} font-size="11" fill="#6b7280" text-anchor="middle">{t}</text>
  {/each}
  <line x1={PAD_L} y1={height - AXIS_H} x2={WIDTH - PAD_R} y2={height - AXIS_H} stroke="#d1d5db" />

  <!-- 父子连线（先画线，条块盖在上层） -->
  {#each spans.filter((s) => s.parentId && barOf[s.parentId]) as sp}
    {@const c = barOf[sp.id]}
    {@const p = barOf[sp.parentId]}
    <line
      x1={c.x1}
      y1={c.y}
      x2={p.x1}
      y2={p.y + LANE_H}
      stroke="#9ca3af"
      stroke-dasharray="4 3"
      marker-end="url(#arrow)"
    />
  {/each}

  <!-- 服务行与 span 条块 -->
  {#each rows as row}
    <text x={PAD_L - 10} y={row.y + row.h / 2 + 4} font-size="12" font-weight="600" text-anchor="end" fill="#374151">
      {row.serviceId}
    </text>
    {#each row.spans as sp}
      {@const bx = x(sp.start)}
      {@const bw = Math.max(2, x(sp.end) - x(sp.start))}
      {@const by = row.y + sp.lane * (LANE_H + LANE_GAP)}
      <rect x={bx} y={by} width={bw} height={LANE_H} rx="4" fill={colorOf[sp.serviceId]} opacity="0.88" />
      {#if bw > 74}
        <text x={bx + 6} y={by + 16} font-size="11" fill="#fff">{sp.id} [{sp.start}, {sp.end}]</text>
      {:else}
        <text x={bx + bw / 2} y={by - 4} font-size="10" fill="#4b5563" text-anchor="middle">
          {sp.id} [{sp.start}, {sp.end}]
        </text>
      {/if}
    {/each}
  {/each}
</svg>
