package uast

import (
	"fmt"
	"sort"
	"strconv"

	"srcd.works/go-errors.v0"
)

var (
	ErrEmptyAST             = errors.NewKind("input AST was empty")
	ErrUnexpectedObject     = errors.NewKind("expected object of type %s, got: %#v")
	ErrUnexpectedObjectSize = errors.NewKind("expected object of size %d, got %d")
	ErrUnsupported          = errors.NewKind("unsupported: %s")
)

// OriginalToNoder is a converter of source ASTs to *Node.
type OriginalToNoder interface {
	// OriginalToNode converts the source AST to a *Node.
	OriginalToNode(src map[string]interface{}) (*Node, error)
}

const (
	// topLevelIsRootNode is true if the top level object is the root node
	// of the AST. If false, top level object should have a single key, that
	// being the root node.
	topLevelIsRootNode = false
	// InternalRoleKey is a key string uses in properties to use the internal
	// role of a node in the AST, if any.
	InternalRoleKey = "internalRole"
)

// BaseOriginalToNoder is an implementation of OriginalToNoder that aims to work
// for the most common source ASTs.
type BaseOriginalToNoder struct {
	// InternalTypeKey is a key in the source AST that can be used to get the
	// InternalType of a node.
	InternalTypeKey string
	// OffsetKey is a key that indicates the position offset.
	OffsetKey string
	// LineKey is a key that indicates the line number.
	LineKey string
	// TokenKeys is a slice of keys used to extract token content.
	TokenKeys map[string]bool
	// SyntheticTokens is a map of InternalType to string used to add
	// synthetic tokens to nodes depending on its InternalType.
	SyntheticTokens map[string]string
}

func (c *BaseOriginalToNoder) OriginalToNode(src map[string]interface{}) (*Node, error) {
	if len(src) == 0 {
		return nil, ErrEmptyAST.New()
	}

	if topLevelIsRootNode {
		return nil, ErrUnsupported.New("top level object as root node")
	}

	if len(src) > 1 {
		return nil, ErrUnexpectedObjectSize.New(1, len(src))
	}

	for _, obj := range src {
		return c.toNode(obj)
	}

	panic("not reachable")
}

func (c *BaseOriginalToNoder) toNode(obj interface{}) (*Node, error) {
	m, ok := obj.(map[string]interface{})
	if !ok {
		return nil, ErrUnexpectedObject.New("map[string]interface{}", obj)
	}

	n := NewNode()
	for k, o := range m {

		switch ov := o.(type) {
		case map[string]interface{}:
			child, err := c.mapToNode(k, ov)
			if err != nil {
				return nil, err
			}

			n.Children = append(n.Children, child)
		case []interface{}:
			children, err := c.sliceToNodes(k, ov)
			if err != nil {
				return nil, err
			}

			n.Children = append(n.Children, children...)
		default:
			if err := c.addProperty(n, k, o); err != nil {
				return nil, err
			}
		}
	}

	sort.Sort(byOffset(n.Children))
	return n, nil
}

func (c *BaseOriginalToNoder) mapToNode(k string, obj map[string]interface{}) (*Node, error) {
	n, err := c.toNode(obj)
	if err != nil {
		return nil, err
	}

	n.Properties[InternalRoleKey] = k
	return n, nil
}

func (c *BaseOriginalToNoder) sliceToNodes(k string, s []interface{}) ([]*Node, error) {
	var ns []*Node
	for _, v := range s {
		n, err := c.toNode(v)
		if err != nil {
			return nil, err
		}

		n.Properties[InternalRoleKey] = k
		ns = append(ns, n)
	}

	return ns, nil
}

func (c *BaseOriginalToNoder) addProperty(n *Node, k string, o interface{}) error {
	switch {
	case c.isTokenKey(k):
		if n.Token != nil {
			return fmt.Errorf("two token keys for same node: %s", k)
		}

		s := fmt.Sprint(o)
		n.Token = &s
	case c.InternalTypeKey == k:
		s := fmt.Sprint(o)
		if err := c.setInternalKey(n, s); err != nil {
			return err
		}

		tk := c.syntheticToken(s)
		if tk != nil {
			if n.Token != nil {
				return fmt.Errorf("two token keys for same node: %s", k)
			}

			n.Token = tk
		}
	case c.OffsetKey == k:
		i, err := toUint32(o)
		if err != nil {
			return err
		}

		if n.StartPosition == nil {
			n.StartPosition = &Position{}
		}

		n.StartPosition.Offset = &i
	case c.LineKey == k:
		i, err := toUint32(o)
		if err != nil {
			return err
		}

		if n.StartPosition == nil {
			n.StartPosition = &Position{}
		}

		n.StartPosition.Line = &i
	default:
		n.Properties[k] = fmt.Sprint(0)
	}

	return nil
}

func (c *BaseOriginalToNoder) isTokenKey(key string) bool {
	return c.TokenKeys != nil && c.TokenKeys[key]
}

func (c *BaseOriginalToNoder) syntheticToken(key string) *string {
	if c.SyntheticTokens == nil {
		return nil
	}

	t, ok := c.SyntheticTokens[key]
	if !ok {
		return nil
	}

	return &t
}

func (c *BaseOriginalToNoder) setInternalKey(n *Node, k string) error {
	if n.InternalType != "" {
		return fmt.Errorf("two internal keys for same node: %s, %s",
			n.InternalType, k)
	}

	n.InternalType = k
	return nil
}

func toUint32(v interface{}) (uint32, error) {
	switch o := v.(type) {
	case string:
		i, err := strconv.ParseUint(o, 10, 32)
		if err != nil {
			return 0, err
		}

		return uint32(i), nil
	case uint32:
		return o, nil
	case int:
		return uint32(o), nil
	case int32:
		return uint32(o), nil
	case int64:
		return uint32(o), nil
	default:
		return 0, fmt.Errorf("toUint32 error: %#v", v)
	}
}

type byOffset []*Node

func (s byOffset) Len() int      { return len(s) }
func (s byOffset) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s byOffset) Less(i, j int) bool {
	a := s[i]
	b := s[j]
	ao := a.offset()
	bo := b.offset()
	if ao == nil {
		return false
	}

	if bo == nil {
		return true
	}

	return *ao < *bo
}
