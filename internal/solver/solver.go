// Package solver 把“校正后每个子调用必须完整落在父调用区间内”的要求
// 化成服务时钟偏移量上的差分约束（difference constraints），
// 用 Bellman-Ford 判定可行性、求按服务 ID 字典序最小的可行偏移向量，
// 并在无解时抽取由真实输入约束构成的矛盾环。
package solver

import (
	"fmt"
	"sort"
)

// ZeroNode 是绝对界约束（偏移上下界、锚定）使用的伪节点 ID。
// 约束 x[v] - x[u] <= w 中 u 或 v 为 ZeroNode 时，x[ZeroNode] 恒为 0。
const ZeroNode = "ZERO"

// Service 描述一个服务允许的整数时钟偏移区间。
type Service struct {
	ID        string `json:"id"`
	MinOffset int    `json:"minOffset"`
	MaxOffset int    `json:"maxOffset"`
	Anchor    bool   `json:"anchor"`
}

// Span 是一次调用片段，起止时刻是该服务本地时钟上的整数。
// ParentID 为空串表示根 span。
type Span struct {
	ID        string `json:"id"`
	ServiceID string `json:"serviceId"`
	ParentID  string `json:"parentId"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
}

// Input 是一次求解请求的全部输入。
type Input struct {
	Services []Service `json:"services"`
	Spans    []Span    `json:"spans"`
}

// ConstraintKind 标记一条约束边来自哪类输入事实。
type ConstraintKind string

const (
	KindSpanStart ConstraintKind = "span-start" // 子 span 开始不得早于父 span
	KindSpanEnd   ConstraintKind = "span-end"   // 子 span 结束不得晚于父 span
	KindBoundMin  ConstraintKind = "bound-min"  // 偏移下界
	KindBoundMax  ConstraintKind = "bound-max"  // 偏移上界
	KindAnchor    ConstraintKind = "anchor"     // 锚定服务偏移为 0
)

// Constraint 是一条呈现形式的差分约束：x[To] - x[From] <= Weight。
type Constraint struct {
	From         string         `json:"from"` // 服务 ID 或 ZeroNode
	To           string         `json:"to"`
	Weight       int            `json:"weight"`
	Kind         ConstraintKind `json:"kind"`
	ServiceID    string         `json:"serviceId,omitempty"`
	SpanID       string         `json:"spanId,omitempty"`
	ParentSpanID string         `json:"parentSpanId,omitempty"`
	Detail       string         `json:"detail"`
}

// Offset 是某服务求得的时钟偏移。
type Offset struct {
	ServiceID string `json:"serviceId"`
	Offset    int    `json:"offset"`
}

// CorrectedSpan 是加上偏移后的 span，供前端直接绘制时间线。
type CorrectedSpan struct {
	ID        string `json:"id"`
	ServiceID string `json:"serviceId"`
	ParentID  string `json:"parentId"`
	Start     int    `json:"start"`
	End       int    `json:"end"`
}

// Margin 是一条父子约束在校正后的余量（均 >= 0）。
type Margin struct {
	SpanID       string `json:"spanId"`
	ParentSpanID string `json:"parentSpanId"`
	StartMargin  int    `json:"startMargin"` // 子起点 - 父起点
	EndMargin    int    `json:"endMargin"`   // 父终点 - 子终点
}

// Cycle 是无解时的证据：一串首尾相接、权重和为负的真实输入约束。
type Cycle struct {
	Edges       []Constraint `json:"edges"`
	TotalWeight int          `json:"totalWeight"`
}

// Result 是求解结果。Feasible 为 false 时只有 Cycle 有值。
type Result struct {
	Feasible       bool            `json:"feasible"`
	Offsets        []Offset        `json:"offsets,omitempty"`
	CorrectedSpans []CorrectedSpan `json:"correctedSpans,omitempty"`
	Margins        []Margin        `json:"margins,omitempty"`
	Cycle          *Cycle          `json:"cycle,omitempty"`
}

// Validate 校验输入是否满足题目限制。
func Validate(in *Input) error {
	if n := len(in.Services); n < 2 || n > 8 {
		return fmt.Errorf("服务数量须在 2~8 之间，当前为 %d", n)
	}
	if len(in.Spans) > 100 {
		return fmt.Errorf("span 数量至多为 100，当前为 %d", len(in.Spans))
	}
	svcIDs := make(map[string]bool, len(in.Services))
	anchors := 0
	for _, s := range in.Services {
		if s.ID == "" {
			return fmt.Errorf("服务 ID 不能为空")
		}
		if s.ID == ZeroNode {
			return fmt.Errorf("服务 ID 不能使用保留名 %q", ZeroNode)
		}
		if svcIDs[s.ID] {
			return fmt.Errorf("服务 ID 重复：%s", s.ID)
		}
		svcIDs[s.ID] = true
		if s.MinOffset > s.MaxOffset {
			return fmt.Errorf("服务 %s 的偏移区间非法：[%d, %d]", s.ID, s.MinOffset, s.MaxOffset)
		}
		if s.Anchor {
			anchors++
			if s.MinOffset != 0 || s.MaxOffset != 0 {
				return fmt.Errorf("锚定服务 %s 的偏移区间必须为 [0, 0]", s.ID)
			}
		}
	}
	if anchors != 1 {
		return fmt.Errorf("必须恰好有一个锚定服务（偏移固定为 0），当前有 %d 个", anchors)
	}
	spanIDs := make(map[string]bool, len(in.Spans))
	for _, sp := range in.Spans {
		if sp.ID == "" {
			return fmt.Errorf("span ID 不能为空")
		}
		if spanIDs[sp.ID] {
			return fmt.Errorf("span ID 重复：%s", sp.ID)
		}
		spanIDs[sp.ID] = true
		if !svcIDs[sp.ServiceID] {
			return fmt.Errorf("span %s 引用了未知服务 %s", sp.ID, sp.ServiceID)
		}
		if sp.Start > sp.End {
			return fmt.Errorf("span %s 的开始时刻 %d 晚于结束时刻 %d", sp.ID, sp.Start, sp.End)
		}
	}
	parent := make(map[string]string, len(in.Spans))
	for _, sp := range in.Spans {
		if sp.ParentID != "" && !spanIDs[sp.ParentID] {
			return fmt.Errorf("span %s 的父 span %s 不存在", sp.ID, sp.ParentID)
		}
		parent[sp.ID] = sp.ParentID
	}
	for _, sp := range in.Spans {
		seen := map[string]bool{sp.ID: true}
		for cur := parent[sp.ID]; cur != ""; cur = parent[cur] {
			if seen[cur] {
				return fmt.Errorf("span 父子关系存在环（涉及 %s）", cur)
			}
			seen[cur] = true
		}
	}
	return nil
}

// edge 是内部约束图的一条边：x[to] - x[from] <= weight。
type edge struct {
	from, to int
	weight   int
	label    Constraint
}

// system 是建好的约束图。节点 0..n-1 是按 ID 排序的服务，节点 n 是 ZERO。
type system struct {
	services []Service
	n        int
	edges    []edge
}

func buildSystem(in *Input) *system {
	services := append([]Service(nil), in.Services...)
	sort.Slice(services, func(i, j int) bool { return services[i].ID < services[j].ID })
	idx := make(map[string]int, len(services))
	for i, s := range services {
		idx[s.ID] = i
	}
	n := len(services)
	zero := n
	name := func(v int) string {
		if v == zero {
			return ZeroNode
		}
		return services[v].ID
	}
	sys := &system{services: services, n: n}
	add := func(from, to, w int, c Constraint) {
		c.From, c.To, c.Weight = name(from), name(to), w
		sys.edges = append(sys.edges, edge{from: from, to: to, weight: w, label: c})
	}
	for i, s := range services {
		if s.Anchor {
			add(zero, i, 0, Constraint{Kind: KindAnchor, ServiceID: s.ID,
				Detail: fmt.Sprintf("锚定服务 %s：偏移固定为 0（x[%s] ≤ 0）", s.ID, s.ID)})
			add(i, zero, 0, Constraint{Kind: KindAnchor, ServiceID: s.ID,
				Detail: fmt.Sprintf("锚定服务 %s：偏移固定为 0（x[%s] ≥ 0）", s.ID, s.ID)})
			continue
		}
		add(zero, i, s.MaxOffset, Constraint{Kind: KindBoundMax, ServiceID: s.ID,
			Detail: fmt.Sprintf("服务 %s 偏移上界：x[%s] ≤ %d", s.ID, s.ID, s.MaxOffset)})
		add(i, zero, -s.MinOffset, Constraint{Kind: KindBoundMin, ServiceID: s.ID,
			Detail: fmt.Sprintf("服务 %s 偏移下界：x[%s] ≥ %d", s.ID, s.ID, s.MinOffset)})
	}
	spanByID := make(map[string]Span, len(in.Spans))
	for _, sp := range in.Spans {
		spanByID[sp.ID] = sp
	}
	for _, sp := range in.Spans {
		if sp.ParentID == "" {
			continue
		}
		p := spanByID[sp.ParentID]
		cs, ps := idx[sp.ServiceID], idx[p.ServiceID]
		// 子起点 + x[cs] ≥ 父起点 + x[ps]  ⇔  x[ps] - x[cs] ≤ 子起点 - 父起点
		add(cs, ps, sp.Start-p.Start, Constraint{
			Kind: KindSpanStart, SpanID: sp.ID, ParentSpanID: p.ID,
			Detail: fmt.Sprintf("子 span %s（%s）开始不得早于父 span %s（%s）", sp.ID, sp.ServiceID, p.ID, p.ServiceID),
		})
		// 子终点 + x[cs] ≤ 父终点 + x[ps]  ⇔  x[cs] - x[ps] ≤ 父终点 - 子终点
		add(ps, cs, p.End-sp.End, Constraint{
			Kind: KindSpanEnd, SpanID: sp.ID, ParentSpanID: p.ID,
			Detail: fmt.Sprintf("子 span %s（%s）结束不得晚于父 span %s（%s）", sp.ID, sp.ServiceID, p.ID, p.ServiceID),
		})
	}
	return sys
}

// Constraints 返回输入化出的全部差分约束（呈现形式），
// 供测试与调用方核对矛盾环是否确实由真实输入约束构成。
func Constraints(in *Input) ([]Constraint, error) {
	if err := Validate(in); err != nil {
		return nil, err
	}
	sys := buildSystem(in)
	out := make([]Constraint, len(sys.edges))
	for i, e := range sys.edges {
		out[i] = e.label
	}
	return out, nil
}

// bellmanFord 以“虚拟源点向所有节点连 0 权边”（等价于 dist 全 0 初始化）
// 跑 Bellman-Ford。存在负环时返回 ok=false 及构成负环的边下标，
// 边按有向环顺序排列（前一条的 To 是后一条的 From）。
func bellmanFord(numNodes int, edges []edge) (dist []int, cycle []int, ok bool) {
	dist = make([]int, numNodes)
	pred := make([]int, numNodes)
	for i := range pred {
		pred[i] = -1
	}
	last := -1
	for round := 0; round < numNodes; round++ {
		last = -1
		for j, e := range edges {
			if dist[e.to] > dist[e.from]+e.weight {
				dist[e.to] = dist[e.from] + e.weight
				pred[e.to] = j
				last = e.to
			}
		}
		if last == -1 {
			return dist, nil, true
		}
	}
	// 第 numNodes 轮仍被松弛的节点在负环上或其下游；
	// 沿前驱走 numNodes 步必进入环内。
	v := last
	for i := 0; i < numNodes; i++ {
		v = edges[pred[v]].from
	}
	var cyc []int
	for cur := v; ; {
		j := pred[cur]
		cyc = append(cyc, j)
		if cur = edges[j].from; cur == v {
			break
		}
	}
	// 收集到的是沿前驱逆向的边，反转后首尾相接。
	for i, j := 0, len(cyc)-1; i < j; i, j = i+1, j-1 {
		cyc[i], cyc[j] = cyc[j], cyc[i]
	}
	return dist, cyc, false
}

const inf = int(^uint(0) >> 1)

// bellmanFordFrom 从单个源点求最短路；发现负环返回 ok=false。
func bellmanFordFrom(numNodes int, edges []edge, source int) (dist []int, ok bool) {
	dist = make([]int, numNodes)
	for i := range dist {
		dist[i] = inf
	}
	dist[source] = 0
	for round := 0; round < numNodes; round++ {
		changed := false
		for _, e := range edges {
			if dist[e.from] == inf {
				continue
			}
			if dist[e.to] > dist[e.from]+e.weight {
				if round == numNodes-1 {
					return nil, false
				}
				dist[e.to] = dist[e.from] + e.weight
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return dist, true
}

// Solve 求解输入。输入非法时返回 error；无可行偏移时返回带矛盾环的结果；
// 可行时返回按服务 ID 字典序最小的偏移向量、校正后的 span 与每条父子约束的余量。
func Solve(in *Input) (*Result, error) {
	if err := Validate(in); err != nil {
		return nil, err
	}
	sys := buildSystem(in)
	_, cycIdx, ok := bellmanFord(sys.n+1, sys.edges)
	if !ok {
		cycle := &Cycle{}
		for _, j := range cycIdx {
			cycle.Edges = append(cycle.Edges, sys.edges[j].label)
			cycle.TotalWeight += sys.edges[j].weight
		}
		return &Result{Feasible: false, Cycle: cycle}, nil
	}

	// 逐变量求字典序最小解：固定 x[0..i-1] 后，x[i] 的最小可行值
	// 等于 y[i] 最大可行值的相反数（y = -x），而 y 的最大可行解
	// 正是 y 约束图上从 ZERO 出发的最短路。
	chosen := make([]int, sys.n)
	for i := 0; i < sys.n; i++ {
		yEdges := make([]edge, 0, len(sys.edges)+2*i)
		for _, e := range sys.edges {
			yEdges = append(yEdges, edge{from: e.to, to: e.from, weight: e.weight})
		}
		for j := 0; j < i; j++ {
			// x[j] = chosen[j] ⇔ y[j] = -chosen[j]
			yEdges = append(yEdges, edge{from: sys.n, to: j, weight: -chosen[j]})
			yEdges = append(yEdges, edge{from: j, to: sys.n, weight: chosen[j]})
		}
		d, ok := bellmanFordFrom(sys.n+1, yEdges, sys.n)
		if !ok {
			return nil, fmt.Errorf("内部错误：固定前 %d 个变量后系统意外不可行", i)
		}
		chosen[i] = -d[i]
	}

	offsetOf := make(map[string]int, sys.n)
	res := &Result{Feasible: true}
	for i, s := range sys.services {
		offsetOf[s.ID] = chosen[i]
		res.Offsets = append(res.Offsets, Offset{ServiceID: s.ID, Offset: chosen[i]})
	}
	spanByID := make(map[string]Span, len(in.Spans))
	for _, sp := range in.Spans {
		spanByID[sp.ID] = sp
	}
	for _, sp := range in.Spans {
		off := offsetOf[sp.ServiceID]
		res.CorrectedSpans = append(res.CorrectedSpans, CorrectedSpan{
			ID: sp.ID, ServiceID: sp.ServiceID, ParentID: sp.ParentID,
			Start: sp.Start + off, End: sp.End + off,
		})
		if sp.ParentID == "" {
			continue
		}
		p := spanByID[sp.ParentID]
		pOff := offsetOf[p.ServiceID]
		res.Margins = append(res.Margins, Margin{
			SpanID:       sp.ID,
			ParentSpanID: p.ID,
			StartMargin:  (sp.Start + off) - (p.Start + pOff),
			EndMargin:    (p.End + pOff) - (sp.End + off),
		})
	}
	return res, nil
}
