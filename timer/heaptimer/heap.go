package heaptimer

import (
	"container/list"
	"errors"
	"math"
	"sync"
)

var (
	// ErrEmpty heap is empty
	ErrEmpty = errors.New("heap is empty")
)

// T 堆类型
type T int

const (
	// MaxHeap 最大堆
	MaxHeap T = iota
	// MinHeap 最小堆
	MinHeap
)

// Interface 用于两个 Interface 比较
type Interface interface {
	Key() interface{} // 唯一标识
	Value() float64   // 排序指标
}

type node struct {
	self     *list.Element
	parent   *node
	children *list.List

	degree uint
	marked bool

	value Interface
	key   interface{}
	v     float64
}

type FibHeap struct {
	main        *node
	root        *list.List
	num         uint
	index       map[interface{}]*node
	degreeTable map[uint]*list.Element

	t       T
	mux     sync.RWMutex
	compare func(a, b float64) bool
}

// NewFibHeap 初始化 fibonacci heap
func NewFibHeap(t T) *FibHeap {
	heap := &FibHeap{
		index:       make(map[interface{}]*node),
		degreeTable: make(map[uint]*list.Element),

		root: list.New(),
		t:    t,
	}
	if t == MinHeap {
		heap.compare = func(a, b float64) bool {
			return a <= b
		}
	} else {
		heap.compare = func(a, b float64) bool {
			return a >= b
		}
	}
	return heap
}

// T heap 的类型
//	最小堆/最大堆
func (h *FibHeap) T() T {
	return h.t
}

// Insert 插入一个元素
//	元素的排序指标范围为 (-inf, +inf)
func (h *FibHeap) Insert(val Interface) error {
	if math.IsInf(val.Value(), 0) {
		return errors.New("infinity is for internal use only")
	}

	h.mux.Lock()
	if _, ok := h.index[val.Key()]; ok {
		h.mux.Unlock()
		return errors.New("duplicate key is not allowed")
	}

	h.insertValue(val)
	h.mux.Unlock()
	return nil
}

func (h *FibHeap) insertValue(val Interface) {
	n := &node{
		value: val,
		key:   val.Key(),
		v:     val.Value(),
	}
	n.children = list.New()
	n.self = h.root.PushBack(n)

	h.index[val.Key()] = n
	h.num++
	if h.main == nil || h.compare(n.v, h.main.v) {
		h.main = n
		return
	}
}

// Pop 返回并移除堆顶元素
func (h *FibHeap) Pop() (val Interface, err error) {
	h.mux.Lock()
	if h.main == nil {
		h.mux.Unlock()
		return nil, ErrEmpty
	}
	node := h.popNode()
	h.mux.Unlock()
	return node.value, nil
}
func (h *FibHeap) popNode() *node {
	top := h.main
	h.root.Remove(top.self)
	delete(h.degreeTable, top.degree)
	delete(h.index, top.key)
	h.num--

	if h.num == 0 {
		h.main = nil
	} else {
		children := top.children
		if children != nil {
			for e := children.Front(); e != nil; e = e.Next() {
				child := e.Value.(*node)
				child.parent = nil
				child.self = h.root.PushBack(child)
			}
		}
		h.consolidate()
	}

	return top
}

func (h *FibHeap) consolidate() {
	var main *node
	if v := h.root.Front(); v != nil {
		main = v.Value.(*node)
	}
	for tree := h.root.Front(); tree != nil; {
		treeNode := tree.Value.(*node)
		if h.compare(treeNode.v, main.v) {
			main = treeNode
		}

		if otherTree, ok := h.degreeTable[tree.Value.(*node).degree]; !ok {
			h.degreeTable[treeNode.degree] = tree
			tree = tree.Next()
		} else {
			if tree == otherTree {
				tree = tree.Next()
				continue
			}

			otherTreeNode := otherTree.Value.(*node)
			delete(h.degreeTable, otherTreeNode.degree)

			if h.compare(treeNode.v, otherTreeNode.v) {
				h.root.Remove(otherTree)
				h.merge(treeNode, otherTreeNode)
				h.degreeTable[treeNode.degree] = tree
				tree = tree.Next()
			} else {
				nextTree := tree.Next()
				h.root.Remove(tree)
				h.merge(otherTreeNode, treeNode)
				h.degreeTable[otherTreeNode.degree] = otherTree
				tree = nextTree
			}
		}
	}
	h.main = main
}

