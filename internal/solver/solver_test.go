package solver

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
)

// ---------- 穷举对拍工具 ----------

// bruteForce 枚举所有落在区间内的整数偏移向量，
// 返回是否存在可行解以及其中按服务 ID 字典序最小的向量。
func bruteForce(in *Input) (bool, []int) {
	services := append([]Service(nil), in.Services...)
	sort.Slice(services, func(i, j int) bool { return services[i].ID < services[j].ID })
	idx := make(map[string]int, len(services))
	for i, s := range services {
		idx[s.ID] = i
	}
	var best []int
	cur := make([]int, len(services))
	var dfs func(i int)
	dfs = func(i int) {
		if i == len(services) {
			if assignmentFeasible(in, idx, cur) && (best == nil || lexLess(cur, best)) {
				best = append([]int(nil), cur...)
			}
			return
		}
		for v := services[i].MinOffset; v <= services[i].MaxOffset; v++ {
			cur[i] = v
			dfs(i + 1)
		}
	}
	dfs(0)
	return best != nil, best
}

func assignmentFeasible(in *Input, idx map[string]int, x []int) bool {
	spanByID := make(map[string]Span, len(in.Spans))
	for _, sp := range in.Spans {
		spanByID[sp.ID] = sp
	}
	for _, sp := range in.Spans {
		if sp.ParentID == "" {
			continue
		}
		p := spanByID[sp.ParentID]
		oc, op := x[idx[sp.ServiceID]], x[idx[p.ServiceID]]
		if sp.Start+oc < p.Start+op || sp.End+oc > p.End+op {
			return false
		}
	}
	return true
}

func lexLess(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

// randomInput 生成合法输入：父子关系只指向更早的 span，保证无环。
func randomInput(rng *rand.Rand, nSvc, bound, nSpans int) *Input {
	in := &Input{}
	anchor := rng.Intn(nSvc)
	for i := 0; i < nSvc; i++ {
		s := Service{ID: fmt.Sprintf("svc-%d", i)}
		if i == anchor {
			s.Anchor = true
		} else {
			lo := rng.Intn(2*bound+1) - bound
			hi := rng.Intn(2*bound+1) - bound
			if lo > hi {
				lo, hi = hi, lo
			}
			s.MinOffset, s.MaxOffset = lo, hi
		}
		in.Services = append(in.Services, s)
	}
	for i := 0; i < nSpans; i++ {
		sp := Span{
			ID:        fmt.Sprintf("sp-%d", i),
			ServiceID: in.Services[rng.Intn(nSvc)].ID,
			Start:     rng.Intn(30),
		}
		sp.End = sp.Start + rng.Intn(20)
		if i > 0 && rng.Intn(4) > 0 {
			sp.ParentID = in.Spans[rng.Intn(i)].ID
		}
		in.Spans = append(in.Spans, sp)
	}
	return in
}

// ---------- 矛盾环逐边核验 ----------

// checkCycleEvidence 验证矛盾环：每条边都来自真实输入约束、
// 边与边首尾相接成环、权重和为负（从而 0 <= 总和 < 0，构成矛盾）。
func checkCycleEvidence(t *testing.T, in *Input, cycle *Cycle) {
	t.Helper()
	if cycle == nil {
		t.Fatal("无可行解但未返回矛盾环")
	}
	if len(cycle.Edges) == 0 {
		t.Fatal("矛盾环为空")
	}
	constraints, err := Constraints(in)
	if err != nil {
		t.Fatalf("重建输入约束失败：%v", err)
	}
	type key struct {
		from, to    string
		weight      int
		kind        ConstraintKind
		spanID, pID string
		serviceID   string
	}
	pool := make(map[key]int, len(constraints))
	for _, c := range constraints {
		pool[key{c.From, c.To, c.Weight, c.Kind, c.SpanID, c.ParentSpanID, c.ServiceID}]++
	}
	svcIDs := map[string]bool{ZeroNode: true}
	for _, s := range in.Services {
		svcIDs[s.ID] = true
	}
	total := 0
	for i, e := range cycle.Edges {
		k := key{e.From, e.To, e.Weight, e.Kind, e.SpanID, e.ParentSpanID, e.ServiceID}
		if pool[k] == 0 {
			t.Fatalf("环上第 %d 条边不是真实输入约束：%+v", i, e)
		}
		pool[k]--
		if !svcIDs[e.From] || !svcIDs[e.To] {
			t.Fatalf("环上第 %d 条边引用了未知节点：%s -> %s", i, e.From, e.To)
		}
		next := cycle.Edges[(i+1)%len(cycle.Edges)]
		if e.To != next.From {
			t.Fatalf("环上第 %d 条边的终点 %s 与下一条起点 %s 不相接", i, e.To, next.From)
		}
		total += e.Weight
	}
	if total != cycle.TotalWeight {
		t.Fatalf("环权重和 %d 与报告值 %d 不一致", total, cycle.TotalWeight)
	}
	if cycle.TotalWeight >= 0 {
		t.Fatalf("矛盾环权重和必须 < 0，实际为 %d", cycle.TotalWeight)
	}
}

// ---------- 测试 ----------

// TestSolveMatchesBruteForce 用少量服务随机生成用例，
// 与穷举所有整数偏移向量的结果逐一对比。
func TestSolveMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(20261001))
	feasible, infeasible := 0, 0
	for trial := 0; trial < 3000; trial++ {
		nSvc := 2 + rng.Intn(2)  // 2~3 个服务，保证穷举代价小
		bound := 1 + rng.Intn(4) // 偏移区间落在 [-4, 4] 内
		nSpans := rng.Intn(9)    // 0~8 个 span
		in := randomInput(rng, nSvc, bound, nSpans)

		res, err := Solve(in)
		if err != nil {
			t.Fatalf("trial %d: Solve 返回错误：%v", trial, err)
		}
		wantOK, wantVec := bruteForce(in)
		if res.Feasible != wantOK {
			t.Fatalf("trial %d: 可行性不一致：Solve=%v 穷举=%v，输入=%+v", trial, res.Feasible, wantOK, *in)
		}
		if !res.Feasible {
			infeasible++
			checkCycleEvidence(t, in, res.Cycle)
			continue
		}
		feasible++

		// 偏移向量必须与穷举出的字典序最小解完全一致。
		if len(res.Offsets) != len(wantVec) {
			t.Fatalf("trial %d: 偏移数量不符", trial)
		}
		for i, o := range res.Offsets {
			if o.Offset != wantVec[i] {
				t.Fatalf("trial %d: 第 %d 个服务 %s 的偏移=%d，穷举最小解=%d", trial, i, o.ServiceID, o.Offset, wantVec[i])
			}
		}
		// 校正后的 span 必须两两满足父子包含，余量非负且与定义一致。
		corr := make(map[string]CorrectedSpan, len(res.CorrectedSpans))
		for _, cs := range res.CorrectedSpans {
			corr[cs.ID] = cs
		}
		for _, m := range res.Margins {
			if m.StartMargin < 0 || m.EndMargin < 0 {
				t.Fatalf("trial %d: 余量为负：%+v", trial, m)
			}
			c, p := corr[m.SpanID], corr[m.ParentSpanID]
			if got := c.Start - p.Start; got != m.StartMargin {
				t.Fatalf("trial %d: span %s 起点余量=%d，校正时间线推算=%d", trial, m.SpanID, m.StartMargin, got)
			}
			if got := p.End - c.End; got != m.EndMargin {
				t.Fatalf("trial %d: span %s 终点余量=%d，校正时间线推算=%d", trial, m.SpanID, m.EndMargin, got)
			}
		}
	}
	if feasible == 0 || infeasible == 0 {
		t.Fatalf("随机用例未同时覆盖可行/不可行：feasible=%d infeasible=%d", feasible, infeasible)
	}
	t.Logf("对拍通过：feasible=%d infeasible=%d", feasible, infeasible)
}

