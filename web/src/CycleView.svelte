<script>
  // 矛盾环视图：逐条列出构成环的真实输入约束。
  export let cycle

  const KIND_LABEL = {
    'span-start': '父子 · 起点',
    'span-end': '父子 · 终点',
    'bound-min': '偏移下界',
    'bound-max': '偏移上界',
    anchor: '锚定',
  }

  const node = (n) => (n === 'ZERO' ? '0' : `x[${n}]`)
</script>

{#if cycle}
  <div class="cycle">
    <p class="sum">
      沿环对约束求和得 <code>0 ≤ {cycle.totalWeight} &lt; 0</code>，矛盾。
    </p>
    <ol>
      {#each cycle.edges as e, i}
        <li>
          <span class="idx">{i + 1}</span>
          <code class="expr">{node(e.to)} − {node(e.from)} ≤ {e.weight}</code>
          <span class="kind">{KIND_LABEL[e.kind] || e.kind}</span>
          <span class="detail">{e.detail}</span>
        </li>
      {/each}
    </ol>
  </div>
{/if}

<style>
  .cycle {
    margin-top: 8px;
  }
  .sum {
    font-size: 13px;
    color: #7f1d1d;
    background: #fef2f2;
    border: 1px solid #fecaca;
    border-radius: 6px;
    padding: 8px 12px;
  }
  ol {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  li {
    display: flex;
    align-items: baseline;
    gap: 10px;
    padding: 7px 4px;
    border-bottom: 1px dashed #f3f4f6;
    font-size: 13px;
    flex-wrap: wrap;
  }
  .idx {
    color: #9ca3af;
    font-family: ui-monospace, monospace;
    font-size: 12px;
    min-width: 18px;
  }
  .expr {
    font-family: ui-monospace, monospace;
    background: #f8fafc;
    border: 1px solid #e2e8f0;
    border-radius: 4px;
    padding: 2px 8px;
    white-space: nowrap;
  }
  .kind {
    font-size: 11px;
    color: #1d4ed8;
    background: #eff6ff;
    border-radius: 999px;
    padding: 1px 8px;
    white-space: nowrap;
  }
  .detail {
    color: #4b5563;
  }
</style>
