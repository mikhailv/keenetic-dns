package lookup

import (
	"slices"
	"strings"
)

// DomainTree is an immutable trie for domain suffix matching.
// Keys are FQDNs (e.g. "youtube.com."). Lookup finds the longest suffix match.
type DomainTree[V any] struct {
	root domainNode[V]
}

type domainNode[V any] struct {
	labels   []string        // sorted
	children []domainNode[V] // parallel to labels
	value    V
	hasValue bool
}

func (n *domainNode[V]) child(label string) *domainNode[V] {
	i, ok := slices.BinarySearch(n.labels, label)
	if !ok {
		return nil
	}
	return &n.children[i]
}

// Get returns the value associated with the longest matching domain suffix.
func (t DomainTree[V]) Get(domain string) (V, bool) {
	labels := splitDomainReversed(domain)
	var bestValue V
	var found bool
	node := &t.root
	if node.hasValue {
		bestValue = node.value
		found = true
	}
	for _, label := range labels {
		node = node.child(label)
		if node == nil {
			break
		}
		if node.hasValue {
			bestValue = node.value
			found = true
		}
	}
	return bestValue, found
}

// Empty reports whether the tree contains no entries.
func (t DomainTree[V]) Empty() bool {
	return !t.root.hasValue && len(t.root.labels) == 0
}

// Has reports whether domain matches any suffix in the tree.
func (t DomainTree[V]) Has(domain string) bool {
	_, ok := t.Get(domain)
	return ok
}

// DomainTreeBuilder builds an immutable DomainTree.
type DomainTreeBuilder[V any] struct {
	root domainBuildNode[V]
}

type domainBuildNode[V any] struct {
	children map[string]*domainBuildNode[V]
	value    V
	hasValue bool
}

func NewDomainTreeBuilder[V any]() DomainTreeBuilder[V] {
	return DomainTreeBuilder[V]{}
}

// Add inserts a domain pattern with an associated value.
// Domain should be an FQDN (e.g. "youtube.com." or "youtube.com").
func (b *DomainTreeBuilder[V]) Add(domain string, value V) {
	labels := splitDomainReversed(domain)
	node := &b.root
	for _, label := range labels {
		if node.children == nil {
			node.children = make(map[string]*domainBuildNode[V])
		}
		child, ok := node.children[label]
		if !ok {
			child = &domainBuildNode[V]{}
			node.children[label] = child
		}
		node = child
	}
	node.value = value
	node.hasValue = true
}

// Build creates an immutable DomainTree from the builder's contents.
func (b DomainTreeBuilder[V]) Build() DomainTree[V] {
	return DomainTree[V]{root: freezeDomainNode(&b.root)}
}

func freezeDomainNode[V any](bn *domainBuildNode[V]) domainNode[V] {
	n := domainNode[V]{value: bn.value, hasValue: bn.hasValue}
	if len(bn.children) == 0 {
		return n
	}
	n.labels = make([]string, 0, len(bn.children))
	for label := range bn.children {
		n.labels = append(n.labels, label)
	}
	slices.Sort(n.labels)
	n.children = make([]domainNode[V], len(n.labels))
	for i, label := range n.labels {
		n.children[i] = freezeDomainNode(bn.children[label])
	}
	return n
}

// splitDomainReversed splits a domain into labels in reverse order.
// "foo.youtube.com." → ["com", "youtube", "foo"].
func splitDomainReversed(domain string) []string {
	domain = strings.Trim(domain, ".")
	if domain == "" {
		return nil
	}
	parts := strings.Split(domain, ".")
	slices.Reverse(parts)
	return parts
}