// TestKnownLexMin 验证字典序按服务 ID 排序而非输入顺序，
// 且先固定字典序靠前的变量会影响后续变量的取值。
func TestKnownLexMin(t *testing.T) {
	in := &Input{
		Services: []Service{
			{ID: "c", MinOffset: 0, MaxOffset: 0, Anchor: true},
			{ID: "b", MinOffset: -10, MaxOffset: 10},
			{ID: "a", MinOffset: -10, MaxOffset: 10},
		},
		Spans: []Span{
			{ID: "root", ServiceID: "c", Start: 0, End: 10},
			{ID: "sa", ServiceID: "a", ParentID: "root", Start: 2, End: 8},
			{ID: "sb", ServiceID: "b", ParentID: "sa", Start: 3, End: 5},
		},
	}
	res, err := Solve(in)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Feasible {
		t.Fatalf("应可行，得到矛盾环：%+v", res.Cycle)
	}
	// x[a] ∈ [-2, 2]，x[b] ≥ x[a]-1；字典序最小：a=-2, b=-3, c=0。
	want := []Offset{{ServiceID: "a", Offset: -2}, {ServiceID: "b", Offset: -3}, {ServiceID: "c", Offset: 0}}
	if fmt.Sprint(res.Offsets) != fmt.Sprint(want) {
		t.Fatalf("偏移向量=%v，期望 %v", res.Offsets, want)
	}
	wantMargins := []Margin{
		{SpanID: "sa", ParentSpanID: "root", StartMargin: 0, EndMargin: 4},
		{SpanID: "sb", ParentSpanID: "sa", StartMargin: 0, EndMargin: 4},
	}
	if fmt.Sprint(res.Margins) != fmt.Sprint(wantMargins) {
		t.Fatalf("余量=%v，期望 %v", res.Margins, wantMargins)
	}
}