func (h *FibHeap) merge(parent, child *node) {
	child.marked = false
	child.self = parent.children.PushBack(child)
	child.parent = parent
	parent.degree++
}

// Peek 返回堆顶元素（不移除）
func (h *FibHeap) Peek() (val Interface, err error) {
	h.mux.RLock()

	if h.main == nil {
		h.mux.RUnlock()
		return nil, ErrEmpty
	}

	val = h.main.value
	h.mux.RUnlock()
	return
}

// UpdateValue 根据元素的 key 更新其值
func (h *FibHeap) UpdateValue(val Interface) {
	h.mux.Lock()
	p, ok := h.index[val.Key()]
	if !ok {
		h.mux.Unlock()
		return
	}

	p.v = val.Value()
	if val.Value() < p.value.Value() {
		h.decreaseValue(p)
	} else {
		h.increaseValue(p)
	}
	p.value = val
	h.mux.Unlock()
}

func (h *FibHeap) decreaseValue(p *node) {
	parent := p.parent
	if h.t == MinHeap {
		if parent == nil { // 是根链表节点
			if h.compare(p.v, h.main.v) {
				h.main = p
			}
			return
		}
		if !h.compare(p.v, parent.v) { // 没有破坏最x堆性质
			return
		}

		h.cut(p)
		if h.compare(p.v, h.main.v) {
			h.main = p
		}
		h.cascadingCut(parent)
	} else {
		h.moveChildren2Root(p)
		h.cut(p)
		h.cascadingCut(parent)
		if p == h.main {
			h.findMain()
		}
	}
}

func (h *FibHeap) increaseValue(p *node) {
	parent := p.parent
	if h.t == MinHeap {
		h.moveChildren2Root(p)
		h.cut(p)
		h.cascadingCut(parent)
		if p == h.main {
			h.findMain()
		}
	} else {
		if parent == nil {
			if h.compare(p.v, h.main.v) {
				h.main = p
			}
			return
		}

		if !h.compare(p.v, parent.v) {
			return
		}

		h.cut(p)
		if h.compare(p.v, h.main.v) {
			h.main = p
			return
		}
		h.cascadingCut(parent)
	}
}

func (h *FibHeap) cut(p *node) {
	p.marked = false
	if p.parent == nil {
		return
	}

	parent := p.parent
	parent.degree--
	parent.children.Remove(p.self)
	p.parent = nil
	p.self = h.root.PushBack(p)
}

func (h *FibHeap) cascadingCut(parent *node) {
	if parent == nil {
		return
	}

	if !parent.marked {
		parent.marked = true
		return
	}

	h.cut(parent)
	h.cascadingCut(parent.parent)
}

func (h *FibHeap) moveChildren2Root(p *node) {
	children := p.children
	if children == nil {
		return
	}

	for e := children.Front(); e != nil; {
		child := e.Value.(*node)
		child.parent = nil
		child.self = h.root.PushBack(child)

		next := e.Next()
		children.Remove(e)
		e = next
	}
	p.degree = 0
}

func (h *FibHeap) findMain() {
	start := h.root.Front()
	if start == nil {
		return
	}

	main := start.Value.(*node)
	for tree := start.Next(); tree != nil; tree = tree.Next() {
		p := tree.Value.(*node)
		if h.compare(p.v, main.v) {
			main = p
		}
	}
	h.main = main
}

// Union 合并另一个堆
func (h *FibHeap) Union(target *FibHeap) error {
	h.mux.Lock()

	for k := range target.index {
		if _, exists := h.index[k]; exists {
			h.mux.Unlock()
			return errors.New("duplicate tag is found in the target heap")
		}
	}

	for _, node := range target.index {
		h.insertValue(node.value)
	}

	h.mux.Unlock()
	return nil
}

// Delete 删除元素
// 	先将元素的排序指标更新为无穷大/无穷小，然后 pop 出堆顶元素
func (h *FibHeap) Delete(key interface{}) Interface {
	h.mux.Lock()
	if node, exists := h.index[key]; exists {
		h.deleteNode(node)
		h.mux.Unlock()
		return node.value
	}
	h.mux.Unlock()
	return nil
}

func (h *FibHeap) deleteNode(n *node) {
	if h.t == MinHeap {
		n.v = math.Inf(-1)
		h.decreaseValue(n)
	} else {
		n.v = math.Inf(1)
		h.increaseValue(n)
	}
	h.popNode()
}