// TestKnownInfeasible 构造一个手算可验的矛盾：父区间 [0,10]，
// 子区间本地 [5,20]，要求 x[b] ≥ -5 且 x[b] ≤ -10。
func TestKnownInfeasible(t *testing.T) {
	in := &Input{
		Services: []Service{
			{ID: "a", MinOffset: 0, MaxOffset: 0, Anchor: true},
			{ID: "b", MinOffset: -5, MaxOffset: 5},
		},
		Spans: []Span{
			{ID: "p", ServiceID: "a", Start: 0, End: 10},
			{ID: "c", ServiceID: "b", ParentID: "p", Start: 5, End: 20},
		},
	}
	res, err := Solve(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Feasible {
		t.Fatalf("应不可行，得到偏移：%v", res.Offsets)
	}
	checkCycleEvidence(t, in, res.Cycle)
	for _, e := range res.Cycle.Edges {
		if e.Kind != KindSpanStart && e.Kind != KindSpanEnd {
			t.Fatalf("本例矛盾环应只含父子约束，出现 %s", e.Kind)
		}
	}
}

// TestSelfLoopInfeasible 同服务父子 span 本地时间已冲突（自环负权）。
func TestSelfLoopInfeasible(t *testing.T) {
	in := &Input{
		Services: []Service{
			{ID: "a", MinOffset: 0, MaxOffset: 0, Anchor: true},
			{ID: "b", MinOffset: -9, MaxOffset: 9},
		},
		Spans: []Span{
			{ID: "p", ServiceID: "b", Start: 0, End: 5},
			{ID: "c", ServiceID: "b", ParentID: "p", Start: 3, End: 10},
		},
	}
	res, err := Solve(in)
	if err != nil {
		t.Fatal(err)
	}
	if res.Feasible {
		t.Fatal("子 span 本地时间已超出父 span，应不可行")
	}
	checkCycleEvidence(t, in, res.Cycle)
}

func TestValidate(t *testing.T) {
	okSvc := []Service{
		{ID: "a", MinOffset: 0, MaxOffset: 0, Anchor: true},
		{ID: "b", MinOffset: -1, MaxOffset: 1},
	}
	okSpans := []Span{{ID: "p", ServiceID: "a", Start: 0, End: 1}}
	cases := []struct {
		name string
		in   *Input
	}{
		{"服务太少", &Input{Services: okSvc[:1]}},
		{"服务太多", &Input{Services: append(okSvc, make([]Service, 7)...)}},
		{"span 太多", &Input{Services: okSvc, Spans: make([]Span, 101)}},
		{"服务 ID 为空", &Input{Services: []Service{{ID: "", Anchor: true}, {ID: "b", MinOffset: -1, MaxOffset: 1}}}},
		{"服务 ID 重复", &Input{Services: []Service{{ID: "a", Anchor: true}, {ID: "a", MinOffset: -1, MaxOffset: 1}}}},
		{"区间非法", &Input{Services: []Service{{ID: "a", Anchor: true}, {ID: "b", MinOffset: 2, MaxOffset: 1}}}},
		{"无锚定", &Input{Services: []Service{{ID: "a", MinOffset: -1, MaxOffset: 1}, {ID: "b", MinOffset: -1, MaxOffset: 1}}}},
		{"双锚定", &Input{Services: []Service{{ID: "a", Anchor: true}, {ID: "b", Anchor: true}}}},
		{"锚定区间非零", &Input{Services: []Service{{ID: "a", MinOffset: -1, MaxOffset: 1, Anchor: true}, {ID: "b", MinOffset: -1, MaxOffset: 1}}}},
		{"span 引用未知服务", &Input{Services: okSvc, Spans: []Span{{ID: "p", ServiceID: "zz", Start: 0, End: 1}}}},
		{"span 起终颠倒", &Input{Services: okSvc, Spans: []Span{{ID: "p", ServiceID: "a", Start: 2, End: 1}}}},
		{"span ID 重复", &Input{Services: okSvc, Spans: []Span{okSpans[0], okSpans[0]}}},
		{"父 span 不存在", &Input{Services: okSvc, Spans: []Span{{ID: "p", ServiceID: "a", ParentID: "zz", Start: 0, End: 1}}}},
		{"父子成环", &Input{Services: okSvc, Spans: []Span{
			{ID: "p", ServiceID: "a", ParentID: "c", Start: 0, End: 9},
			{ID: "c", ServiceID: "a", ParentID: "p", Start: 1, End: 2},
		}}},
	}
	for _, tc := range cases {
		if _, err := Solve(tc.in); err == nil {
			t.Fatalf("%s：应返回校验错误", tc.name)
		}
	}
	good := &Input{Services: okSvc, Spans: okSpans}
	if _, err := Solve(good); err != nil {
		t.Fatalf("合法输入被误判：%v", err)
	}
}
